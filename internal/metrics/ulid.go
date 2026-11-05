// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"crypto/rand"
	"sync"
	"time"
)

// crockford is the Crockford base32 alphabet ULIDs are written in.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// eventIDs is the process-wide event id generator (ADR 021 §9). The engine
// and the storage package draw from it, so their events never collide.
var eventIDs ulidGenerator

// NewEventID returns a new ULID: 26 Crockford base32 characters encoding 48
// bits of Unix milliseconds and 80 bits of crypto/rand randomness. Ids are
// strictly increasing within the process, so a batch keeps its order when
// sorted as text, and unique across process starts (EVT-1).
func NewEventID() string { return eventIDs.next(time.Now()) }

type ulidGenerator struct {
	mu     sync.Mutex
	lastMs uint64
	random [10]byte
}

// next applies the monotonic rule: within one millisecond, and when the clock
// steps back, the previous randomness is incremented instead of redrawn.
func (g *ulidGenerator) next(now time.Time) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := uint64(max(now.UnixMilli(), 0)) & (1<<48 - 1)
	if ms > g.lastMs || g.lastMs == 0 {
		g.lastMs = ms
		_, _ = rand.Read(g.random[:]) // crypto/rand.Read never returns an error.
	} else if !increment(g.random[:]) {
		// 2^80 ids in one millisecond: move to the next millisecond.
		g.lastMs++
		_, _ = rand.Read(g.random[:])
	}
	return encodeULID(g.lastMs, g.random)
}

// increment adds one to a big-endian number and reports false on overflow.
func increment(b []byte) bool {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return true
		}
	}
	return false
}

func encodeULID(ms uint64, random [10]byte) string {
	// The 128-bit value as hi:lo, timestamp in the top 48 bits.
	hi := ms<<16 | uint64(random[0])<<8 | uint64(random[1])
	var lo uint64
	for _, b := range random[2:] {
		lo = lo<<8 | uint64(b)
	}
	var out [26]byte
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}
