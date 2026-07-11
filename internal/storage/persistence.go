// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/talos/internal/metrics"
)

// Persistence schedules immutable current snapshots into a bounded writer
// queue. Storage failure never blocks collectors or the live Metrics Engine.
type Persistence struct {
	Engine     *metrics.Engine
	Store      *Manager
	Interval   time.Duration
	QueueLimit int
	Dropped    atomic.Uint64
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewPersistence(engine *metrics.Engine, store *Manager, interval time.Duration, limit int) *Persistence {
	return &Persistence{Engine: engine, Store: store, Interval: interval, QueueLimit: limit}
}

func (p *Persistence) Start(parent context.Context) error {
	if p.Interval <= 0 {
		p.Interval = 10 * time.Second
	}
	if p.QueueLimit < 1 {
		p.QueueLimit = 60
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	queue := make(chan metrics.PersistenceBatch, p.QueueLimit)
	enqueue := func() {
		batch := p.Engine.PersistenceBatch()
		if batch.Snapshot.Sequence == 0 {
			return
		}
		select {
		case queue <- batch:
		default:
			select {
			case <-queue:
				p.Dropped.Add(1)
			default:
				{
				}
			}
			select {
			case queue <- batch:
			default:
				p.Dropped.Add(1)
			}
		}
	}
	p.wg.Add(2)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		enqueue()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				enqueue()
			}
		}
	}()
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case batch := <-queue:
				_ = Retry(ctx, func() error { return p.Store.WriteBatch(ctx, batch) })
			}
		}
	}()
	return nil
}
func (p *Persistence) Stop(context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	return nil
}
