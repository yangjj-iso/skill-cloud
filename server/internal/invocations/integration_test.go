//go:build integration

package invocations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/db"
	"github.com/yangjj-iso/skill-cloud/server/internal/invocations"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
)

func TestPostgresInvocationLogAndStats(t *testing.T) {
	dsn := os.Getenv("SKILLCLOUD_TEST_DSN")
	if dsn == "" {
		t.Skip("SKILLCLOUD_TEST_DSN not set; skipping postgres integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, db.Migrate(ctx, pool))

	// Fresh org + skill so the row counts are deterministic.
	orgID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $2)`,
		orgID, "test-"+orgID.String()[:8])
	require.NoError(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM orgs WHERE id = $1`, orgID)
	}()

	reg := registry.NewPostgres(pool)
	_, err = reg.Upsert(ctx, orgID, models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12-slim"},
	})
	require.NoError(t, err)

	store := invocations.NewPostgres(pool)
	for i := 0; i < 5; i++ {
		err := store.Log(ctx, invocations.Entry{
			OrgID:       orgID,
			Namespace:   "acme",
			Name:        "hello",
			Version:     "0.1.0",
			Status:      "ok",
			LatencyMS:   10 + i,
			InputBytes:  16,
			OutputBytes: 64,
			CallerIP:    "203.0.113.42",
			UserAgent:   "skillcloud-test/0.1",
			StartedAt:   time.Now().UTC(),
		})
		require.NoError(t, err)
	}

	stats, err := store.Stats(ctx, orgID, "acme", "hello")
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.Total)
	assert.Equal(t, int64(5), stats.Last24h)
	require.NotNil(t, stats.LastInvokedAt)
	assert.Equal(t, "203.0.113.42", stats.LastCallerIP)

	// Logging an entry for a non-existent skill is silently dropped, not
	// an error (the request itself must succeed).
	err = store.Log(ctx, invocations.Entry{
		OrgID:     orgID,
		Namespace: "acme",
		Name:      "does-not-exist",
		Version:   "0.1.0",
		Status:    "ok",
	})
	assert.NoError(t, err)
}
