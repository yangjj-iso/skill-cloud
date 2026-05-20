package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCLITest spins up an httptest server with the supplied handler
// and points the CLI at it via SKILLCLOUD_* env vars. It also redirects
// $SKILLCLOUD_CONFIG to a tempfile so the test can't accidentally read
// or write the operator's real ~/.skillcloud/config.yaml.
func setupCLITest(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfgDir := t.TempDir()
	t.Setenv("SKILLCLOUD_CONFIG", filepath.Join(cfgDir, "config.yaml"))
	t.Setenv("SKILLCLOUD_HOST", srv.URL)
	t.Setenv("SKILLCLOUD_API_KEY", "test-key")
	return srv.URL
}

// runRoot constructs a fresh root command, captures stdout+stderr, and
// returns (stdout, stderr, err). Building the root each call avoids
// global flag state leaking between tests.
func runRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	// reset shared globals so tests don't leak --host/--api-key.
	Globals = rootOpts{}
	root := NewRoot()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	root.SetContext(context.Background())
	err := root.Execute()
	return out.String(), errBuf.String(), err
}

func TestListPrintsTableOfSkills(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skills": []map[string]any{
				{"namespace": "demo", "name": "hello", "version": "0.1.0", "runtime": map[string]any{"type": "docker"}, "description": "say hi"},
				{"namespace": "demo", "name": "echo", "version": "0.2.0", "runtime": map[string]any{"type": "http_proxy"}, "description": ""},
			},
		})
	})
	out, _, err := runRoot(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"NAMESPACE/NAME", "demo/hello", "demo/echo", "0.1.0", "docker", "http_proxy"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

func TestListEmptyShowsHelpfulMessage(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	out, _, err := runRoot(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "no skills registered") {
		t.Fatalf("expected empty-state hint, got:\n%s", out)
	}
}

func TestCallSendsInputAndPrintsResult(t *testing.T) {
	var gotInput map[string]any
	setupCLITest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills/demo/hello/invoke" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotInput)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skill":  "demo/hello",
			"status": "ok",
			"output": map[string]any{"message": "hello, world"},
		})
	})
	out, _, err := runRoot(t, "call", "demo/hello", "--input", `{"name":"world"}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotInput["name"] != "world" {
		t.Fatalf("server didn't receive input, got %+v", gotInput)
	}
	if !strings.Contains(out, `"status": "ok"`) || !strings.Contains(out, `"message": "hello, world"`) {
		t.Fatalf("output missing fields:\n%s", out)
	}
}

func TestCallStatusErrorMakesCommandFail(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skill":  "demo/hello",
			"status": "error",
			"error":  "skill exited 1",
		})
	})
	_, _, err := runRoot(t, "call", "demo/hello", "--input", "{}")
	if err == nil {
		t.Fatal("expected error when invocation status != ok")
	}
}

func TestCallRejectsMalformedQualifiedName(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })
	_, _, err := runRoot(t, "call", "no-slash", "--input", "{}")
	if err == nil {
		t.Fatal("expected error for missing namespace separator")
	}
}

func TestLogsPrintsHeaderAndRow(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills/demo/hello/logs" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invocations": []map[string]any{
				{"status": "ok", "latency_ms": 42, "input_bytes": 9, "output_bytes": 17, "caller_ip": "10.0.0.5", "started_at": "2026-01-01T00:00:00Z"},
			},
		})
	})
	out, _, err := runRoot(t, "logs", "demo/hello")
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	for _, want := range []string{"STARTED", "ok", "42ms", "9/17", "10.0.0.5"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestStatsRendersJSON(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 7, "last_24h": 3})
	})
	out, _, err := runRoot(t, "stats", "demo/hello")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !strings.Contains(out, `"total": 7`) {
		t.Fatalf("output should include total: %s", out)
	}
}

func TestPushReadsManifestAndPostsIt(t *testing.T) {
	var receivedBody []byte
	setupCLITest(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = readAll(r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespace": "demo",
			"name":      "hello",
			"version":   "0.1.0",
		})
	})

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "skill.yaml")
	yaml := []byte("namespace: demo\nname: hello\nversion: 0.1.0\nruntime:\n  type: docker\n  image: demo/hello:0.1.0\n")
	if err := os.WriteFile(manifestPath, yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := runRoot(t, "push", "--file", manifestPath)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !strings.Contains(out, "published demo/hello@0.1.0") {
		t.Fatalf("output should confirm publish: %s", out)
	}
	if !strings.Contains(string(receivedBody), `"namespace":"demo"`) {
		t.Fatalf("server didn't receive JSON manifest, got %s", string(receivedBody))
	}
}

func TestLoginRejectsMissingAPIKey(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_, _, err := runRoot(t, "login")
	if err == nil {
		t.Fatal("expected login to fail without --api-key")
	}
}

func TestLoginSavesConfigWhenHealthzPasses(t *testing.T) {
	url := setupCLITest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	_, _, err := runRoot(t, "login", "--host", url, "--api-key", "fresh-key")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	path := os.Getenv("SKILLCLOUD_CONFIG")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(raw), "fresh-key") {
		t.Fatalf("config missing api_key: %s", string(raw))
	}
}

func TestInitScaffoldsTargetDirectory(t *testing.T) {
	setupCLITest(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	target := filepath.Join(t.TempDir(), "hello")
	_, _, err := runRoot(t, "init", "hello", "--dir", target, "--namespace", "demo")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, rel := range []string{"skill.yaml", "Dockerfile", "app/main.py", "README.md"} {
		if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}

func readAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
