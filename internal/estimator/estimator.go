// SPDX-License-Identifier: AGPL-3.0-only

package estimator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/observe"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const (
	// DefaultInterval is how often estimates are recomputed. Requests read
	// the engine's copy and never trigger a computation.
	DefaultInterval = 5 * time.Minute
	// defaultDelay is the wait before the first run, so that the collectors
	// have published capacities and the memory total.
	defaultDelay = 15 * time.Second
	// runTimeout bounds one run's queries.
	runTimeout = 30 * time.Second
	// maxFilesystems bounds the mounts estimated per run.
	maxFilesystems = 64
	// MinWindowHours and MaxWindowHours bound estimator.window_hours.
	MinWindowHours = 6
	MaxWindowHours = 168
)

// Source is the usage history the estimator reads.
type Source interface {
	FilesystemUsage(ctx context.Context, mountKey string, from, to time.Time) ([]storage.UsageSample, error)
	MemoryUsage(ctx context.Context, from, to time.Time) ([]storage.UsageSample, error)
}

// Estimator recomputes the engine's time-to-full estimates every Interval.
type Estimator struct {
	Engine *metrics.Engine
	Source Source
	// WindowHours returns the configured window; nil or out of range means
	// metrics.DefaultTrendWindowHours.
	WindowHours func() int
	Interval    time.Duration
	Delay       time.Duration
	Now         func() time.Time
	Logger      *slog.Logger

	cancel     context.CancelFunc
	wg         sync.WaitGroup
	wakeOnce   sync.Once
	wake       chan struct{}
	errlog     observe.ErrorLog
	errlogOnce sync.Once
}

func (e *Estimator) Name() string { return "estimator" }

func (e *Estimator) wakeChan() chan struct{} {
	e.wakeOnce.Do(func() { e.wake = make(chan struct{}, 1) })
	return e.wake
}

// Wake asks for a run now, for example after the window setting changed.
func (e *Estimator) Wake() {
	select {
	case e.wakeChan() <- struct{}{}:
	default:
	}
}

func (e *Estimator) Start(ctx context.Context) error {
	if e.Engine == nil || e.Source == nil {
		return errors.New("estimator dependencies unavailable")
	}
	interval, delay := e.Interval, e.Delay
	if interval <= 0 {
		interval = DefaultInterval
	}
	if delay <= 0 {
		delay = defaultDelay
	}
	ctx, e.cancel = context.WithCancel(ctx)
	wake := e.wakeChan()
	// A wake from before Start (settings are applied once at boot) would
	// run before the collectors have published; the first run waits for
	// the delay instead.
	select {
	case <-wake:
	default:
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			case <-wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			if err := e.Run(ctx); err != nil && ctx.Err() == nil {
				e.errorLog().Error("time-to-full estimation failed", "error", err)
			}
			timer.Reset(interval)
		}
	}()
	return nil
}

func (e *Estimator) Stop(context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	return nil
}

func (e *Estimator) errorLog() *observe.ErrorLog {
	e.errlogOnce.Do(func() {
		e.errlog.Logger = func() *slog.Logger {
			if e.Logger != nil {
				return e.Logger
			}
			return slog.Default()
		}
	})
	return &e.errlog
}

func (e *Estimator) window() int {
	if e.WindowHours != nil {
		if h := e.WindowHours(); h >= MinWindowHours && h <= MaxWindowHours {
			return h
		}
	}
	return metrics.DefaultTrendWindowHours
}

// Run computes every estimate once and hands them to the engine. A series
// whose history cannot be read is estimated as unknown and the error is
// returned after the others are published.
func (e *Estimator) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}
	hours := e.window()
	window := time.Duration(hours) * time.Hour
	from := now.Add(-window)
	snap := e.Engine.Snapshot()
	var errs []error

	memory := metrics.UnknownTrend(hours)
	if snap.Host.MemoryTotalBytes != nil {
		samples, err := e.Source.MemoryUsage(ctx, from, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("memory history: %w", err))
		} else {
			current := lastUsed(samples)
			if snap.Host.MemoryUsedBytes != nil {
				current = float64(*snap.Host.MemoryUsedBytes)
			}
			memory = Estimate(samples, current, float64(*snap.Host.MemoryTotalBytes), window, now)
		}
	}

	filesystems := map[string]metrics.Trend{}
	for i, fs := range snap.Filesystems {
		if i >= maxFilesystems {
			break
		}
		samples, err := e.Source.FilesystemUsage(ctx, fs.MountKey, from, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("filesystem %s history: %w", fs.MountKey, err))
			continue
		}
		current, capacity := filesystemNow(fs, samples)
		filesystems[fs.MountKey] = Estimate(samples, current, capacity, window, now)
	}
	e.Engine.SetTrends(hours, memory, filesystems)
	return errors.Join(errs...)
}

// filesystemNow returns the current used bytes and capacity of fs, from the
// live observation when it has them and from the newest bucket otherwise.
func filesystemNow(fs metrics.FilesystemObservation, samples []storage.UsageSample) (float64, float64) {
	current, capacity := lastUsed(samples), 0.0
	if n := len(samples); n > 0 {
		capacity = samples[n-1].Capacity
	}
	if fs.UsedBytes != nil {
		current = float64(*fs.UsedBytes)
		switch {
		case fs.AvailableBytes != nil:
			capacity = float64(*fs.UsedBytes + *fs.AvailableBytes)
		case fs.TotalBytes != nil:
			capacity = float64(*fs.TotalBytes)
		}
	}
	return current, capacity
}

func lastUsed(samples []storage.UsageSample) float64 {
	if n := len(samples); n > 0 {
		return samples[n-1].Used
	}
	return 0
}
