package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// Postgres is a Postgres-backed Registry. Every query is scoped by org_id
// (passed in by callers, which themselves derive it from the authenticated
// principal) so no row can leak across orgs.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres constructs a Postgres-backed registry.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Upsert inserts or replaces a skill and bumps `latest_version`. Every
// version is also written to skill_versions.
func (r *Postgres) Upsert(ctx context.Context, orgID uuid.UUID, m models.SkillManifest) (models.SkillManifest, error) {
	manifestJSON, err := json.Marshal(m)
	if err != nil {
		return models.SkillManifest{}, fmt.Errorf("marshal manifest: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.SkillManifest{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var skillID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO skills (id, org_id, namespace, name, description, latest_version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (org_id, namespace, name)
		DO UPDATE SET description = EXCLUDED.description,
		              latest_version = EXCLUDED.latest_version,
		              updated_at = now()
		RETURNING id
	`, uuid.New(), orgID, m.Namespace, m.Name, m.Description, m.Version).Scan(&skillID)
	if err != nil {
		return models.SkillManifest{}, fmt.Errorf("upsert skill: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO skill_versions (id, skill_id, version, manifest)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (skill_id, version)
		DO UPDATE SET manifest = EXCLUDED.manifest
	`, uuid.New(), skillID, m.Version, manifestJSON)
	if err != nil {
		return models.SkillManifest{}, fmt.Errorf("upsert version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SkillManifest{}, fmt.Errorf("commit: %w", err)
	}
	return m, nil
}

// Get returns the latest version of a skill within an org.
func (r *Postgres) Get(ctx context.Context, orgID uuid.UUID, namespace, name string) (models.SkillManifest, bool, error) {
	var manifestJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT sv.manifest
		FROM skills s
		JOIN skill_versions sv ON sv.skill_id = s.id AND sv.version = s.latest_version
		WHERE s.org_id = $1 AND s.namespace = $2 AND s.name = $3
	`, orgID, namespace, name).Scan(&manifestJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.SkillManifest{}, false, nil
		}
		return models.SkillManifest{}, false, fmt.Errorf("get skill: %w", err)
	}
	var m models.SkillManifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return models.SkillManifest{}, false, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return m, true, nil
}

// List returns the latest version of every skill in an org.
func (r *Postgres) List(ctx context.Context, orgID uuid.UUID) ([]models.SkillManifest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sv.manifest
		FROM skills s
		JOIN skill_versions sv ON sv.skill_id = s.id AND sv.version = s.latest_version
		WHERE s.org_id = $1
		ORDER BY s.namespace, s.name
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	out := make([]models.SkillManifest, 0)
	for rows.Next() {
		var manifestJSON []byte
		if err := rows.Scan(&manifestJSON); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var m models.SkillManifest
		if err := json.Unmarshal(manifestJSON, &m); err != nil {
			return nil, fmt.Errorf("unmarshal manifest: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
