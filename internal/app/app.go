// SPDX-License-Identifier: AGPL-3.0-only

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ShutdownBudget is the maximum time allotted to the complete shutdown.
const ShutdownBudget = 15 * time.Second

// State describes the externally visible lifecycle state.
type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
)

// Component is a long-lived service owned by the application.
type Component interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// ComponentFuncs adapts functions to Component.
type ComponentFuncs struct {
	StartFunc func(context.Context) error
	StopFunc  func(context.Context) error
}

func (f ComponentFuncs) Start(ctx context.Context) error {
	if f.StartFunc == nil {
		return nil
	}
	return f.StartFunc(ctx)
}
func (f ComponentFuncs) Stop(ctx context.Context) error {
	if f.StopFunc == nil {
		return nil
	}
	return f.StopFunc(ctx)
}

// Application starts components in registration order and stops them in reverse.
type Application struct {
	log        *slog.Logger
	components []Component
	mu         sync.RWMutex
	state      State
	failed     chan error
}

func New(log *slog.Logger, components ...Component) *Application {
	if log == nil {
		log = slog.Default()
	}
	return &Application{log: log, components: components, state: StateStarting, failed: make(chan error, 1)}
}

// Add registers a component before Run is called.
func (a *Application) Add(components ...Component) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != StateStarting {
		panic("app: Add after startup")
	}
	a.components = append(a.components, components...)
}

// Fail reports that a running component can no longer serve. Run then stops
// every component and returns err, so the process exits non-zero instead of
// running on without, for example, its HTTP listener. Only the first failure
// is kept; later ones are logged.
func (a *Application) Fail(err error) {
	if err == nil {
		return
	}
	select {
	case a.failed <- err:
	default:
		a.log.Error("additional component failure", "error", err)
	}
}

func (a *Application) State() State         { a.mu.RLock(); defer a.mu.RUnlock(); return a.state }
func (a *Application) setState(state State) { a.mu.Lock(); a.state = state; a.mu.Unlock() }

// Run blocks until ctx is cancelled, then stops all started components.
func (a *Application) Run(ctx context.Context) error {
	started := make([]Component, 0, len(a.components))
	for _, c := range a.components {
		if err := c.Start(ctx); err != nil {
			a.log.Error("component startup failed", "error", err)
			a.setState(StateStopping)
			return errors.Join(fmt.Errorf("start component: %w", err), a.stop(started))
		}
		started = append(started, c)
	}
	a.setState(StateRunning)
	a.log.Info("application started")
	var failure error
	select {
	case <-ctx.Done():
		a.log.Info("application stopping", "reason", ctx.Err())
	case failure = <-a.failed:
		a.log.Error("component failed; application stopping", "error", failure)
	}
	a.setState(StateStopping)
	return errors.Join(failure, a.stop(started))
}

func (a *Application) stop(started []Component) error {
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownBudget)
	defer cancel()
	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		if err := started[i].Stop(ctx); err != nil {
			a.log.Error("component shutdown failed", "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
