// Package template renders the scaffolded skill project that
// `skill init` produces. Keeping the renderer in its own package lets
// the cli's unit tests assert on the file shape without invoking
// Cobra.
package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Params drives the renderer.
type Params struct {
	Namespace string
	Name      string
	// Runtime is one of "docker" (default) or "http_proxy". Anything
	// else is treated as "docker" — the manifest carries the literal
	// string so the user can edit it post-hoc anyway.
	Runtime string
}

// Render writes every file of the skill scaffold under `dir`. It
// creates `dir` (and any missing parents) and returns an error on the
// first failure without partial cleanup — the caller is expected to
// have asserted that dir doesn't already exist.
func Render(dir string, p Params) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	files := buildFiles(p)
	for _, f := range files {
		path := filepath.Join(dir, f.relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

type file struct {
	relPath string
	content string
}

func buildFiles(p Params) []file {
	manifest := dockerManifest(p)
	if strings.EqualFold(p.Runtime, "http_proxy") {
		manifest = httpProxyManifest(p)
	}
	files := []file{
		{relPath: "skill.yaml", content: manifest},
		{relPath: "README.md", content: readme(p)},
	}
	if !strings.EqualFold(p.Runtime, "http_proxy") {
		files = append(files,
			file{relPath: "Dockerfile", content: dockerfile()},
			file{relPath: "app/main.py", content: pythonEntrypoint()},
		)
	}
	return files
}

func dockerManifest(p Params) string {
	return fmt.Sprintf(`namespace: %s
name: %s
version: 0.1.0
description: %s
runtime:
  type: docker
  image: %s/%s:0.1.0
  timeout_seconds: 30
  memory_mb: 256
inputs:
  name:
    type: string
    description: Who to greet
    required: true
outputs:
  message:
    type: string
    description: Greeting returned by the skill
`, p.Namespace, p.Name, fmt.Sprintf("a sample skill called %s", p.Name), p.Namespace, p.Name)
}

func httpProxyManifest(p Params) string {
	return fmt.Sprintf(`namespace: %s
name: %s
version: 0.1.0
description: %s
runtime:
  type: http_proxy
  url: https://example.com/%s
  timeout_seconds: 30
inputs:
  name:
    type: string
    description: Who to greet
    required: true
outputs:
  message:
    type: string
    description: Greeting returned by the upstream service
`, p.Namespace, p.Name, fmt.Sprintf("a sample proxy skill called %s", p.Name), p.Name)
}

func readme(p Params) string {
	return fmt.Sprintf(`# %s/%s

Scaffolded by `+"`skill init`"+`. Edit `+"`skill.yaml`"+` and the source under `+"`app/`"+` to your taste.

## Develop

`+"```bash"+`
# Iterate locally — reads JSON on stdin, writes JSON on stdout
echo '{"name":"world"}' | python app/main.py
`+"```"+`

## Publish

`+"```bash"+`
docker build -t %s/%s:0.1.0 .
skill push
`+"```"+`

## Invoke

`+"```bash"+`
skill call %s/%s --input '{"name":"world"}'
skill logs %s/%s
`+"```"+`
`, p.Namespace, p.Name, p.Namespace, p.Name, p.Namespace, p.Name, p.Namespace, p.Name)
}

func dockerfile() string {
	return `FROM python:3.12-slim
WORKDIR /app
COPY app/ /app/
USER nobody
ENTRYPOINT ["python", "/app/main.py"]
`
}

func pythonEntrypoint() string {
	return `"""Skill entrypoint: read JSON from stdin, write JSON to stdout.

The Skill Cloud docker runtime pipes the caller's input as one JSON
document on stdin and expects one JSON document back on stdout. Stderr
is captured for diagnostics but is NOT returned to the caller.
"""

import json
import sys


def main() -> None:
    raw = sys.stdin.read() or "{}"
    payload = json.loads(raw)
    name = payload.get("name", "world")
    json.dump({"message": f"hello, {name}"}, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
`
}
