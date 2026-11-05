// SPDX-License-Identifier: AGPL-3.0-only
package docker

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

// Metadata is the cached result of one Inspect. State is overwritten with the
// container list's state on every tick and with lifecycle events in between;
// Health comes from Inspect and from health_status events (ADR 022 §2, §5).
type Metadata struct {
	ID, Name, Image, Created, State, Health string
	Labels                                  map[string]string
	Environment                             map[string]string
	Networks                                []string
	Mounts                                  []dockerapi.Mount
	// InspectedAt is when the metadata was last inspected; the sampler
	// re-inspects entries older than its maximum age.
	InspectedAt time.Time
	// Reinspect is set by an event that may have changed the metadata; the
	// next tick inspects the container again and clears it.
	Reinspect bool
}

// MetadataFromInspect copies an Inspect result into cacheable metadata.
func MetadataFromInspect(v dockerapi.Inspect, at time.Time) Metadata {
	return Metadata{ID: v.ID, Name: v.Name, Image: v.Image, Created: v.Created, State: v.State, Health: v.Health, Labels: v.Labels, Environment: v.Environment, Networks: v.Networks, Mounts: v.Mounts, InspectedAt: at}
}

// Inspect converts cached metadata back into the Inspect shape.
func (m Metadata) Inspect() dockerapi.Inspect {
	return dockerapi.Inspect{ID: m.ID, Name: m.Name, Image: m.Image, Created: m.Created, State: m.State, Health: m.Health, Labels: m.Labels, Environment: m.Environment, Networks: m.Networks, Mounts: m.Mounts}
}

type Cache struct {
	mu     sync.RWMutex
	values map[string]Metadata
}

func NewCache() *Cache { return &Cache{values: map[string]Metadata{}} }
func (c *Cache) Discover(ctx context.Context, client dockerapi.Client) error {
	list, e := client.List(ctx)
	if e != nil {
		return e
	}
	next := map[string]Metadata{}
	for _, item := range list {
		v, e := client.Inspect(ctx, item.ID)
		if e != nil {
			return e
		}
		next[item.ID] = clone(MetadataFromInspect(v, time.Now()))
	}
	c.mu.Lock()
	c.values = next
	c.mu.Unlock()
	return nil
}
func (c *Cache) Get(id string) (Metadata, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.values[id]
	return v, ok
}
func (c *Cache) Set(v Metadata) {
	c.mu.Lock()
	c.values[v.ID] = clone(v)
	c.mu.Unlock()
}
func (c *Cache) Remove(id string) { c.mu.Lock(); delete(c.values, id); c.mu.Unlock() }

// Retain removes every entry whose id is not in keep, so the cache holds only
// containers that currently exist (COL-8). It returns the number removed.
func (c *Cache) Retain(keep map[string]struct{}) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for id := range c.values {
		if _, ok := keep[id]; !ok {
			delete(c.values, id)
			removed++
		}
	}
	return removed
}

// Len reports the number of cached containers.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.values)
}

// EventEffect tells the caller what an applied event means beyond the cache.
type EventEffect struct {
	// DropStats: the previous stats sample of the container must be dropped
	// so no rate is computed across the transition.
	DropStats bool
	// Removed: the container no longer exists.
	Removed bool
}

// Apply updates the cache for one Docker event (ADR 022 §5). Events for a
// container that is not cached change nothing in the cache; the next tick's
// list and inspect pick it up.
func (c *Cache) Apply(e dockerapi.Event) EventEffect {
	action := strings.ToLower(e.Action)
	if action == "destroy" {
		c.Remove(e.ID)
		return EventEffect{DropStats: true, Removed: true}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, cached := c.values[e.ID]
	effect := EventEffect{}
	switch action {
	case "die", "stop", "kill":
		v.State = "exited"
		effect.DropStats = true
	case "pause":
		v.State = "paused"
	case "start", "unpause", "restart", "rename", "update":
		v.Reinspect = true
	default:
		health, ok := HealthFromAction(action)
		if !ok {
			return effect
		}
		if health == "" {
			v.Reinspect = true
		} else {
			v.Health = health
		}
	}
	if cached {
		c.values[e.ID] = v
	}
	return effect
}

// HealthFromAction parses Docker's health event action, which carries the new
// status after the name: "health_status: healthy". ok is false for any other
// action; health is empty when the action has no status suffix.
func HealthFromAction(action string) (health string, ok bool) {
	rest, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(action)), "health_status")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, ":")), true
}

func clone(v Metadata) Metadata {
	v.Labels = copyLabels(v.Labels)
	v.Environment = copyLabels(v.Environment)
	v.Networks = append([]string(nil), v.Networks...)
	v.Mounts = append([]dockerapi.Mount(nil), v.Mounts...)
	return v
}

func copyLabels(v map[string]string) map[string]string {
	o := map[string]string{}
	for k, x := range v {
		o[k] = x
	}
	return o
}
