// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import "time"

// TrendState is the direction of a time-to-full estimate.
type TrendState string

const (
	TrendSteady    TrendState = "steady"
	TrendFilling   TrendState = "filling"
	TrendShrinking TrendState = "shrinking"
	TrendUnknown   TrendState = "unknown"
)

// DefaultTrendWindowHours is the estimator window when none is configured.
const DefaultTrendWindowHours = 24

// Trend is a time-to-full estimate (SPEC §P3-12). DaysToFull and FullAt are
// set only when State is TrendFilling. The pointers are never mutated after
// the estimate is made, so copies of a Trend may share them.
type Trend struct {
	State       TrendState `json:"state"`
	DaysToFull  *float64   `json:"daysToFull"`
	FullAt      *time.Time `json:"fullAt"`
	Confidence  float64    `json:"confidence"`
	WindowHours int        `json:"windowHours"`
}

// UnknownTrend is the estimate of a series without enough history.
func UnknownTrend(windowHours int) Trend {
	return Trend{State: TrendUnknown, WindowHours: windowHours}
}
