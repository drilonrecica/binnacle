// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dockercollector "github.com/drilonrecica/binnacle/internal/collector/docker"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func runningStats() dockerapi.Stats {
	return dockerapi.Stats{CPU: dockerapi.CPUStats{TotalUsage: 100, SystemUsage: 1000, OnlineCPUs: 2}, Memory: dockerapi.MemoryStats{Usage: 100, Limit: 1000}}
}

func componentOf(resources []metrics.ResourceSnapshot, id string) (metrics.ResourceSnapshot, metrics.ResourceComponent, bool) {
	for _, r := range resources {
		for _, c := range r.Components {
			if string(c.ID) == id {
				return r, c, true
			}
		}
	}
	return metrics.ResourceSnapshot{}, metrics.ResourceComponent{}, false
}

// COL-6: a Stats call that never answers is cut by its call timeout; only
// that container is unknown and the tick still completes.
func TestBlockedStatsSkipsOnlyThatContainer(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.add("c2", "worker", "running", "")
	docker.stats = func(ctx context.Context, id string) (dockerapi.Stats, error) {
		if id == "c2" {
			<-ctx.Done()
			return dockerapi.Stats{}, ctx.Err()
		}
		return runningStats(), nil
	}
	s := newStateSampler(docker)
	s.callTimeout = 50 * time.Millisecond
	started := time.Now()
	resources, reason, err := s.collectDocker(context.Background(), stateEpoch, nil)
	if err != nil {
		t.Fatalf("a blocked Stats failed the tick: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("tick took %v; the call timeout did not bound it", elapsed)
	}
	_, web, _ := componentOf(resources, "c1")
	if web.MemoryBytes == nil || web.Status != metrics.StatusHealthy {
		t.Fatalf("healthy container did not report: %+v", web)
	}
	_, worker, ok := componentOf(resources, "c2")
	if !ok || worker.Status != metrics.StatusUnknown || worker.MemoryBytes != nil || worker.CPUHostPercent != nil || worker.RuntimeState != "running" {
		t.Fatalf("blocked container component=%+v ok=%v, want unknown with nil metrics", worker, ok)
	}
	if want := "1 of 2 containers skipped: c2: context deadline exceeded"; reason != want {
		t.Fatalf("reason=%q want %q", reason, want)
	}
}

// COL-6: the tick deadline turns containers not reached in time into
// per-container failures and says so in the reason.
func TestTickDeadlineSkipsUnreachedContainers(t *testing.T) {
	docker := newScriptedDocker()
	for i := range 4 {
		docker.add(fmt.Sprintf("c%d", i), fmt.Sprintf("svc%d", i), "running", "")
	}
	docker.stats = func(ctx context.Context, _ string) (dockerapi.Stats, error) {
		<-ctx.Done()
		return dockerapi.Stats{}, ctx.Err()
	}
	s := newStateSampler(docker)
	s.MaxDockerConcurrency = 1
	s.callTimeout = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	resources, reason, err := s.collectDocker(ctx, stateEpoch, nil)
	if err != nil {
		t.Fatalf("tick failed on the deadline: %v", err)
	}
	// The inspects finished before the deadline; no Stats call did.
	if len(resources) != 4 || !strings.HasPrefix(reason, "4 of 4 containers skipped") || !strings.HasSuffix(reason, "; tick deadline reached") {
		t.Fatalf("resources=%d reason=%q", len(resources), reason)
	}
}

// COL-5: a container removed between List and Stats is skipped; the tick
// succeeds, its resource is still published and no rate spans the gap.
func TestStatsNotFoundSkipsContainer(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.add("c2", "worker", "running", "")
	s := newStateSampler(docker)
	tickAt(t, s, stateEpoch, "c1")
	if _, ok := s.previousStats["c2"]; !ok {
		t.Fatal("first tick did not keep a sample for c2")
	}
	docker.mu.Lock()
	docker.stats = func(_ context.Context, id string) (dockerapi.Stats, error) {
		if id == "c2" {
			return dockerapi.Stats{}, errors.New("Error response from daemon: No such container: c2")
		}
		return runningStats(), nil
	}
	docker.mu.Unlock()
	resources, reason, err := s.collectDocker(context.Background(), stateEpoch.Add(2*time.Second), nil)
	if err != nil {
		t.Fatalf("a 404 failed the tick: %v", err)
	}
	resource, worker, ok := componentOf(resources, "c2")
	if !ok || worker.Status != metrics.StatusUnknown || worker.MemoryBytes != nil {
		t.Fatalf("worker component=%+v ok=%v", worker, ok)
	}
	if resource.Stale || resource.LastSeenAt != stateEpoch.Add(2*time.Second) {
		t.Fatalf("resource of a skipped container=%+v, want published fresh", resource)
	}
	if _, ok := s.previousStats["c2"]; ok {
		t.Fatal("previous sample kept for a skipped container")
	}
	if !strings.HasPrefix(reason, "1 of 2 containers skipped: c2: ") {
		t.Fatalf("reason=%q", reason)
	}
}

// COL-5: an Inspect failure for a container without cached metadata omits it
// and counts it; it does not fail the tick.
func TestInspectFailureSkipsContainer(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.mu.Lock()
	docker.list = append(docker.list, dockerapi.Container{ID: "gone", Name: "gone", State: "running"})
	docker.mu.Unlock()
	s := newStateSampler(docker)
	resources, reason, err := s.collectDocker(context.Background(), stateEpoch, nil)
	if err != nil {
		t.Fatalf("an Inspect error failed the tick: %v", err)
	}
	if len(resources) != 1 || !strings.HasPrefix(reason, "1 of 2 containers skipped: gone: no such container") {
		t.Fatalf("resources=%d reason=%q", len(resources), reason)
	}
}

// COL-5: a List error is the only tick failure. The last resources are
// republished stale with nil metrics and their old LastSeenAt, and a
// persistence batch built from that snapshot stores no resource rows.
func TestListErrorPublishesStaleResourcesAndPersistsNone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	engine := metrics.NewEngine(16)
	s := &Sampler{Engine: engine, Docker: docker, Cache: dockercollector.NewCache(), HostProc: t.TempDir(), Store: store}
	s.hostTick()
	s.dockerTick(ctx)
	fresh := engine.Snapshot()
	if len(fresh.Resources) != 1 || fresh.Resources[0].MemoryBytes == nil {
		t.Fatalf("first tick resources=%+v", fresh.Resources)
	}
	first := engine.PersistenceBatch()
	if err := store.WriteBatch(ctx, first); err != nil {
		t.Fatal(err)
	}
	freshAt := fresh.Collectors["docker"].FreshAt

	docker.mu.Lock()
	docker.listErr = errors.New("daemon unavailable")
	docker.mu.Unlock()
	s.dockerTick(ctx)
	snapshot := engine.Snapshot()
	if len(snapshot.Resources) != 1 {
		t.Fatalf("resources after a List error=%d, want the last-known set", len(snapshot.Resources))
	}
	r := snapshot.Resources[0]
	if !r.Stale || r.Status != metrics.StatusUnknown || r.MemoryBytes != nil || r.CPUHostPercent != nil || !r.LastSeenAt.Equal(fresh.Resources[0].LastSeenAt) {
		t.Fatalf("republished resource=%+v, want stale with nil metrics and unchanged LastSeenAt", r)
	}
	if len(r.Components) != 1 || r.Components[0].Status != metrics.StatusUnknown || r.Components[0].MemoryBytes != nil {
		t.Fatalf("republished components=%+v", r.Components)
	}
	h := snapshot.Collectors["docker"]
	if h.Reason != "daemon unavailable" || !h.FreshAt.Equal(freshAt) {
		t.Fatalf("docker health=%+v, want the error and FreshAt of the last success", h)
	}

	batch := engine.PersistenceBatch()
	batch.Snapshot.At = first.Snapshot.At.Add(10 * time.Second)
	if err := store.WriteBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"resource_samples_10s", "container_instance_samples_10s"} {
		var rows int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE ts=?", batch.Snapshot.At.UnixMilli()).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Fatalf("%s has %d rows for the failed tick, want a gap", table, rows)
		}
	}
	var hostRows int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM host_samples_10s WHERE ts=?", batch.Snapshot.At.UnixMilli()).Scan(&hostRows); err != nil || hostRows != 1 {
		t.Fatalf("host rows=%d err=%v, want the host row still written", hostRows, err)
	}
}

// COL-6: never more Stats calls in flight than MaxDockerConcurrency.
func TestStatsConcurrencyIsBounded(t *testing.T) {
	docker := newScriptedDocker()
	for i := range 9 {
		docker.add(fmt.Sprintf("c%d", i), fmt.Sprintf("svc%d", i), "running", "")
	}
	var inFlight, peak atomic.Int32
	docker.stats = func(context.Context, string) (dockerapi.Stats, error) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return runningStats(), nil
	}
	s := newStateSampler(docker)
	s.MaxDockerConcurrency = 3
	if _, reason, err := s.collectDocker(context.Background(), stateEpoch, nil); err != nil || reason != "" {
		t.Fatalf("err=%v reason=%q", err, reason)
	}
	if got := peak.Load(); got > 3 || got < 2 {
		t.Fatalf("peak Stats in flight=%d, want concurrent but at most 3", got)
	}
}

// COL-6: the host loop keeps publishing while a Docker tick is stuck.
func TestHostLoopPublishesWhileDockerTickIsBlocked(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	release := make(chan struct{})
	docker.stats = func(ctx context.Context, _ string) (dockerapi.Stats, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return runningStats(), nil
	}
	proc, _ := hostProcFixture(t)
	engine := metrics.NewEngine(16)
	s := &Sampler{Engine: engine, Docker: docker, Cache: dockercollector.NewCache(), HostProc: proc, HostRoot: "/host/root", Interval: func() time.Duration { return time.Second }}
	s.callTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		snapshot := engine.Snapshot()
		_, docker := snapshot.Collectors["docker"]
		return snapshot.Host.MemoryUsedBytes != nil && !docker
	})
	first := engine.Snapshot().Host.At
	waitFor(t, 3*time.Second, func() bool { return engine.Snapshot().Host.At.After(first) })
	if _, ok := engine.Snapshot().Collectors["docker"]; ok {
		t.Fatal("docker tick finished although its Stats call was blocked")
	}
	close(release)
	waitFor(t, 3*time.Second, func() bool { return len(engine.Snapshot().Resources) == 1 })
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if stopCtx.Err() != nil {
		t.Fatal("loops did not stop within the deadline")
	}
}

// A skipped container is logged once per minute, not once per tick.
func TestSkippedContainerLogIsCoalesced(t *testing.T) {
	s := &Sampler{}
	containers := []dockerapi.Container{{ID: "c1"}}
	failures := map[string]error{"c1": errors.New("boom")}
	s.skipReason(context.Background(), containers, failures, stateEpoch)
	logged := s.skipLogged["c1"]
	s.skipReason(context.Background(), containers, failures, stateEpoch.Add(30*time.Second))
	if s.skipLogged["c1"] != logged {
		t.Fatal("skip logged again within a minute")
	}
	s.skipReason(context.Background(), containers, failures, stateEpoch.Add(time.Minute))
	if s.skipLogged["c1"] == logged {
		t.Fatal("skip not logged again after a minute")
	}
}

// RUN-3: before a supported Engine is verified the gate refuses List. The
// docker collector is degraded with the gate's reason from the first tick,
// not healthy-with-a-reason until three failures accumulate.
func TestUnverifiedEngineDegradesDockerFromFirstTick(t *testing.T) {
	docker := newScriptedDocker()
	docker.add("c1", "web", "running", "")
	docker.listErr = fmt.Errorf("%w: Docker Engine 28.0.0 is unsupported; version %s or newer is required", dockerapi.ErrEngineUnverified, dockerapi.MinimumEngineVersion)
	engine := metrics.NewEngine(16)
	s := &Sampler{Engine: engine, Docker: docker, Cache: dockercollector.NewCache(), HostProc: t.TempDir()}
	s.hostTick()
	s.dockerTick(context.Background())
	h := engine.Snapshot().Collectors["docker"]
	if h.State != metrics.CollectorDegraded || !strings.Contains(h.Reason, "28.0.0 is unsupported") {
		t.Fatalf("docker health = %+v, want degraded with the version reason", h)
	}
}
