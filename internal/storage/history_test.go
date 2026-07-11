// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"testing"
	"time"
)

func TestMetricQueryValidationAndResolution(t *testing.T) {
	q := MetricQuery{Scope: "resource", ID: "res_test", Metrics: []Metric{MetricCPU}, From: time.Now().Add(-time.Hour), To: time.Now()}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := selectResolution(30 * 24 * time.Hour); got != Resolution1h {
		t.Fatalf("resolution=%s", got)
	}
	q.Metrics = []Metric{MetricBlockRead}
	q.Scope = "host"
	if err := q.Validate(); err == nil {
		t.Fatal("host block metric accepted")
	}
}
