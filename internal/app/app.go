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

// Shutdown budgets (RUN-1). Docker sends SIGKILL 10 s after SIGTERM by
// default, so the whole shutdown is capped at ShutdownBudget and every
// component gets at most its own budget out of what is left. A component
// declares a budget with StopBudgeter; others get DefaultStopBudget.
const (
	ShutdownBudget    = 9 * time.Second
	DefaultStopBudget = 1 * time.Second
)

// StopBudgeter is implemented by components that need more (or less) than
// DefaultStopBudget to stop, for example to drain a queue.
type StopBudgeter interface {
	StopBudget() time.Duration
}

// Namer is implemented by components that want a readable name in shutdown
// logs; others are named by their type.
type Namer interface {
	Name() string
}

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

// ComponentFuncs adapts functions to Component. Label and Budget are
// optional; see Namer and StopBudgeter.
type ComponentFuncs struct {
	Label     string
	Budget    time.Duration
	StartFunc func(context.Context) error
	StopFunc  func(context.Context) error
}

func (f ComponentFuncs) Name() string {
	if f.Label == "" {
		return "component"
	}
	return f.Label
}
func (f ComponentFuncs) StopBudget() time.Duration {
	if f.Budget <= 0 {
		return DefaultStopBudget
	}
	return f.Budget
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
	// budget overrides ShutdownBudget in tests.
	budget time.Duration
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

// Run starts every component, blocks until ctx is cancelled or a component
// fails, then stops the started components in reverse order.
//
// Components receive a context that the signal context does not cancel once
// startup has finished: each component ends its own work in Stop, in order,
// so the HTTP server stops before the collectors and persistence drains
// before the database closes (RUN-1). A cancellation that arrives during
// startup still cancels the start context, so a long migration is not waited
// for. The run context is cancelled after the last Stop as a backstop.
func (a *Application) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRun()
	detach := context.AfterFunc(ctx, cancelRun)
	started := make([]Component, 0, len(a.components))
	for _, c := range a.components {
		if err := c.Start(runCtx); err != nil {
			detach()
			a.log.Error("component startup failed", "component", componentName(c), "error", err)
			a.setState(StateStopping)
			return errors.Join(fmt.Errorf("start component: %w", err), a.stop(started))
		}
		started = append(started, c)
	}
	detach()
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

// stop stops the started components in reverse order. Each Stop gets the
// smaller of its own budget and what is left of ShutdownBudget; a Stop that
// overruns is abandoned and logged so one stuck component cannot hold the
// rest past Docker's grace period.
func (a *Application) stop(started []Component) error {
	began := time.Now()
	overall, cancel := context.WithTimeout(context.Background(), a.shutdownBudget())
	defer cancel()
	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		c := started[i]
		name := componentName(c)
		ctx, cancelComponent := context.WithTimeout(overall, componentBudget(c))
		stoppedAt := time.Now()
		done := make(chan error, 1)
		go func() { done <- c.Stop(ctx) }()
		var err error
		select {
		case err = <-done:
		case <-ctx.Done():
			// Give a Stop that honours ctx a moment to return its own error.
			select {
			case err = <-done:
			case <-time.After(10 * time.Millisecond):
				err = fmt.Errorf("stop %s: did not finish within its budget: %w", name, ctx.Err())
			}
		}
		cancelComponent()
		if err != nil {
			a.log.Error("component shutdown failed", "component", name, "error", err, "elapsed", time.Since(stoppedAt))
			errs = append(errs, err)
		}
	}
	a.log.Info("application stopped", "elapsed", time.Since(began))
	return errors.Join(errs...)
}

func (a *Application) shutdownBudget() time.Duration {
	if a.budget > 0 {
		return a.budget
	}
	return ShutdownBudget
}

func componentBudget(c Component) time.Duration {
	if b, ok := c.(StopBudgeter); ok && b.StopBudget() > 0 {
		return b.StopBudget()
	}
	return DefaultStopBudget
}

func componentName(c Component) string {
	if n, ok := c.(Namer); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", c)
}
