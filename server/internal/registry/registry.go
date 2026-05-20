// Package registry stores skill manifests. The package exposes a
// Registry interface implemented by an in-memory backend (used for unit
// tests and as a fallback when no database is configured) and a
// Postgres-backed backend (used in production).
package registry

import (
	"context"

	"github.com/google/uuid"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// Registry stores skills scoped by org.
type Registry interface {
	// Upsert inserts or replaces the manifest for (orgID, namespace, name).
	Upsert(ctx context.Context, orgID uuid.UUID, m models.SkillManifest) (models.SkillManifest, error)

	// Get returns the manifest for a skill in the given org, or ok=false.
	Get(ctx context.Context, orgID uuid.UUID, namespace, name string) (models.SkillManifest, bool, error)

	// List returns every skill belonging to the given org.
	List(ctx context.Context, orgID uuid.UUID) ([]models.SkillManifest, error)
}
