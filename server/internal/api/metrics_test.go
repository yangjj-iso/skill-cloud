package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/metrics"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

func TestMetricsEndpointIsUnauthenticated(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "skillcloud_invocations_total")
}

func TestMetricsRecordsInvocationsAndHTTPRequests(t *testing.T) {
	metrics.InvocationsTotal.Reset()
	metrics.HTTPRequestsTotal.Reset()

	s := api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
		Dispatcher: runtime.NewDispatcher(stubRunner{}, stubRunner{}),
	})
	orgID := uuid.New()
	p := auth.Principal{OrgID: orgID, UserID: uuid.New(), APIKeyID: uuid.New()}

	manifest := models.SkillManifest{
		Namespace:   "acme",
		Name:        "metricstest",
		Version:     "0.1.0",
		Description: "metrics test",
		Runtime:     models.Runtime{Type: models.RuntimeDocker, Image: "ignored:latest"},
		Inputs:      map[string]models.IOField{"x": {Type: "string"}},
		Outputs:     map[string]models.IOField{"echoed": {Type: "object"}},
	}
	body, _ := json.Marshal(manifest)

	createReq := withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body)), p)
	createReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, createReq)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	invokeReq := withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/skills/acme/metricstest/invoke", strings.NewReader(`{"x":"hi"}`)), p)
	invokeReq.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, invokeReq)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	dump := rec.Body.String()

	require.Contains(t, dump, `skillcloud_invocations_total{name="metricstest",namespace="acme",org="`+orgID.String()+`",status="ok"} 1`)
	require.Contains(t, dump, `skillcloud_invocation_latency_seconds_count{name="metricstest",namespace="acme",org="`+orgID.String()+`"} 1`)
	require.Contains(t, dump, `skillcloud_http_requests_total{method="POST",route="/v1/skills",status_code="201"} 1`)
	require.Contains(t, dump, `skillcloud_http_requests_total{method="POST",route="/v1/skills/:namespace/:name/invoke",status_code="200"} 1`)
}

func TestMetricsRecordsRateLimitDrops(t *testing.T) {
	metrics.RateLimitDropped.Reset()

	s := api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
		Dispatcher: runtime.NewDispatcher(stubRunner{}, stubRunner{}),
		RateLimit:  api.RateLimitConfig{RequestsPerMinute: 1},
	})
	p := auth.Principal{OrgID: uuid.New(), UserID: uuid.New(), APIKeyID: uuid.New()}

	// First request is allowed; second is blocked and should bump the counter.
	for i := 0; i < 2; i++ {
		req := withPrincipal(httptest.NewRequest(http.MethodGet, "/v1/skills", nil), p)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `skillcloud_rate_limit_dropped_total{api_key_prefix="`+p.APIKeyID.String()[:12]+`"} 1`)
}
