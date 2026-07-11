// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"context"
	"sync"
)

// Client is deliberately read-only; mutation operations are not part of TALOS's boundary.
type Client interface {
	List(context.Context) ([]Container, error)
	Inspect(context.Context, string) (Inspect, error)
	Stats(context.Context, string) (Stats, error)
	Events(context.Context) <-chan Event
	Version(context.Context) (Version, error)
	Diagnostics(context.Context) (Diagnostics, error)
}
type Container struct{ ID, Name, Image string }
type Inspect struct {
	ID, Name, Image, Created, State, Health string
	Labels                                  map[string]string
	Networks                                []string
	Mounts                                  []Mount
}
type Mount struct{ Source, Destination, Type string }
type Event struct{ ID, Action, Time string }
type Version struct{ APIVersion string }
type Diagnostics struct{ Containers int }
type Stats struct{}
type Limited struct {
	Client Client
	sem    chan struct{}
	once   sync.Once
}

func New(client Client, max int) *Limited {
	if max < 1 {
		max = 1
	}
	return &Limited{Client: client, sem: make(chan struct{}, max)}
}
func (l *Limited) with(ctx context.Context, fn func() error) error {
	select {
	case l.sem <- struct{}{}:
		defer func() { <-l.sem }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
