package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDockerScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hello")
	err := Render(dir, Params{Namespace: "demo", Name: "hello", Runtime: "docker"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := []string{
		"skill.yaml",
		"README.md",
		"Dockerfile",
		"app/main.py",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	manifest := mustRead(t, filepath.Join(dir, "skill.yaml"))
	if !strings.Contains(manifest, "namespace: demo") || !strings.Contains(manifest, "name: hello") {
		t.Fatalf("manifest missing namespace/name: %s", manifest)
	}
	if !strings.Contains(manifest, "type: docker") {
		t.Fatalf("manifest runtime should be docker: %s", manifest)
	}
}

func TestRenderHTTPProxySkipsDockerfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proxy")
	err := Render(dir, Params{Namespace: "demo", Name: "proxy", Runtime: "http_proxy"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); !os.IsNotExist(err) {
		t.Errorf("http_proxy scaffold should not include Dockerfile, stat err=%v", err)
	}
	manifest := mustRead(t, filepath.Join(dir, "skill.yaml"))
	if !strings.Contains(manifest, "type: http_proxy") {
		t.Fatalf("manifest runtime should be http_proxy: %s", manifest)
	}
	if !strings.Contains(manifest, "url:") {
		t.Fatalf("http_proxy manifest must have url: %s", manifest)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
