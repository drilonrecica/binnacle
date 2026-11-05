// SPDX-License-Identifier: AGPL-3.0-only

// Package estimator computes time-to-full estimates of each filesystem and of
// host memory from their recent history (SPEC §P3-12).
package estimator

import (
	"math"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const (
	// MinHistory is the shortest span of samples that gives an estimate.
	MinHistory = 3 * time.Hour
	// steadyFraction is the projected change over the window, as a share of
	// capacity, below which a series is steady.
	steadyFraction = 0.01
	// maxDaysToFull caps a projection so that a near-flat slope that still
	// counts as filling cannot overflow a date.
	maxDaysToFull = 100 * 365
)

// Estimate fits a least-squares line to samples' used amounts over time and
// projects it to capacity.
//
//   - unknown: the samples span less than MinHistory, or capacity is not
//     positive; confidence is 0.
//   - steady: the slope changes usage by less than 1 % of capacity over the
//     window.
//   - shrinking: the slope is negative.
//   - filling: otherwise; DaysToFull is the time for current to reach
//     capacity at the slope, 0 when it already has.
//
// Confidence is 1 minus the normalised residual, clamped to [0, 1]: the
// residuals' root mean square divided by the larger of the projected change
// over the window and the steady band (1 % of capacity). A clean line scores
// near 1; noise as large as the trend scores 0.
func Estimate(samples []storage.UsageSample, current, capacity float64, window time.Duration, now time.Time) metrics.Trend {
	hours := int(window / time.Hour)
	trend := metrics.UnknownTrend(hours)
	if len(samples) < 2 || capacity <= 0 || samples[len(samples)-1].At.Sub(samples[0].At) < MinHistory {
		return trend
	}
	// Two passes with centred values: bytes are ~1e12, and summing their
	// squares directly would lose the residual to rounding.
	origin := samples[0].At
	n := float64(len(samples))
	var meanX, meanY float64
	for _, s := range samples {
		meanX += s.At.Sub(origin).Hours()
		meanY += s.Used
	}
	meanX /= n
	meanY /= n
	var sxx, sxy float64
	for _, s := range samples {
		dx := s.At.Sub(origin).Hours() - meanX
		sxx += dx * dx
		sxy += dx * (s.Used - meanY)
	}
	if sxx == 0 {
		return trend
	}
	slope := sxy / sxx // per hour
	var sse float64
	for _, s := range samples {
		residual := s.Used - (meanY + slope*(s.At.Sub(origin).Hours()-meanX))
		sse += residual * residual
	}
	change := math.Abs(slope) * window.Hours()
	band := steadyFraction * capacity
	trend.Confidence = clamp(1 - math.Sqrt(sse/n)/math.Max(change, band))
	switch {
	case change < band:
		trend.State = metrics.TrendSteady
	case slope < 0:
		trend.State = metrics.TrendShrinking
	default:
		trend.State = metrics.TrendFilling
		days := math.Min(math.Max(capacity-current, 0)/slope/24, maxDaysToFull)
		at := now.Add(time.Duration(days * 24 * float64(time.Hour))).UTC().Truncate(time.Second)
		trend.DaysToFull, trend.FullAt = &days, &at
	}
	return trend
}

func clamp(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	return math.Min(v, 1)
}
