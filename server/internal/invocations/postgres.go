package invocations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is a Store backed by the `invocations` table.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres constructs a Postgres-backed invocation store.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Log inserts an invocation row. If the skill cannot be resolved
// (deleted between dispatch and log), the entry is dropped silently so
// the request itself doesn't fail.
func (p *Postgres) Log(ctx context.Context, e Entry) error {
	var skillID uuid.UUID
	err := p.pool.QueryRow(ctx, `
		SELECT id FROM skills WHERE org_id = $1 AND namespace = $2 AND name = $3
	`, e.OrgID, e.Namespace, e.Name).Scan(&skillID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lookup skill: %w", err)
	}

	var userID, apiKeyID any
	if e.UserID != uuid.Nil {
		userID = e.UserID
	}
	if e.APIKeyID != uuid.Nil {
		apiKeyID = e.APIKeyID
	}
	var callerIP any
	if e.CallerIP != "" {
		callerIP = e.CallerIP
	}

	started := e.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	finished := started.Add(time.Duration(e.LatencyMS) * time.Millisecond)

	_, err = p.pool.Exec(ctx, `
		INSERT INTO invocations
		    (id, org_id, user_id, api_key_id, skill_id, version, status,
		     input, output, error_message, caller_ip, user_agent,
		     input_bytes, output_bytes,
		     started_at, finished_at, latency_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
		        '{}'::jsonb, '{}'::jsonb, $8, $9, $10,
		        $11, $12,
		        $13, $14, $15)
	`,
		uuid.New(), e.OrgID, userID, apiKeyID, skillID, e.Version, e.Status,
		e.ErrorMessage, callerIP, e.UserAgent,
		e.InputBytes, e.OutputBytes,
		started, finished, e.LatencyMS,
	)
	if err != nil {
		return fmt.Errorf("insert invocation: %w", err)
	}
	return nil
}

// Recent returns the most recent `limit` invocations for one skill,
// newest first. limit <= 0 is treated as 50; the query caps at 200
// regardless to keep one bad caller from yanking the whole table over
// the wire.
func (p *Postgres) Recent(ctx context.Context, orgID uuid.UUID, namespace, name string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := p.pool.Query(ctx, `
		SELECT i.status, i.latency_ms, i.input_bytes, i.output_bytes,
		       i.started_at, COALESCE(host(i.caller_ip), ''), COALESCE(i.user_agent, ''),
		       COALESCE(i.error_message, ''), s.namespace, s.name, i.version
		FROM invocations i
		JOIN skills s ON s.id = i.skill_id
		WHERE i.org_id = $1 AND s.namespace = $2 AND s.name = $3
		ORDER BY i.started_at DESC
		LIMIT $4
	`, orgID, namespace, name, limit)
	if err != nil {
		return nil, fmt.Errorf("recent invocations: %w", err)
	}
	defer rows.Close()
	out := make([]Entry, 0, limit)
	for rows.Next() {
		var e Entry
		e.OrgID = orgID
		if err := rows.Scan(
			&e.Status, &e.LatencyMS, &e.InputBytes, &e.OutputBytes,
			&e.StartedAt, &e.CallerIP, &e.UserAgent,
			&e.ErrorMessage, &e.Namespace, &e.Name, &e.Version,
		); err != nil {
			return nil, fmt.Errorf("scan invocation: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Stats aggregates the invocations table for one skill.
func (p *Postgres) Stats(ctx context.Context, orgID uuid.UUID, namespace, name string) (Stats, error) {
	var (
		s        Stats
		lastTime *time.Time
		lastIP   *string
	)
	err := p.pool.QueryRow(ctx, `
		SELECT
		    COALESCE(COUNT(*), 0),
		    COALESCE(COUNT(*) FILTER (WHERE i.started_at >= now() - interval '24 hours'), 0),
		    MAX(i.started_at),
		    (SELECT host(caller_ip) FROM invocations
		     WHERE org_id = $1 AND skill_id = s.id
		     ORDER BY started_at DESC LIMIT 1)
		FROM skills s
		LEFT JOIN invocations i ON i.skill_id = s.id
		WHERE s.org_id = $1 AND s.namespace = $2 AND s.name = $3
		GROUP BY s.id
	`, orgID, namespace, name).Scan(&s.Total, &s.Last24h, &lastTime, &lastIP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Stats{}, nil
		}
		return Stats{}, fmt.Errorf("stats: %w", err)
	}
	if lastTime != nil {
		t := lastTime.UTC()
		s.LastInvokedAt = &t
	}
	if lastIP != nil {
		s.LastCallerIP = *lastIP
	}
	return s, nil
}
