package host

import (
	"os"
	"testing"
)

func TestNetwork(t *testing.T) {
	n, e := ParseNetDev("eth0: 10 1 0 0 0 0 0 0 20 2 0 0 0 0 0 0\nlo: 99 0 0 0 0 0 0 0 99 0 0 0 0 0 0 0")
	if e != nil || AggregateNetwork(n).RXBytes != 10 {
		t.Fatal(e)
	}
	if Rate(1, 2, 1) != nil {
		t.Fatal("reset")
	}
}

func TestNetDevFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/net_dev")
	if err != nil {
		t.Fatal(err)
	}
	n, err := ParseNetDev(string(raw))
	if err != nil || len(n) != 6 {
		t.Fatalf("%v %v", n, err)
	}
	if eth := n["eth0"]; eth.RXBytes != 8812344017 || eth.RXErrors != 3 || eth.RXDrops != 12 || eth.TXBytes != 1290348812 || eth.TXPackets != 4123334 || eth.TXDrops != 2 {
		t.Fatalf("eth0 %+v", eth)
	}
	if agg := AggregateNetwork(n); agg.RXBytes != 8812344017+1000 || agg.TXBytes != 1290348812+2000 {
		t.Fatalf("aggregate %+v", agg)
	}
}
