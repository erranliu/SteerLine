package testmanagement

import (
	"strings"
	"sync"
	"time"
)

// GenerationQueue is home-scoped admission bookkeeping reconstructed from
// Generation records. Runner remains the execution owner.
type GenerationQueue struct {
	mu      sync.Mutex
	limit   int
	entries map[string]generationSlot
}

// A project that could not start must not strand a FIFO head. Its submitted
// executions remain counted; only unsent work loses in-memory eligibility.
func (q *GenerationQueue) RemoveWaiting(project string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for key, entry := range q.entries {
		if strings.HasPrefix(key, project+"\n") && !entry.active {
			delete(q.entries, key)
		}
	}
}

type generationSlot struct {
	created time.Time
	active  bool
}

func NewGenerationQueue(limit int) *GenerationQueue {
	if limit < 1 {
		limit = 5
	}
	return &GenerationQueue{limit: limit, entries: map[string]generationSlot{}}
}

func generationKey(g Generation) string { return g.Project + "\n" + g.ID }

// Update runs under the project's generation lock, or during startup census
// before any project can submit. Unknown submissions retain their slots.
func (q *GenerationQueue) Update(g Generation) {
	q.UpdateContent(g.Project, g.ID, g.CreatedAt, !g.Unsubmitted(), g.Settled())
}

// UpdateContent admits another content purpose into the same home-wide budget.
// The caller owns its durable record; this queue never stores business payloads.
func (q *GenerationQueue) UpdateContent(project, id string, created time.Time, active, settled bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := project + "\n" + id
	if settled {
		delete(q.entries, key)
		return
	}
	q.entries[key] = generationSlot{created: created, active: active}
}

func (q *GenerationQueue) acquire(g Generation) bool {
	return q.AcquireContent(g.Project, g.ID)
}

func (q *GenerationQueue) AcquireContent(project, id string) bool {
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
