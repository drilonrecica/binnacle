// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"context"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

type Component struct {
	Generator *Generator
	Engine    *metrics.Engine
	Interval  time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func (c *Component) Start(ctx context.Context) error {
	if c.Interval <= 0 {
		c.Interval = 2 * time.Second
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		tick := uint64(0)
		t := time.NewTicker(c.Interval)
		defer t.Stop()
		for {
			now := c.Generator.clock.Now().UTC()
			c.Engine.PublishFilesystems(c.Generator.FilesystemsAt(tick, now))
			c.Engine.Publish(c.Generator.SnapshotAt(tick, now), c.Generator.Events(tick)...)
			tick++
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return nil
}
func (c *Component) Name() string { return "demo" }
func (c *Component) Stop(context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}
