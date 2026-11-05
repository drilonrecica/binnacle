// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dockercollector "github.com/drilonrecica/binnacle/internal/collector/docker"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

// scriptedDocker is a Docker client whose list, inspect results and event
// streams are set per test step; it counts Inspect calls per container.
type scriptedDocker struct {
	mu           sync.Mutex
	list         []dockerapi.Container
	inspects     map[string]dockerapi.Inspect
	inspectCalls map[string]int
	// streams are handed out one per Events call; once exhausted, Events
	// returns a stream that stays open until the context ends.
	streams       []chan dockerapi.Event
	subscriptions []time.Time
	// listErr fails List; stats replaces the default Stats answer.
	listErr error
	stats   func(ctx context.Context, id string) (dockerapi.Stats, error)
}

func newScriptedDocker() *scriptedDocker {
	return &scriptedDocker{inspects: map[string]dockerapi.Inspect{}, inspectCalls: map[string]int{}}
}

func (d *scriptedDocker) add(id, name, state, health string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.list = append(d.list, dockerapi.Container{ID: id, Name: name, State: state})
	d.inspects[id] = dockerapi.Inspect{ID: id, Name: name, Image: "example/" + name, State: state, Health: health, Labels: map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": name}}
}

func (d *scriptedDocker) setState(id, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.list {
		if d.list[i].ID == id {
			d.list[i].State = state
		}
	}
	v := d.inspects[id]
	v.State = state
	d.inspects[id] = v
}

func (d *scriptedDocker) remove(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.list {
		if d.list[i].ID == id {
			d.list = append(d.list[:i], d.list[i+1:]...)
			break
		}
	}
	delete(d.inspects, id)
}

func (d *scriptedDocker) calls(id string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.inspectCalls[id]
}

func (d *scriptedDocker) List(context.Context) ([]dockerapi.Container, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.listErr != nil {
		return nil, d.listErr
	}
	return append([]dockerapi.Container(nil), d.list...), nil
}
func (d *scriptedDocker) Inspect(_ context.Context, id string) (dockerapi.Inspect, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inspectCalls[id]++
	v, ok := d.inspects[id]
	if !ok {
		return dockerapi.Inspect{}, errors.New("no such container: " + id)
	}
	return v, nil
}
func (d *scriptedDocker) Stats(ctx context.Context, id string) (dockerapi.Stats, error) {
	d.mu.Lock()
	stats := d.stats
	d.mu.Unlock()
	if stats != nil {
		return stats(ctx, id)
	}
	return dockerapi.Stats{CPU: dockerapi.CPUStats{TotalUsage: 100, SystemUsage: 1000, OnlineCPUs: 2}, Memory: dockerapi.MemoryStats{Usage: 100, Limit: 1000}}, nil
}
func (d *scriptedDocker) Events(ctx context.Context) <-chan dockerapi.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subscriptions = append(d.subscriptions, time.Now())
	if len(d.streams) > 0 {
		next := d.streams[0]
		d.streams = d.streams[1:]
		return next
	}
	return make(chan dockerapi.Event)
}
func (d *scriptedDocker) Version(context.Context) (dockerapi.Version, error) {
	return dockerapi.Version{APIVersion: "1.47"}, nil
}
func (d *scriptedDocker) Diagnostics(context.Context) (dockerapi.Diagnostics, error) {
	return dockerapi.Diagnostics{}, nil
}

func newStateSampler(docker *scriptedDocker) *Sampler {
	return &Sampler{Docker: docker, Cache: dockercollector.NewCache()}
}

var stateEpoch = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// tickAt runs one Docker tick and returns the component of id.
func tickAt(t *testing.T, s *Sampler, at time.Time, id string) (metrics.ResourceComponent, []metrics.ResourceSnapshot) {
	t.Helper()
	resources, _, err := s.collectDocker(context.Background(), at, nil)
	if err != nil {
		t.Fatalf("collectDocker: %v", err)
	}
	for _, r := range resources {
		for _, c := range r.Components {
			if string(c.ID) == id {
				return c, resources
			}
		}
	}
	return metrics.ResourceComponent{}, resources
}

// COL-4: Docker's action is "health_status: unhealthy"; the health must come
// from the suffix, without waiting for an inspect.
func TestHealthStatusEventIsMatchedByPrefix(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   metrics.ResourceStatus
		health string
	}{
		{"health_status: unhealthy", metrics.StatusDown, "unhealthy"},
		{"health_status: starting", metrics.StatusUnknown, "starting"},
		{"health_status:healthy", metrics.StatusHealthy, "healthy"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			docker := newScriptedDocker()
			start := "healthy"
			if tc.health == "healthy" {
				start = "unhealthy"
			}
			docker.add("c1", "web", "running", start)
			s := newStateSampler(docker)
			tickAt(t, s, stateEpoch, "c1")
			s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: tc.action})
			got, _ := tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
			if got.HealthStatus != tc.health || got.Status != tc.want {
				t.Fatalf("after %q: health=%q status=%q, want %q %q", tc.action, got.HealthStatus, got.Status, tc.health, tc.want)
			}
			if n := docker.calls("c1"); n != 1 {
				t.Fatalf("inspect calls=%d, want 1: the health comes from the event", n)
			}
		})
	}
}

// COL-4: die, stop and kill mean the container exited, which maps to down,
// and no rate may be computed across the stop.
func TestStopEventsMapToExited(t *testing.T) {
	for _, action := range []string{"die", "stop", "kill"} {
		t.Run(action, func(t *testing.T) {
			docker := newScriptedDocker()
			docker.add("c1", "web", "running", "")
			s := newStateSampler(docker)
			tickAt(t, s, stateEpoch, "c1")
			if _, ok := s.previousStats["c1"]; !ok {
				t.Fatal("no previous stats after the first tick")
			}
			s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: action})
			cached, _ := s.Cache.Get("c1")
			if cached.State != "exited" {
				t.Fatalf("cached state after %s=%q, want exited", action, cached.State)
			}
			if _, ok := s.previousStats["c1"]; ok {
				t.Fatalf("previous stats kept after %s", action)
			}
			docker.setState("c1", "exited")
			got, _ := tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
			if got.Status != metrics.StatusDown || got.RuntimeState != "exited" {
				t.Fatalf("after %s: status=%q state=%q, want down exited", action, got.Status, got.RuntimeState)
			}
			if n := docker.calls("c1"); n != 1 {
				t.Fatalf("inspect calls=%d, want 1: the listed state matches the event", n)
			}
		})
	}
}

// COL-4: with every event lost, the listed state still reaches the published
// component within one tick, and a state change re-inspects the container.
func TestListStateWinsWithoutEvents(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "healthy")
	s := newStateSampler(docker)
	got, _ := tickAt(t, s, stateEpoch, "c1")
	if got.Status != metrics.StatusHealthy {
		t.Fatalf("initial status=%q", got.Status)
	}
	docker.setState("c1", "exited")
	got, _ = tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
	if got.Status != metrics.StatusDown || got.RuntimeState != "exited" {
		t.Fatalf("after exit without events: status=%q state=%q", got.Status, got.RuntimeState)
	}
	if n := docker.calls("c1"); n != 2 {
		t.Fatalf("inspect calls=%d, want 2: a changed listed state re-inspects", n)
	}
	if _, ok := s.previousStats["c1"]; ok {
		t.Fatal("previous stats kept for a container that is not running")
	}
	docker.setState("c1", "running")
	got, _ = tickAt(t, s, stateEpoch.Add(4*time.Second), "c1")
	if got.Status != metrics.StatusHealthy || docker.calls("c1") != 3 {
		t.Fatalf("after restart: status=%q inspects=%d", got.Status, docker.calls("c1"))
	}
	// Unchanged state: the cache is used.
	tickAt(t, s, stateEpoch.Add(6*time.Second), "c1")
	if n := docker.calls("c1"); n != 3 {
		t.Fatalf("inspect calls=%d after an unchanged tick, want 3", n)
	}
}

// The listed state overrides a stale inspected one: a container listed as
// paused is published paused even if inspect still said running.
func TestListedStateOverridesInspect(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.list[0].State = "paused"
	s := newStateSampler(docker)
	got, _ := tickAt(t, s, stateEpoch, "c1")
	if got.Status != metrics.StatusPaused || got.RuntimeState != "paused" {
		t.Fatalf("status=%q state=%q, want paused", got.Status, got.RuntimeState)
	}
}

// Lifecycle events that can change metadata mark the container for one
// re-inspect; pause sets the state directly.
func TestLifecycleEventsMarkForReinspect(t *testing.T) {
	for _, action := range []string{"start", "unpause", "restart", "rename", "update", "health_status"} {
		t.Run(action, func(t *testing.T) {
			docker := newScriptedDocker()
			docker.add("c1", "web", "running", "")
			s := newStateSampler(docker)
			tickAt(t, s, stateEpoch, "c1")
			s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: action})
			tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
			tickAt(t, s, stateEpoch.Add(4*time.Second), "c1")
			if n := docker.calls("c1"); n != 2 {
				t.Fatalf("inspect calls after %s=%d, want exactly one re-inspect", action, n)
			}
		})
	}
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: "pause"})
	if cached, _ := s.Cache.Get("c1"); cached.State != "paused" {
		t.Fatalf("cached state after pause=%q", cached.State)
	}
}

func TestMetadataReinspectedWhenOld(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	tickAt(t, s, stateEpoch.Add(metadataMaxAge-time.Second), "c1")
	if n := docker.calls("c1"); n != 1 {
		t.Fatalf("inspect calls before max age=%d", n)
	}
	tickAt(t, s, stateEpoch.Add(metadataMaxAge), "c1")
	if n := docker.calls("c1"); n != 2 {
		t.Fatalf("inspect calls at max age=%d, want 2", n)
	}
}

// COL-8: containers gone from the list leave neither the metadata cache nor
// previousStats, with or without a destroy event.
func TestPerContainerStatePrunedAfterList(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.add("c2", "worker", "running", "")
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	docker.remove("c2")
	tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
	if _, ok := s.Cache.Get("c2"); ok {
		t.Fatal("metadata cache kept a container that is no longer listed")
	}
	if _, ok := s.previousStats["c2"]; ok {
		t.Fatal("previousStats kept a container that is no longer listed")
	}
	if _, ok := s.Cache.Get("c1"); !ok || len(s.previousStats) != 1 {
		t.Fatalf("listed container pruned: cache ok=%v stats=%d", ok, len(s.previousStats))
	}
}

func TestDestroyEventPrunesBothMaps(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	event, accepted := s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: "destroy"})
	if _, ok := s.Cache.Get("c1"); ok {
		t.Fatal("destroy kept the cache entry")
	}
	if _, ok := s.previousStats["c1"]; ok {
		t.Fatal("destroy kept previousStats")
	}
	if !accepted || event.ResourceID == "" {
		t.Fatalf("destroy event=%+v accepted=%v, want the resource id resolved before removal", event, accepted)
	}
}

// EVT-3: Docker events carry the resource id the container is published
// under, so restart-storm and OOM-loop counters can group by it.
func TestDockerEventCarriesResourceID(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	s := newStateSampler(docker)
	_, published := tickAt(t, s, stateEpoch, "c1")
	if len(published) != 1 {
		t.Fatalf("resources=%d", len(published))
	}
	want := published[0].ID
	for _, action := range []string{"restart", "oom", "die", "start"} {
		event, accepted := s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c1", Action: action, Time: stateEpoch.Format(time.RFC3339)})
		if !accepted || event.ResourceID != want {
			t.Fatalf("%s event resource=%q accepted=%v, want %q", action, event.ResourceID, accepted, want)
		}
	}
	// A container seen first through an event is inspected once and lands
	// on the resource it will be published under.
	docker.add("c2", "web-2", "running", "")
	docker.inspects["c2"] = dockerapi.Inspect{ID: "c2", Name: "web-2", State: "running", Labels: docker.inspects["c1"].Labels}
	event, _ := s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "c2", Action: "start"})
	if event.ResourceID != want || docker.calls("c2") != 1 {
		t.Fatalf("uncached start resource=%q inspects=%d, want %q after one inspect", event.ResourceID, docker.calls("c2"), want)
	}
	// Unresolvable: published without a resource rather than dropped.
	event, accepted := s.handleDockerEvent(context.Background(), dockerapi.Event{ID: "gone", Action: "die"})
	if !accepted || event.ResourceID != "" {
		t.Fatalf("unknown container event=%+v accepted=%v", event, accepted)
	}
}

// EVT-3 through the real loop: an event received between ticks is published
// with the next snapshot and carries its resource id.
func TestRunPublishesEventsWithResourceID(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	stream := make(chan dockerapi.Event, 1)
	docker.streams = []chan dockerapi.Event{stream}
	engine := metrics.NewEngine(16)
	s := &Sampler{Engine: engine, Docker: docker, Cache: dockercollector.NewCache(), HostProc: t.TempDir(), Interval: func() time.Duration { return time.Second }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return len(engine.Snapshot().Resources) == 1 })
	want := engine.Snapshot().Resources[0].ID
	stream <- dockerapi.Event{ID: "c1", Action: "restart", Time: time.Now().UTC().Format(time.RFC3339)}
	waitFor(t, 3*time.Second, func() bool { return len(engine.EventsAfter(0)) > 0 })
	got := engine.EventsAfter(0)[0]
	if got.Type != "container_restart" || got.ResourceID != want {
		t.Fatalf("published event=%+v, want container_restart on %q", got, want)
	}
}

// COL-4: a closed events stream is resubscribed with growing delays, the
// disconnect is visible, and the next tick re-inspects every container.
func TestEventsResubscribeWithBackoffAndRediscover(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.add("c2", "worker", "running", "")
	closed := func() chan dockerapi.Event { c := make(chan dockerapi.Event); close(c); return c }
	docker.streams = []chan dockerapi.Event{closed(), closed(), closed(), closed()}
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	base := 20 * time.Millisecond
	s.eventsBackoff = &[3]time.Duration{base, time.Second, time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue := make(chan dockerapi.Event, 4)
	done := make(chan struct{})
	go func() { s.watchEvents(ctx, queue); close(done) }()
	waitFor(t, 5*time.Second, func() bool {
		docker.mu.Lock()
		defer docker.mu.Unlock()
		return len(docker.subscriptions) == 5
	})
	cancel()
	<-done

	docker.mu.Lock()
	times := append([]time.Time(nil), docker.subscriptions...)
	docker.mu.Unlock()
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		floor := time.Duration(float64(base<<(i-1)) * 0.8)
		if gap < floor {
			t.Fatalf("resubscription %d after %v, want at least %v (backoff doubles)", i, gap, floor)
		}
	}
	if s.eventsDisconnected.Load() {
		t.Fatal("events still reported disconnected after a stream stayed open")
	}
	if !s.rediscover.Load() {
		t.Fatal("resubscription did not ask for a re-discover")
	}
	tickAt(t, s, stateEpoch.Add(2*time.Second), "c1")
	if docker.calls("c1") != 2 || docker.calls("c2") != 2 {
		t.Fatalf("inspects after reconnect c1=%d c2=%d, want every container re-inspected", docker.calls("c1"), docker.calls("c2"))
	}
	tickAt(t, s, stateEpoch.Add(4*time.Second), "c1")
	if docker.calls("c1") != 2 {
		t.Fatal("re-discover repeated on the following tick")
	}
}

func TestEventsDisconnectedReason(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	engine := metrics.NewEngine(16)
	s := &Sampler{Engine: engine, Docker: docker, Cache: dockercollector.NewCache(), HostProc: t.TempDir()}
	s.eventsDisconnected.Store(true)
	s.hostTick()
	s.dockerTick(context.Background())
	if got := engine.Snapshot().Collectors["docker"]; got.Reason != "events disconnected" || got.State != metrics.CollectorHealthy {
		t.Fatalf("docker collector=%+v", got)
	}
}

func TestEnqueueEventDropsOldestAndAsksRediscover(t *testing.T) {
	s := &Sampler{}
	queue := make(chan dockerapi.Event, 2)
	for _, id := range []string{"a", "b", "c"} {
		s.enqueueEvent(queue, dockerapi.Event{ID: id, Action: "start"})
	}
	if s.eventsDropped.Load() != 1 || !s.rediscover.Load() {
		t.Fatalf("dropped=%d rediscover=%v", s.eventsDropped.Load(), s.rediscover.Load())
	}
	if first, second := <-queue, <-queue; first.ID != "b" || second.ID != "c" {
		t.Fatalf("queue kept %q %q, want the newest two", first.ID, second.ID)
	}
}

func TestJitterStaysWithinTwentyPercent(t *testing.T) {
	for range 1000 {
		if d := jitter(time.Second); d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("jitter(1s)=%v", d)
		}
	}
}

func waitFor(t *testing.T, limit time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
