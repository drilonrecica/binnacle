// SPDX-License-Identifier: AGPL-3.0-only
package docker

type IOCounters struct {
	RX, TX, Read, Write uint64
	// Network is false when the container has no networks (host or none
	// network mode); its RX and TX rates are then nil, not zero (COL-7).
	Network bool
}
type IORates struct{ RX, TX, Read, Write *float64 }

func SumIO(values []IOCounters) IOCounters {
	var o IOCounters
	for _, v := range values {
		o.RX += v.RX
		o.TX += v.TX
		o.Read += v.Read
		o.Write += v.Write
		o.Network = o.Network || v.Network
	}
	return o
}
func NormalizeIO(previous, current IOCounters, elapsed float64) IORates {
	rates := IORates{Read: delta(current.Read, previous.Read, elapsed), Write: delta(current.Write, previous.Write, elapsed)}
	if previous.Network && current.Network {
		rates.RX, rates.TX = delta(current.RX, previous.RX, elapsed), delta(current.TX, previous.TX, elapsed)
	}
	return rates
}
func delta(now, old uint64, elapsed float64) *float64 {
	if elapsed <= 0 || now < old {
		return nil
	}
	v := float64(now-old) / elapsed
	return &v
}
