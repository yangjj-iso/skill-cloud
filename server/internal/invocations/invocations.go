// Package invocations records every skill call and exposes per-skill
// usage statistics. Each invocation logs the caller's identity, source
// IP, payload sizes, status, and latency, supporting both abuse
// investigation and future billing.
package invocations

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Entry is a single recorded skill invocation.
type Entry struct {
	OrgID        uuid.UUID
	UserID       uuid.UUID // zero when called in no-auth dev mode
	APIKeyID     uuid.UUID // zero when called in no-auth dev mode
	Namespace    string
	Name         string
	Version      string
	Status       string // "ok" | "error" | "rejected"
	LatencyMS    int
	InputBytes   int
	OutputBytes  int
	ErrorMessage string
	CallerIP     string
	UserAgent    string
	StartedAt    time.Time
}

// Stats summarises invocation history for a single skill.
type Stats struct {
	Total         int64      `json:"total"`
	Last24h       int64      `json:"last_24h"`
	LastInvokedAt *time.Time `json:"last_invoked_at,omitempty"`
	LastCallerIP  string     `json:"last_caller_ip,omitempty"`
}

// Store records invocations and serves aggregate stats.
type Store interface {
	Log(ctx context.Context, e Entry) error
	Stats(ctx context.Context, orgID uuid.UUID, namespace, name string) (Stats, error)
	// Recent returns the most recent invocations for one skill, newest
	// first, capped at `limit` rows. Implementations should treat
	// limit <= 0 as "use a default" (typically 50).
	Recent(ctx context.Context, orgID uuid.UUID, namespace, name string, limit int) ([]Entry, error)
}
