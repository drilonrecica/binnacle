package host

import "testing"

func TestNetwork(t *testing.T) {
	n, e := ParseNetDev("eth0: 10 1 0 0 0 0 0 0 20 2 0 0 0 0 0 0\nlo: 99 0 0 0 0 0 0 0 99 0 0 0 0 0 0 0")
	if e != nil || AggregateNetwork(n).RXBytes != 10 {
		t.Fatal(e)
	}
	if Rate(1, 2, 1) != nil {
		t.Fatal("reset")
	}
}
