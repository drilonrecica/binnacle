// SPDX-License-Identifier: AGPL-3.0-only

package alerts

import "time"

type State struct {
	Phase          Phase
	Since          time.Time
	LastNotifiedAt *time.Time
	CooldownUntil  *time.Time
}
type Transition struct {
	State                         State
	Triggered, Repeated, Resolved bool
}

// Advance is a deterministic, restart-safe alert lifecycle transition.
func Advance(now time.Time, previous State, failing, recovered, suppressed bool, rule Rule) Transition {
	if previous.Phase == "" {
		previous = State{Phase: Healthy, Since: now}
	}
	out := Transition{State: previous}
	switch previous.Phase {
	case Healthy:
		if failing {
			out.State = State{Phase: Pending, Since: now, CooldownUntil: previous.CooldownUntil}
		}
	case Pending:
		if !failing {
			out.State = State{Phase: Healthy, Since: now, CooldownUntil: previous.CooldownUntil}
		} else if now.Sub(previous.Since) >= rule.TriggerDuration && !suppressed && (previous.CooldownUntil == nil || !now.Before(*previous.CooldownUntil)) {
			out.State.Phase = Firing
			out.State.Since = now
			out.State.LastNotifiedAt = &now
			out.Triggered = true
		}
	case Firing:
		if recovered {
			out.State.Phase = Recovering
			out.State.Since = now
		} else if !suppressed && previous.LastNotifiedAt != nil && rule.Repeat > 0 && now.Sub(*previous.LastNotifiedAt) >= rule.Repeat {
			out.State.LastNotifiedAt = &now
			out.Repeated = true
		}
	case Recovering:
		if failing {
			out.State.Phase = Firing
			out.State.Since = now
		} else if recovered && now.Sub(previous.Since) >= rule.RecoveryDuration {
			cooldown := now.Add(rule.Cooldown)
			out.State = State{Phase: Healthy, Since: now, CooldownUntil: &cooldown}
			out.Resolved = true
		}
	}
	return out
}
