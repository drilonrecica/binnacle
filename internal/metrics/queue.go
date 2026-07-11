// SPDX-License-Identifier: AGPL-3.0-only
package metrics

import "sync"

type BatchQueue struct {
	mu             sync.Mutex
	items          []PersistenceBatch
	Limit, Dropped int
}

func (q *BatchQueue) Push(v PersistenceBatch) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.Limit <= 0 {
		q.Limit = 60
	}
	if len(q.items) >= q.Limit {
		q.items = q.items[1:]
		q.Dropped++
	}
	q.items = append(q.items, v)
}
func (q *BatchQueue) Pop() (PersistenceBatch, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return PersistenceBatch{}, false
	}
	v := q.items[0]
	q.items = q.items[1:]
	return v, true
}
func (q *BatchQueue) Depth() int { q.mu.Lock(); defer q.mu.Unlock(); return len(q.items) }
