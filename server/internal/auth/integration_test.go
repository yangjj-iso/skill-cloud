//go:build integration

package auth_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/db"
)

func TestAuthServiceIssueAndVerify(t *testing.T) {
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

	svc := auth.NewService(pool)
	orgID, err := svc.CreateOrg(ctx, "test-"+time.Now().Format("150405.000"), "test org")
	require.NoError(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM orgs WHERE id = $1`, orgID)
	}()

	userID, err := svc.CreateUser(ctx, orgID, "test-"+time.Now().Format("150405.000")+"@example.com")
	require.NoError(t, err)

	issued, err := svc.IssueAPIKey(ctx, orgID, userID, "test key")
	require.NoError(t, err)
	require.NotEmpty(t, issued.Plaintext)
	require.NotEmpty(t, issued.Prefix)

	// Valid token authenticates and returns the expected principal.
	p, err := svc.Authenticate(ctx, issued.Plaintext)
	require.NoError(t, err)
	assert.Equal(t, orgID, p.OrgID)
	assert.Equal(t, userID, p.UserID)
	assert.Equal(t, issued.ID, p.APIKeyID)

	// Tampered token fails.
	_, err = svc.Authenticate(ctx, issued.Prefix+".not-the-real-secret")
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidKey))

	// Unknown prefix fails.
	_, err = svc.Authenticate(ctx, "sc_live_unknown.abc")
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidKey))
}
