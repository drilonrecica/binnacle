// SPDX-License-Identifier: AGPL-3.0-only
package estimator

import (
	"math"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const gib = float64(1 << 30)

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// history returns one sample every 15 minutes over span, ending at now, with
// used(hoursAgo) as the value.
func history(span time.Duration, used func(hoursAgo float64) float64) []storage.UsageSample {
	var out []storage.UsageSample
	for at := now.Add(-span); !at.After(now); at = at.Add(15 * time.Minute) {
		out = append(out, storage.UsageSample{At: at, Used: used(now.Sub(at).Hours())})
	}
	return out
}

func TestEstimateLinearFill(t *testing.T) {
	// 100 GiB disk, 60 GiB used now, growing 1 GiB per hour: full in 40 h.
	samples := history(24*time.Hour, func(h float64) float64 { return 60*gib - h*gib })
	got := Estimate(samples, 60*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendFilling || got.DaysToFull == nil || got.FullAt == nil {
		t.Fatalf("trend=%+v, want filling with a date", got)
	}
	if math.Abs(*got.DaysToFull-40.0/24) > 0.01 {
		t.Fatalf("daysToFull=%v, want %v", *got.DaysToFull, 40.0/24)
	}
	if want := now.Add(40 * time.Hour); got.FullAt.Sub(want).Abs() > time.Minute {
		t.Fatalf("fullAt=%v, want %v", got.FullAt, want)
	}
	if got.Confidence < 0.99 || got.WindowHours != 24 {
		t.Fatalf("confidence=%v windowHours=%d, want ~1 and 24", got.Confidence, got.WindowHours)
	}
}

func TestEstimateSteady(t *testing.T) {
	// 0.5 GiB of change per day on 100 GiB is under the 1 % band.
	samples := history(24*time.Hour, func(h float64) float64 { return 50*gib - h*0.5*gib/24 })
	got := Estimate(samples, 50*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendSteady || got.DaysToFull != nil || got.FullAt != nil {
		t.Fatalf("trend=%+v, want steady without a date", got)
	}
	if got.Confidence < 0.9 {
		t.Fatalf("confidence=%v, want a clean steady line to be confident", got.Confidence)
	}
}

func TestEstimateShrinking(t *testing.T) {
	samples := history(12*time.Hour, func(h float64) float64 { return 40*gib + h*gib })
	got := Estimate(samples, 40*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendShrinking || got.DaysToFull != nil {
		t.Fatalf("trend=%+v, want shrinking without a date", got)
	}
}

func TestEstimateNeedsThreeHours(t *testing.T) {
	samples := history(2*time.Hour+45*time.Minute, func(h float64) float64 { return 60*gib - h*gib })
	got := Estimate(samples, 60*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendUnknown || got.Confidence != 0 || got.DaysToFull != nil {
		t.Fatalf("trend=%+v, want unknown with confidence 0", got)
	}
	got = Estimate(history(3*time.Hour, func(h float64) float64 { return 60*gib - h*gib }), 60*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendFilling {
		t.Fatalf("three hours of history: trend=%+v, want filling", got)
	}
	if got := Estimate(nil, 0, 100*gib, 24*time.Hour, now); got.State != metrics.TrendUnknown {
		t.Fatalf("no history: %+v", got)
	}
}

func TestEstimateUnknownWithoutCapacity(t *testing.T) {
	samples := history(24*time.Hour, func(h float64) float64 { return 60*gib - h*gib })
	if got := Estimate(samples, 60*gib, 0, 24*time.Hour, now); got.State != metrics.TrendUnknown {
		t.Fatalf("trend=%+v, want unknown without a capacity", got)
	}
}

func TestEstimateNoisyIsNotConfident(t *testing.T) {
	// A cache volume that swings ±3 GiB every few hours over a slight
	// upward drift (0.1 GiB/h, 2.4 GiB over the window): the swing dominates
	// the trend, so the estimate must not be trusted enough to alert.
	samples := history(24*time.Hour, func(h float64) float64 {
		return 50*gib + 3*gib*math.Sin(h*1.7) - h*0.1*gib
	})
	got := Estimate(samples, samples[len(samples)-1].Used, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendFilling || got.Confidence >= 0.6 {
		t.Fatalf("trend=%+v, want filling with confidence < 0.6", got)
	}
	// A swing with no drift reads as steady, but not confidently.
	flat := history(24*time.Hour, func(h float64) float64 { return 50*gib + 3*gib*math.Sin(h*1.7) })
	if got := Estimate(flat, 50*gib, 100*gib, 24*time.Hour, now); got.State != metrics.TrendSteady || got.Confidence >= 0.6 {
		t.Fatalf("trend=%+v, want steady with confidence < 0.6", got)
	}
	// The same drift with small noise is confident.
	clean := history(24*time.Hour, func(h float64) float64 {
		return 60*gib - h*gib + 0.05*gib*math.Sin(h*7)
	})
	if got := Estimate(clean, 60*gib, 100*gib, 24*time.Hour, now); got.Confidence < 0.9 {
		t.Fatalf("confidence=%v for a clean fill, want >= 0.9", got.Confidence)
	}
}

func TestEstimateAlreadyFull(t *testing.T) {
	samples := history(6*time.Hour, func(h float64) float64 { return 100*gib - h*gib })
	got := Estimate(samples, 100*gib, 100*gib, 24*time.Hour, now)
	if got.State != metrics.TrendFilling || got.DaysToFull == nil || *got.DaysToFull != 0 {
		t.Fatalf("trend=%+v, want filling with 0 days", got)
	}
}

func TestEstimateKeepsPrecisionAtTerabytes(t *testing.T) {
	// 16 TiB volume, 12 TiB used, +10 GiB/h. Summing raw squares at this
	// size would cancel the residual; the centred fit must stay exact.
	base := 12 * 1024 * gib
	samples := history(24*time.Hour, func(h float64) float64 { return base - h*10*gib })
	got := Estimate(samples, base, 16*1024*gib, 24*time.Hour, now)
	if got.State != metrics.TrendFilling || got.Confidence < 0.99 {
		t.Fatalf("trend=%+v, want a confident fill", got)
	}
	if want := 4 * 1024 / 10.0 / 24; math.Abs(*got.DaysToFull-want) > 0.01 {
		t.Fatalf("daysToFull=%v, want %v", *got.DaysToFull, want)
	}
}
