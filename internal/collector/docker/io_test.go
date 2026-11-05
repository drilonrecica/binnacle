// SPDX-License-Identifier: AGPL-3.0-only
package docker

import "testing"

func TestNormalizeIOWithoutNetworksIsUnknown(t *testing.T) {
	previous := IOCounters{Read: 100, Write: 100}
	current := IOCounters{Read: 300, Write: 500}
	got := NormalizeIO(previous, current, 2)
	if got.RX != nil || got.TX != nil {
		t.Fatalf("rx=%v tx=%v, want nil for a container without networks", got.RX, got.TX)
	}
	if got.Read == nil || *got.Read != 100 || got.Write == nil || *got.Write != 200 {
		t.Fatalf("read=%v write=%v", got.Read, got.Write)
	}
	// A container that gains a network mid-life has no previous counters.
	current.Network, current.RX = true, 50
	if got := NormalizeIO(previous, current, 2); got.RX != nil {
		t.Fatalf("rx=%v, want nil without a previous network sample", *got.RX)
	}
	previous.Network = true
	if got := NormalizeIO(previous, current, 2); got.RX == nil || *got.RX != 25 {
		t.Fatalf("rx=%v, want 25", got.RX)
	}
	if sum := SumIO([]IOCounters{{}, {Network: true}}); !sum.Network {
		t.Fatal("SumIO lost the network flag")
	}
}
