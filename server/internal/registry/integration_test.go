//go:build integration

package registry_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/db"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
)

func setupDB(t *testing.T) (context.Context, *registry.Postgres, uuid.UUID, uuid.UUID, func()) {
	t.Helper()
	dsn := os.Getenv("SKILLCLOUD_TEST_DSN")
	if dsn == "" {
		t.Skip("SKILLCLOUD_TEST_DSN not set; skipping postgres integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	pool, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, pool))

	// Each test gets its own pair of orgs so runs don't interfere.
	orgA := uuid.New()
	orgB := uuid.New()
	for _, id := range []uuid.UUID{orgA, orgB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $2)`,
			id, "test-"+id.String()[:8])
		require.NoError(t, err)
	}

	reg := registry.NewPostgres(pool)
	cleanup := func() {
		for _, id := range []uuid.UUID{orgA, orgB} {
			_, _ = pool.Exec(context.Background(),
				`DELETE FROM orgs WHERE id = $1`, id)
		}
		pool.Close()
		cancel()
	}
	return ctx, reg, orgA, orgB, cleanup
}

func sampleManifest() models.SkillManifest {
	return models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12-slim"},
		Inputs:    map[string]models.IOField{"name": {Type: "string"}},
	}
}

func TestPostgresUpsertAndGet(t *testing.T) {
	ctx, reg, orgA, _, cleanup := setupDB(t)
	defer cleanup()

	m := sampleManifest()
	_, err := reg.Upsert(ctx, orgA, m)
	require.NoError(t, err)

	got, ok, err := reg.Get(ctx, orgA, m.Namespace, m.Name)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, m.QualifiedName(), got.QualifiedName())
	assert.Equal(t, m.Version, got.Version)

	// Upsert is idempotent on (org_id, namespace, name).
	m.Description = "updated"
	_, err = reg.Upsert(ctx, orgA, m)
	require.NoError(t, err)
	got2, _, err := reg.Get(ctx, orgA, m.Namespace, m.Name)
	require.NoError(t, err)
	assert.Equal(t, "updated", got2.Description)
}

func TestPostgresListIsScopedByOrg(t *testing.T) {
	ctx, reg, orgA, orgB, cleanup := setupDB(t)
	defer cleanup()

	_, err := reg.Upsert(ctx, orgA, sampleManifest())
	require.NoError(t, err)

	// orgB sees nothing.
	listB, err := reg.List(ctx, orgB)
	require.NoError(t, err)
	assert.Empty(t, listB)

	_, ok, err := reg.Get(ctx, orgB, "acme", "hello")
	require.NoError(t, err)
	assert.False(t, ok)

	// orgA sees its own skill.
	listA, err := reg.List(ctx, orgA)
	require.NoError(t, err)
	require.Len(t, listA, 1)
	assert.Equal(t, "acme/hello", listA[0].QualifiedName())
}
