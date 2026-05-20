# Skill Cloud — Architecture

## Components

| Component | Tech | Responsibility |
|---|---|---|
| **API server** | Go 1.23 + gin | REST + MCP endpoints, auth, routing, validation |
| **Registry** | Postgres 16 | Stores orgs, users, API keys, skills, versions, invocations |
| **Object storage** | MinIO (dev) / S3 (prod) | Stores uploaded skill code archives |
| **Runtime dispatcher** | Go module | Decides whether to run a skill via `docker` or `http_proxy`; collects logs |
| **Docker sandbox** | Docker daemon | One-shot container per invocation with CPU/mem/timeout limits |
| **HTTP proxy** | Go module | Forwards inputs to externally hosted skill endpoints |
| **MCP adapter** | Go module | Maps registered skills to MCP `tools/*` JSON-RPC methods |
| **SDKs** | Python (httpx), TS (fetch) | Thin HTTP clients |
| **CLI** | Cobra (Go) | `skill init / push / list / call / logs` |

## Request flow — REST invocation

```
Local Agent
  │  POST /v1/skills/acme/hello/invoke
  ▼
API Server
  ├─ Auth: resolve api_key → org/user
  ├─ Rate-limit: 60 req/min/api_key default (sliding window, 429 with Retry-After)
  ├─ Registry: load skill manifest (org-scoped)
  └─ Dispatcher.Run(skill, input)
        ├─ runtime.type == docker  → DockerRunner
        │     │  docker run --rm -i \
        │     │             --network=none --read-only --cap-drop ALL \
        │     │             --security-opt no-new-privileges \
        │     │             --user nobody --pids-limit 128 \
        │     │             --memory <manifest.memory_mb>m --cpus 1.0 \
        │     │             [--entrypoint <manifest.entrypoint>] \
        │     │             <manifest.image>
        │     │  ← stdin: input JSON
        │     │  → stdout: output JSON (capped at 1 MiB)
        │     │  context.WithTimeout(manifest.timeout_seconds) → SIGKILL on deadline
        │     │  When the server itself runs inside docker-compose, the
        │     │  binary talks to the host's docker daemon over the
        │     │  bind-mounted /var/run/docker.sock; skill containers
        │     │  therefore spawn as **siblings** of the server, not nested
        │     │  inside it.
        │     ▼
        └─ runtime.type == http_proxy  → HTTPProxy
              │  POST manifest.url, Content-Type: application/json
              │  context.WithTimeout(manifest.timeout_seconds)
              │  response body capped at 1 MiB; non-2xx → status=error
              ▼
  ◀── { skill, status, output, error? }
       status=ok    → HTTP 200
       status=error → HTTP 502 Bad Gateway (body still carries error string)
       status=timeout → HTTP 504 Gateway Timeout
       always writes one invocation row: org/user/api_key, status,
       latency_ms, input_bytes, output_bytes, caller_ip, user_agent,
       error_message
```

## Request flow — MCP

The `/mcp` endpoint speaks JSON-RPC 2.0. After `initialize`, the client calls
`tools/list` and receives every skill (filtered by org, with runtime details
redacted — see *Anti-theft* below). When the client calls `tools/call` with
`name = "acme/hello"`, the handler routes through the **same runtime
dispatcher** as the REST path, so behaviour, logging, and quotas are identical.
A non-`ok` dispatcher result is surfaced as `isError: true` in the MCP
response, with the error message as the `text` content; the JSON-RPC envelope
itself still returns 200 so the MCP client can render the failure inline.

## Multi-tenancy

- Top-level resources are scoped by `org_id`.
- `users` belong to one or more orgs (membership table).
- `api_keys` belong to one user **and** carry an `org_id` (their effective scope).
- Every read/write goes through a tenant-aware query helper that enforces the
  `org_id` filter — no row can leak across orgs.
- `skills` carry `(namespace, name)`. `namespace` defaults to the org's slug but
  can be any string the org owns (claimed namespaces table).

## Schema

Implemented in `server/internal/db/migrations/*.sql`. Tables are applied
on every startup via an embedded migration runner
(`server/internal/db/db.go`).

```
orgs           (id, slug, name, created_at)
users          (id, email, created_at)
org_members    (org_id, user_id, role)
api_keys       (id, org_id, user_id, prefix, hash, name, last_used_at, created_at)
                                                UNIQUE (prefix)  -- prevents silent collisions
skills         (id, org_id, namespace, name, description, latest_version,
                created_at, updated_at)         UNIQUE (org_id, namespace, name)
skill_versions (id, skill_id, version, manifest, storage_key, created_at)
                                                UNIQUE (skill_id, version)
invocations    (id, org_id, user_id, api_key_id, skill_id, version, status,
                input, output, error_message,
                caller_ip, user_agent, input_bytes, output_bytes,
                started_at, finished_at, latency_ms)
```

## Authentication

- Bootstrap endpoints (`POST /v1/auth/orgs`, `/v1/auth/users`,
  `/v1/auth/api_keys`) create the first tenant and issue an API key.
- API keys are returned in the form `<prefix>.<secret>` exactly once at
  creation time. Only the prefix and a bcrypt hash of the secret are
  stored; the plaintext secret is never persisted.
- Every request to `/v1/*` and `/mcp` carries
  `Authorization: Bearer <prefix>.<secret>`. The middleware looks up the
  key by `prefix`, bcrypt-verifies the secret, and injects a
  `Principal{OrgID, UserID, APIKeyID}` into the request context.
- Handlers read the principal from context and pass `OrgID` to the
  registry so every read/write is row-level scoped to the caller's org.

## Anti-theft / abuse defences

- **Manifest projection.** `/v1/skills` (list + get) and the MCP
  `tools/list` response only return `name`, `description`, `inputSchema`,
  and the runtime *type* (so callers know whether they're hitting a
  container or an HTTP proxy). The fields that would let a third party
  reproduce the skill — `runtime.image`, `runtime.entrypoint`,
  `runtime.url`, `runtime.env`, `runtime.cmd` — are stripped.
- **Dedicated runtime endpoint.** Owners (i.e. callers whose principal
  matches the skill's `org_id`) read implementation details from
  `GET /v1/skills/:ns/:name/runtime`. The registry is already org-scoped,
  so a successful lookup here doubles as the authorization check.
- **Per-key rate limit.** Default 60 req/min/api_key, configurable via
  `SKILLCLOUD_RATE_LIMIT`. Excess requests return `429` plus
  `Retry-After` and `X-RateLimit-{Limit,Remaining,Reset}` headers. The
  current implementation is an in-process sliding window — acceptable
  for single-replica deployments and trivially swappable for Redis when
  we go multi-replica.
- **Caller-IP capture.** A startup-time switch (`SKILLCLOUD_TRUST_PROXY`)
  controls whether `X-Forwarded-For` / `X-Real-IP` are honoured.
  Direct-exposure deployments leave it off so spoofed headers can't
  rewrite the audit trail; only deployments behind a trusted load
  balancer turn it on.
- **Per-invocation audit row.** Every REST `/invoke` and MCP
  `tools/call` writes one row to `invocations` with `org_id`, `user_id`,
  `api_key_id`, `skill_id`, `version`, `status`, `latency_ms`,
  `input_bytes`, `output_bytes`, `caller_ip`, and `user_agent`. The
  `(org_id, skill_id, started_at DESC)` index keeps per-skill stat
  queries cheap.
- **Stats endpoint.** `GET /v1/skills/:ns/:name/stats` exposes total
  call count, 24h call count, last invocation time, and last caller IP
  to the owning org so abuse patterns can be spotted by hand and / or
  surfaced in a future dashboard.

## Sandbox security (MVP)

- Run as `--user nobody`, `--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges`.
- `--network=none` by default; per-skill manifest may declare allowed hosts (a
  small egress proxy whitelists those domains).
- Hard limits: `--memory`, `--cpus`, `--pids-limit`, timeout enforced by the
  dispatcher (not just by docker), output size cap.
- Secrets are passed via env vars and never logged.

## Observability

- Structured JSON logs (zerolog or slog).
- Prometheus `/metrics` endpoint (unauthenticated by design; restrict
  at the network layer in production). Series:
  - `skillcloud_invocations_total{org,namespace,name,status}` — counter
  - `skillcloud_invocation_latency_seconds{org,namespace,name}` — histogram
  - `skillcloud_rate_limit_dropped_total{api_key_prefix}` — counter
  - `skillcloud_skills_registered{org,runtime_type}` — gauge
  - `skillcloud_http_requests_total{method,route,status_code}` — counter
- The bundled `docker-compose.yml` runs Prometheus + Grafana with a
  pre-provisioned datasource and the "Skill Cloud — overview" dashboard.
  See [`observability.md`](observability.md) for the full reference.
- OpenTelemetry traces for `request → registry → dispatcher → runtime` (P1).
