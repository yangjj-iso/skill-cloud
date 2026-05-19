package invocations

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// memoryCap bounds the in-memory ring buffer so the dev process can't
// leak unboundedly. Records past the cap are evicted FIFO.
const memoryCap = 10_000

// Memory is a Store backed by an in-process slice. Used for the no-DB
// dev mode and unit tests; production wiring uses Postgres instead.
type Memory struct {
	mu      sync.Mutex
	entries []Entry
}

// NewMemory constructs an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{entries: make([]Entry, 0)}
}

// Log records one invocation. Thread-safe.
func (m *Memory) Log(_ context.Context, e Entry) error {
	if e.StartedAt.IsZero() {
		e.StartedAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	if len(m.entries) > memoryCap {
		m.entries = m.entries[len(m.entries)-memoryCap:]
	}
	return nil
}

// Stats summarises invocations of (orgID, namespace, name).
func (m *Memory) Stats(_ context.Context, orgID uuid.UUID, namespace, name string) (Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	var s Stats
	for _, e := range m.entries {
		if e.OrgID != orgID || e.Namespace != namespace || e.Name != name {
			continue
		}
		s.Total++
		if e.StartedAt.After(cutoff) {
			s.Last24h++
		}
		if s.LastInvokedAt == nil || e.StartedAt.After(*s.LastInvokedAt) {
			t := e.StartedAt
			s.LastInvokedAt = &t
			s.LastCallerIP = e.CallerIP
		}
	}
	return s, nil
}
