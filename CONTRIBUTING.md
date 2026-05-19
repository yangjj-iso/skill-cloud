# Contributing

Thanks for your interest in Skill Cloud! The repo is a monorepo with three
independently buildable pieces.

## Layout

- `server/` — Go 1.23 backend (gin + MCP)
- `sdk/python/` — Python SDK
- `sdk/typescript/` — TypeScript SDK
- `examples/` — sample skills
- `docs/` — design and reference docs

## Prerequisites

- Go ≥ 1.23
- Python ≥ 3.9
- Node ≥ 18
- Docker + Docker Compose v2

## Common commands

### Server

```bash
cd server
make build    # go build ./...
make test     # go test ./... -race
make run      # start server on :8080
```

### Python SDK

```bash
cd sdk/python
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
pytest -q
ruff check .
```

### TypeScript SDK

```bash
cd sdk/typescript
npm install
npm run build
npm test
```

### Full platform

```bash
docker compose up -d
curl http://localhost:8080/healthz
```

## Code style

- Go: `gofmt` + `go vet` (enforced in CI).
- Python: `ruff` (enforced in CI), type hints encouraged.
- TypeScript: `tsc --noEmit` (enforced in CI), strict mode on.

## Tests

CI runs all three jobs on every push and PR (`.github/workflows/ci.yml`). Please
add tests for new behavior — the existing suites in `server/internal/...` and
`sdk/*/tests/` are good templates.
