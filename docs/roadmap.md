# Roadmap

## Milestones

### M0 — Scaffolding (this PR)

- [x] Monorepo layout (`server/`, `sdk/python/`, `sdk/typescript/`, `examples/`, `docs/`)
- [x] Go server skeleton with `/healthz`, `/v1/skills` CRUD, `/v1/skills/{ns}/{name}/invoke` (stub), `/mcp` (initialize / tools/list / tools/call stub)
- [x] In-memory registry + manifest validation + unit tests
- [x] Python SDK + tests
- [x] TypeScript SDK + tests
- [x] `docker-compose.yml` (server + Postgres + MinIO)
- [x] GitHub Actions CI (Go + Python + TS)
- [x] Architecture, manifest, and requirements docs

### M1 — Persistence + auth (next)

- [ ] Postgres schema + migrations (sqlc or goose)
- [ ] `orgs` / `users` / `api_keys` / `skills` / `skill_versions` tables
- [ ] Replace `InMemory` registry with Postgres-backed one
- [ ] API key auth middleware + row-level `org_id` filtering
- [ ] `POST /v1/auth/orgs`, `POST /v1/auth/api_keys`

### M2 — Real runtime dispatch

- [ ] `runtime/docker_runner.go` — one-shot container w/ resource limits, stdin/stdout JSON contract
- [ ] `runtime/http_proxy.go` — forward + retry + timeout
- [ ] Skill code upload → MinIO; layer it into the container at run time
- [ ] Invocation log written to Postgres

### M3 — CLI + end-to-end

- [ ] `skill` CLI (Cobra): `init`, `push`, `list`, `call`, `logs`
- [ ] `examples/hello-skill` end-to-end: push → list → call → see log
- [ ] MCP `tools/call` routes through the real dispatcher

### M4 — Hardening / P1

- [ ] Streaming output (SSE)
- [ ] Async invocations + webhook callbacks
- [ ] Rate limits / quotas
- [ ] Prometheus metrics + OpenTelemetry
- [ ] Web UI (Next.js)
