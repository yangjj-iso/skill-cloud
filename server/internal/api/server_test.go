package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

// stubRunner is a deterministic in-process Runner used by the api
// package's unit tests. It echoes its input back as the output so
// assertions can pin to a known shape without needing docker / a real
// HTTP server.
type stubRunner struct{}

func (stubRunner) Run(_ context.Context, req runtime.Request) (runtime.Result, error) {
	output := map[string]any{
		"echoed": req.Input,
		"skill":  req.Skill.QualifiedName(),
	}
	body, _ := json.Marshal(output)
	return runtime.Result{
		Status:      runtime.StatusOK,
		Output:      output,
		OutputBytes: len(body),
	}, nil
}

// newTestServer constructs a server with the default in-memory registry
// and a stub runner for both runtime types. Tests inject a Principal
// directly via the request context using `withPrincipal`.
func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	return api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
		Dispatcher: runtime.NewDispatcher(stubRunner{}, stubRunner{}),
	})
}

// withPrincipal returns a copy of req with the given Principal attached to
// its context, mimicking what the auth middleware would do in production.
func withPrincipal(req *http.Request, p auth.Principal) *http.Request {
	return req.WithContext(auth.InjectPrincipal(req.Context(), p))
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok"`)
}

func TestCreateAndGetSkill(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())

	manifest := models.SkillManifest{
		Namespace:   "acme",
		Name:        "hello",
		Version:     "0.1.0",
		Description: "Say hello.",
		Runtime: models.Runtime{
			Type:           models.RuntimeDocker,
			Image:          "python:3.12-slim",
			Entrypoint:     "python -m hello",
			TimeoutSeconds: 10,
		},
		Inputs: map[string]models.IOField{
			"name": {Type: "string", Required: true},
		},
		Outputs: map[string]models.IOField{
			"message": {Type: "string"},
		},
	}

	body, err := json.Marshal(manifest)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusCreated, rec.Code, "response body: %s", rec.Body.String())

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello", nil)
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, p))
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), `"acme"`)
}

func TestInvokeSkillDispatchesToRunner(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())

	manifest := models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime: models.Runtime{
			Type:  models.RuntimeDocker,
			Image: "python:3.12-slim",
		},
	}
	body, _ := json.Marshal(manifest)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusCreated, rec.Code)

	rec2 := httptest.NewRecorder()
	input := []byte(`{"name":"world"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/hello/invoke", bytes.NewReader(input))
	req2.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, p))
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		Skill  string         `json:"skill"`
		Status string         `json:"status"`
		Output map[string]any `json:"output"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	assert.Equal(t, "acme/hello", resp.Skill)
	assert.Equal(t, "ok", resp.Status)
	echoed, ok := resp.Output["echoed"].(map[string]any)
	require.True(t, ok, "stub runner should echo input back")
	assert.Equal(t, "world", echoed["name"])
}

func TestInvokeSkillSurfacesRuntimeError(t *testing.T) {
	// The dispatcher returns a 502 with the error body when the runner
	// reports `error` status — proves invoke no longer hard-codes 200.
	failing := &runnerFunc{fn: func(_ context.Context, _ runtime.Request) (runtime.Result, error) {
		return runtime.Result{
			Status:       runtime.StatusError,
			ErrorMessage: "boom",
		}, nil
	}}
	s := api.NewServer(api.Config{ListenAddr: ":0"}, api.Options{
		Dispatcher: runtime.NewDispatcher(failing, failing),
	})
	p := auth.PrincipalForOrg(uuid.New())

	manifest := models.SkillManifest{
		Namespace: "acme",
		Name:      "boom",
		Version:   "0.1.0",
		Runtime: models.Runtime{
			Type:  models.RuntimeDocker,
			Image: "ubuntu:24.04",
		},
	}
	body, _ := json.Marshal(manifest)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	require.Equal(t, http.StatusCreated, rec.Code)

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/skills/acme/boom/invoke", bytes.NewReader([]byte(`{}`)))
	req2.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, p))
	assert.Equal(t, http.StatusBadGateway, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "boom")
}

// runnerFunc lets a test supply an ad-hoc runner without defining a
// new type for each case.
type runnerFunc struct {
	fn func(ctx context.Context, req runtime.Request) (runtime.Result, error)
}

func (r *runnerFunc) Run(ctx context.Context, req runtime.Request) (runtime.Result, error) {
	return r.fn(ctx, req)
}

func TestInvalidManifestRejected(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())

	bad := models.SkillManifest{
		Namespace: "Bad Namespace!",
		Name:      "hello",
		Version:   "not-semver",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "x"},
	}
	body, _ := json.Marshal(bad)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, p))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	s := newTestServer(t)
	p := auth.PrincipalForOrg(uuid.New())

	// Initialize doesn't require a principal.
	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"protocolVersion"`)

	// Register a skill (as principal p), then list tools (as same principal).
	manifest := models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12-slim"},
		Inputs:    map[string]models.IOField{"name": {Type: "string", Required: true}},
	}
	body, _ := json.Marshal(manifest)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, p))
	require.Equal(t, http.StatusCreated, rec2.Code)

	listReq := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(listReq))
	req3.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec3, withPrincipal(req3, p))
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Contains(t, rec3.Body.String(), `"acme/hello"`)
}

func TestNoAuthDevModeServesRequests(t *testing.T) {
	// When the server is started without an auth Service (no DB),
	// /v1 endpoints must still work — the route group injects an
	// anonymous principal so handlers don't return 401.
	s := newTestServer(t)

	manifest := models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12-slim"},
	}
	body, _ := json.Marshal(manifest)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No principal injected — middleware should fall back to anonymous.
	s.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello", nil)
	s.Handler().ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestTenantIsolation(t *testing.T) {
	// Skills registered by org A must not be visible to org B.
	s := newTestServer(t)
	orgA := auth.PrincipalForOrg(uuid.New())
	orgB := auth.PrincipalForOrg(uuid.New())

	manifest := models.SkillManifest{
		Namespace: "acme",
		Name:      "hello",
		Version:   "0.1.0",
		Runtime:   models.Runtime{Type: models.RuntimeDocker, Image: "python:3.12-slim"},
	}
	body, _ := json.Marshal(manifest)

	// org A creates the skill.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, withPrincipal(req, orgA))
	require.Equal(t, http.StatusCreated, rec.Code)

	// org B cannot see it.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/skills/acme/hello", nil)
	s.Handler().ServeHTTP(rec2, withPrincipal(req2, orgB))
	assert.Equal(t, http.StatusNotFound, rec2.Code)

	// org B's list is empty.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	s.Handler().ServeHTTP(rec3, withPrincipal(req3, orgB))
	require.Equal(t, http.StatusOK, rec3.Code)
	var resp struct {
		Skills []models.SkillManifest `json:"skills"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &resp))
	assert.Empty(t, resp.Skills)
}
