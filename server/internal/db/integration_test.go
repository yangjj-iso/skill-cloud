//go:build integration

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/db"
)

func dsn(t *testing.T) string {
	t.Helper()
	v := os.Getenv("SKILLCLOUD_TEST_DSN")
	if v == "" {
		t.Skip("SKILLCLOUD_TEST_DSN not set; skipping postgres integration test")
	}
	return v
}

func TestConnectAndMigrate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn(t))
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, db.Migrate(ctx, pool))

	// Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`).
	require.NoError(t, db.Migrate(ctx, pool))

	// Every table the application relies on should exist.
	tables := []string{"orgs", "users", "org_members", "api_keys", "skills", "skill_versions", "invocations"}
	for _, name := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			name).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %q should exist", name)
	}
}
