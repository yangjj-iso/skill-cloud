package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
)

// fakeAuthenticator is a stub Authenticator used by middleware tests.
type fakeAuthenticator struct {
	expectedToken string
	principal     auth.Principal
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token != f.expectedToken {
		return auth.Principal{}, auth.ErrInvalidKey
	}
	return f.principal, nil
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	a := &fakeAuthenticator{expectedToken: "valid", principal: auth.PrincipalForOrg(uuid.New())}
	engine.Use(auth.Middleware(a))
	engine.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_RejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	a := &fakeAuthenticator{expectedToken: "valid", principal: auth.PrincipalForOrg(uuid.New())}
	engine.Use(auth.Middleware(a))
	engine.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_InjectsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	want := auth.PrincipalForOrg(uuid.New())
	a := &fakeAuthenticator{expectedToken: "valid", principal: want}
	engine.Use(auth.Middleware(a))
	engine.GET("/me", func(c *gin.Context) {
		p, ok := auth.PrincipalFromContext(c.Request.Context())
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no principal"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"org_id": p.OrgID.String()})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, want.OrgID.String(), resp["org_id"])
}
