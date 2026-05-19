package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

// helloManifest returns a fully-formed docker-runtime manifest used by
// the M1.5 test suite.
func helloManifest() models.SkillManifest {
	return models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime: models.Runtime{
			Type:           models.RuntimeDocker,
			Image:          "python:3.12-slim",
			Entrypoint:     "python -m hello",
			TimeoutSeconds: 10,
		},
		Inputs: map[string]models.IOField{
			"name": {Type: "string", Required: true},
		},
	}
}

func registerSkill(t *testing.T, s *api.Server, p auth.Principal, m models.SkillManifest) {
	t.Helper()
	body, _ := json.Marshal(m)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusCreated, rec.Code, "register failed: %s", rec.Body.String())
}

func TestRuntimeRedactedFromListAndGet(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, p, helloManifest())

	// GET /v1/skills/:ns/:name strips runtime internals.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusOK, rec.Code)
	var got models.SkillManifest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Empty(t, got.Runtime.Image, "runtime.image must be redacted")
	assert.Empty(t, got.Runtime.Entrypoint, "runtime.entrypoint must be redacted")
	assert.Empty(t, got.Runtime.URL, "runtime.url must be redacted")
	assert.Equal(t, models.RuntimeDocker, got.Runtime.Type, "runtime.type stays visible")
	assert.Equal(t, 10, got.Runtime.TimeoutSeconds, "resource limits stay visible")

	// GET /v1/skills also returns redacted entries.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, p))
	require.Equal(t, http.StatusOK, rec2.Code)
	var listResp struct {
		Skills []models.SkillManifest `json:"skills"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &listResp))
	require.Len(t, listResp.Skills, 1)
	assert.Empty(t, listResp.Skills[0].Runtime.Image)

	// Dedicated runtime endpoint exposes the full implementation details
	// to the owning org.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello/runtime", nil)
	s.Handler().ServeHTTP(rec3, withPrincipal(req3, p))
	require.Equal(t, http.StatusOK, rec3.Code)
	var rt models.Runtime
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &rt))
	assert.Equal(t, "python:3.12-slim", rt.Image)
	assert.Equal(t, "python -m hello", rt.Entrypoint)
}

func TestStatsCountsInvocations(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, p, helloManifest())

	// Three invocations.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/hello/invoke", bytes.NewReader([]byte(`{"name":"world"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.7:54321"
		s.Handler().ServeHTTP(rec, withPrincipal(req, p))
		require.Equal(t, http.StatusOK, rec.Code, "invocation %d failed: %s", i, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello/stats", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var stats struct {
		Total         int64  `json:"total"`
		Last24h       int64  `json:"last_24h"`
		LastInvokedAt string `json:"last_invoked_at"`
		LastCallerIP  string `json:"last_caller_ip"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))
	assert.Equal(t, int64(3), stats.Total, "total should match invocation count")
	assert.Equal(t, int64(3), stats.Last24h, "all three should land in the 24h window")
	assert.NotEmpty(t, stats.LastInvokedAt)
	assert.Equal(t, "203.0.113.7", stats.LastCallerIP)
}

func TestStatsScopedToOwningOrg(t *testing.T) {
	s := newTestServer(t)
	orgA := auth.PrincipalForOrg(uuid.New())
	orgB := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, orgA, helloManifest())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello/stats", nil)
	s.Handler().ServeHTTP(rec, withPrincipal(req, orgB))
	assert.Equal(t, http.StatusNotFound, rec.Code, "org B must not see org A's stats")
}

func TestCallerIPTrustsProxyHeaderOnlyWhenEnabled(t *testing.T) {
	manifest := helloManifest()

	cases := []struct {
		name        string
		trustProxy  bool
		xff         string
		remoteAddr  string
		expectedIP  string
		description string
	}{
		{
			name:        "header_honoured_when_trusted",
			trustProxy:  true,
			xff:         "198.51.100.42, 10.0.0.1",
			remoteAddr:  "10.0.0.1:443",
			expectedIP:  "198.51.100.42",
			description: "behind a trusted LB, left-most XFF entry is the real client",
		},
		{
			name:        "header_ignored_when_not_trusted",
			trustProxy:  false,
			xff:         "1.2.3.4",
			remoteAddr:  "127.0.0.1:8080",
			expectedIP:  "127.0.0.1",
			description: "direct exposure must not honour spoofable headers",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
				TrustProxy: tc.trustProxy,
				Dispatcher: runtime.NewDispatcher(stubRunner{}, stubRunner{}),
			})
			p := auth.PrincipalForOrg(uuid.New())
			registerSkill(t, s, p, manifest)

			body := []byte(`{"name":"world"}`)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/hello/invoke", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", tc.xff)
			req.RemoteAddr = tc.remoteAddr
			s.Handler().ServeHTTP(rec, withPrincipal(req, p))
			require.Equal(t, http.StatusOK, rec.Code, tc.description)

			statsRec := httptest.NewRecorder()
			statsReq := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello/stats", nil)
			s.Handler().ServeHTTP(statsRec, withPrincipal(statsReq, p))
			require.Equal(t, http.StatusOK, statsRec.Code)
			var stats struct {
				LastCallerIP string `json:"last_caller_ip"`
			}
			require.NoError(t, json.Unmarshal(statsRec.Body.Bytes(), &stats))
			assert.Equal(t, tc.expectedIP, stats.LastCallerIP, tc.description)
		})
	}
}

func TestRateLimit429AndHeaders(t *testing.T) {
	// Limit = 2 requests per minute. Use a fresh server (no skills
	// registered) so the rate-limit budget isn't spent before the test.
	s := api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
		RateLimit:  api.RateLimitConfig{RequestsPerMinute: 2},
		Dispatcher: runtime.NewDispatcher(stubRunner{}, stubRunner{}),
	})
	p := auth.Principal{OrgID: uuid.New(), APIKeyID: uuid.New()}

	send := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
		req.RemoteAddr = "203.0.113.1:1234"
		s.Handler().ServeHTTP(rec, withPrincipal(req, p))
		return rec
	}

	r1 := send()
	assert.Equal(t, http.StatusOK, r1.Code)
	assert.Equal(t, "2", r1.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "1", r1.Header().Get("X-RateLimit-Remaining"))

	r2 := send()
	assert.Equal(t, http.StatusOK, r2.Code)
	assert.Equal(t, "0", r2.Header().Get("X-RateLimit-Remaining"))

	r3 := send()
	require.Equal(t, http.StatusTooManyRequests, r3.Code, "third request must be rate-limited")
	assert.NotEmpty(t, r3.Header().Get("Retry-After"))
	remaining, err := strconv.Atoi(r3.Header().Get("X-RateLimit-Remaining"))
	require.NoError(t, err)
	assert.Equal(t, 0, remaining, "no quota left")
}

func TestMCPToolsListHidesRuntime(t *testing.T) {
	// tools/list must never echo runtime.image / entrypoint / url, even
	// to org members.
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, p, helloManifest())

	listReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(listReq))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "python:3.12-slim", "tools/list must not leak runtime.image")
	assert.NotContains(t, body, "python -m hello", "tools/list must not leak runtime.entrypoint")
	assert.Contains(t, body, "acme/hello", "tool name must still appear")
}

func TestMCPToolsCallRecordsInvocation(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())
	registerSkill(t, s, p, helloManifest())

	callReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"acme/hello","arguments":{"name":"world"}}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(callReq))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:4242"
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	statsRec := httptest.NewRecorder()
	statsReq := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello/stats", nil)
	s.Handler().ServeHTTP(statsRec, withPrincipal(statsReq, p))
	require.Equal(t, http.StatusOK, statsRec.Code)
	var stats struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(statsRec.Body.Bytes(), &stats))
	assert.Equal(t, int64(1), stats.Total, "MCP tools/call must record an invocation")
}
