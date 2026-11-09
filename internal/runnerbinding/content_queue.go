package runnerbinding

import (
	"strings"
	"sync"
	"time"
)

// ContentQueue is home-scoped admission bookkeeping reconstructed from
// content records. Runner remains the execution owner.
type ContentQueue struct {
	mu      sync.Mutex
	limit   int
	entries map[string]contentSlot
}

// A project that could not start must not strand a FIFO head. Its submitted
// executions remain counted; only unsent work loses in-memory eligibility.
func (q *ContentQueue) RemoveWaiting(project string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for key, entry := range q.entries {
		if strings.HasPrefix(key, project+"\n") && !entry.active {
			delete(q.entries, key)
		}
	}
}

type contentSlot struct {
	created time.Time
	active  bool
}

func NewContentQueue(limit int) *ContentQueue {
	if limit < 1 {
		limit = 5
	}
	return &ContentQueue{limit: limit, entries: map[string]contentSlot{}}
}

// UpdateContent admits another content purpose into the same home-wide budget.
// The caller owns its durable record; this queue never stores business payloads.
func (q *ContentQueue) UpdateContent(project, id string, created time.Time, active, settled bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := project + "\n" + id
	if settled {
		delete(q.entries, key)
		return
	}
	q.entries[key] = contentSlot{created: created, active: active}
}

func (q *ContentQueue) AcquireContent(project, id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := project + "\n" + id
	entry, ok := q.entries[key]
	if !ok || entry.active {
		return false
	}
	active, ahead := 0, 0
	for k, v := range q.entries {
		if v.active {
			active++
			continue
		}
		if v.created.Before(entry.created) || v.created.Equal(entry.created) && k < key {
			ahead++
		}
	}
	if active+ahead >= q.limit {
		return false
	}
	entry.active = true
	q.entries[key] = entry
	return true
}
