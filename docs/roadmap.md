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

### M1 — Persistence + auth

- [x] Postgres schema + embedded migrations (pgx + `embed.FS`)
- [x] `orgs` / `users` / `org_members` / `api_keys` / `skills` / `skill_versions` / `invocations` tables
- [x] Replace `InMemory` registry with Postgres-backed one (org-scoped queries)
- [x] API key auth middleware + row-level `org_id` filtering
- [x] `POST /v1/auth/orgs`, `POST /v1/auth/users`, `POST /v1/auth/api_keys` bootstrap endpoints
- [x] CI runs Go unit + Postgres integration tests

### M1.5 — Invocation auditing + anti-theft

- [x] `002_invocations_logging.sql` adds `caller_ip`, `user_agent`,
      `input_bytes`, `output_bytes`, `api_key_id` columns and a
      `(org_id, skill_id, started_at DESC)` index
- [x] Every `/v1/.../invoke` and MCP `tools/call` writes an invocation
      row (org_id, user_id, api_key_id, skill, version, status,
      latency, payload sizes, caller IP, user-agent)
- [x] `GET /v1/skills/:ns/:name/stats` returns total / 24h call counts,
      `last_invoked_at`, `last_caller_ip` (org-scoped)
- [x] Caller-IP middleware honours `X-Forwarded-For` /
      `X-Real-IP` only when `SKILLCLOUD_TRUST_PROXY=true`
- [x] Per-API-key sliding-window rate limit (default 60 req/min) with
      `X-RateLimit-Limit` / `X-RateLimit-Remaining` /
      `X-RateLimit-Reset` / `Retry-After` headers and 429 response
- [x] Manifest projection: `/v1/skills` list/get and MCP `tools/list`
      strip `runtime.image` / `entrypoint` / `url`; runtime details are
      only available to the owning org via
      `GET /v1/skills/:ns/:name/runtime`

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
