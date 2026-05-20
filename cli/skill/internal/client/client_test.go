package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key", srv.Client())
}

func TestPublishSkillSendsManifestAndDecodesResponse(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespace": gotBody["namespace"],
			"name":      gotBody["name"],
			"version":   gotBody["version"],
			"runtime":   map[string]any{"type": "docker"},
		})
	})

	got, err := c.PublishSkill(context.Background(), map[string]any{
		"namespace": "demo",
		"name":      "hello",
		"version":   "0.1.0",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got.Namespace != "demo" || got.Name != "hello" || got.Version != "0.1.0" {
		t.Fatalf("unexpected response: %+v", got)
	}
	if gotBody["name"] != "hello" {
		t.Fatalf("server didn't receive manifest, got %+v", gotBody)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q, want Bearer test-key", gotAuth)
	}
}

func TestListSkillsEmptyResponseReturnsNonNilSlice(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	got, err := c.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestInvokeDecodesInvokeFailureBody(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway) // dispatcher returned StatusError
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skill":  "demo/hello",
			"status": "error",
			"error":  "skill exited 1: boom",
		})
	})
	res, err := c.Invoke(context.Background(), "demo", "hello", map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("status = %q, want error", res.Status)
	}
	if res.Error != "skill exited 1: boom" {
		t.Fatalf("error mismatch: %s", res.Error)
	}
}

func TestNon2xxYieldsRemoteError(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	})
	_, err := c.ListSkills(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var re *RemoteError
	if !errors.As(err, &re) {
		t.Fatalf("expected RemoteError, got %T: %v", err, err)
	}
	if re.Status != http.StatusUnauthorized {
		t.Fatalf("status = %d", re.Status)
	}
	if !strings.Contains(re.Body, "bad token") {
		t.Fatalf("body should be captured: %q", re.Body)
	}
	if !IsAuthError(err) {
		t.Fatal("IsAuthError should be true for 401")
	}
}

func TestHealthzNotOKErrors(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if err := c.Healthz(context.Background()); err == nil {
		t.Fatal("expected healthz error on non-200")
	}
}

func TestListLogsAndStats(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/skills/demo/hello/logs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"invocations": []map[string]any{
					{"status": "ok", "latency_ms": 12, "input_bytes": 5, "output_bytes": 7, "caller_ip": "1.2.3.4"},
				},
			})
		case "/v1/skills/demo/hello/stats":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total":    7,
				"last_24h": 3,
			})
		default:
			http.NotFound(w, r)
		}
	})
	logs, err := c.ListLogs(context.Background(), "demo", "hello")
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Status != "ok" || logs[0].CallerIP != "1.2.3.4" {
		t.Fatalf("unexpected logs: %+v", logs)
	}
	stats, err := c.GetStats(context.Background(), "demo", "hello")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 7 || stats.Last24h != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
