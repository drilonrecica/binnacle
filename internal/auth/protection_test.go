// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"testing"
	"time"
)

func TestLimiterRecoversAfterRefill(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(2)
	l.now = func() time.Time { return now }
	p := BucketPolicy{Capacity: 1, Refill: time.Minute}
	if ok, _ := l.Allow("ip", p); !ok {
		t.Fatal("first request denied")
	}
	if ok, _ := l.Allow("ip", p); ok {
		t.Fatal("burst limit not enforced")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow("ip", p); !ok {
		t.Fatal("bucket did not refill")
	}
}
