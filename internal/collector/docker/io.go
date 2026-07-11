// SPDX-License-Identifier: AGPL-3.0-only
package docker

type IOCounters struct{ RX, TX, Read, Write uint64 }
type IORates struct{ RX, TX, Read, Write *float64 }

func SumIO(values []IOCounters) IOCounters {
	var o IOCounters
	for _, v := range values {
		o.RX += v.RX
		o.TX += v.TX
		o.Read += v.Read
		o.Write += v.Write
	}
	return o
}
func NormalizeIO(previous, current IOCounters, elapsed float64) IORates {
	return IORates{delta(current.RX, previous.RX, elapsed), delta(current.TX, previous.TX, elapsed), delta(current.Read, previous.Read, elapsed), delta(current.Write, previous.Write, elapsed)}
}
func delta(now, old uint64, elapsed float64) *float64 {
	if elapsed <= 0 || now < old {
		return nil
	}
	v := float64(now-old) / elapsed
	return &v
}
