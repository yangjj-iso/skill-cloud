package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
)

// TestOrgOverviewEndpoint exercises the BFF endpoint that powers the
// Web UI dashboard. It registers a skill, fires off two invocations,
// then asserts the aggregate fields and the recent-list shape.
func TestOrgOverviewEndpoint(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, p, helloManifest())

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/hello/invoke", strings.NewReader(`{"name":"world"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.9:1024"
		s.Handler().ServeHTTP(rec, withPrincipal(req, p))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/overview", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var got struct {
		SkillsTotal      int            `json:"skills_total"`
		SkillsByRuntime  map[string]int `json:"skills_by_runtime"`
		InvocationsTotal int64          `json:"invocations_total"`
		Invocations24h   int64          `json:"invocations_24h"`
		Recent           []struct {
			Status    string `json:"status"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			LatencyMS int    `json:"latency_ms"`
			CallerIP  string `json:"caller_ip"`
		} `json:"recent"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 1, got.SkillsTotal)
	assert.Equal(t, 1, got.SkillsByRuntime["docker"])
	assert.Equal(t, int64(2), got.InvocationsTotal)
	assert.Equal(t, int64(2), got.Invocations24h)
	require.Len(t, got.Recent, 2)
	assert.Equal(t, "acme", got.Recent[0].Namespace)
	assert.Equal(t, "hello", got.Recent[0].Name)
	assert.Equal(t, "ok", got.Recent[0].Status)
	assert.Equal(t, "203.0.113.9", got.Recent[0].CallerIP)
}

// TestOrgInvocationsEndpoint asserts that /v1/invocations returns the
// cross-skill recent list and is scoped to the calling org.
func TestOrgInvocationsEndpoint(t *testing.T) {
	s := newTestServer(t)
	orgA := auth.PrincipalForOrg(uuid.New())
	orgB := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, orgA, helloManifest())

	body := []byte(`{"name":"world"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/hello/invoke", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, orgA))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/invocations", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, orgA))
	require.Equal(t, http.StatusOK, rec.Code)
	var asOwner struct {
		Invocations []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"invocations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &asOwner))
	assert.Len(t, asOwner.Invocations, 1)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/invocations", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, orgB))
	require.Equal(t, http.StatusOK, rec.Code)
	var asOther struct {
		Invocations []any `json:"invocations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &asOther))
	assert.Empty(t, asOther.Invocations, "org B must not see org A's invocations")
}
