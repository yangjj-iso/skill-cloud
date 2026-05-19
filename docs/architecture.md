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

## Request flow — REST invocation (docker runtime)

```
Local Agent
  │  POST /v1/skills/acme/hello/invoke
  ▼
API Server
  ├─ Auth: resolve api_key → org/user
  ├─ Registry: load skill manifest + latest version
  ├─ Validate input against manifest.inputs schema
  └─ Dispatch to Runtime
        ├─ runtime.type == docker
        │     │  pull image (cached) + start container with
        │     │  --network=none|whitelist, --memory, --cpus, --rm
        │     │  pipe inputs as JSON to stdin
        │     │  read JSON from stdout (up to size limit)
        │     │  kill on timeout
        │     ▼
        └─ runtime.type == http_proxy
              │  POST to runtime.url with input JSON
              │  timeout + bounded retries on idempotent failures
              ▼
  ◀── 200 OK { output: ..., status: ok, invocation_id }
       writes invocation row to Postgres (input, output, latency, status)
```

## Request flow — MCP

The `/mcp` endpoint speaks JSON-RPC 2.0. After `initialize`, the client calls
`tools/list` and receives every skill (filtered by org). When the client calls
`tools/call` with `name = "acme/hello"`, the handler routes through the **same
runtime dispatcher** as the REST path, so behavior, logging, and quotas are
identical.

## Multi-tenancy

- Top-level resources are scoped by `org_id`.
- `users` belong to one or more orgs (membership table).
- `api_keys` belong to one user **and** carry an `org_id` (their effective scope).
- Every read/write goes through a tenant-aware query helper that enforces the
  `org_id` filter — no row can leak across orgs.
- `skills` carry `(namespace, name)`. `namespace` defaults to the org's slug but
  can be any string the org owns (claimed namespaces table).

## Schema

Implemented in `server/internal/db/migrations/001_initial.sql`. Tables
are applied on every startup via an embedded migration runner
(`server/internal/db/db.go`).

```
orgs           (id, slug, name, created_at)
users          (id, email, created_at)
org_members    (org_id, user_id, role)
api_keys       (id, org_id, user_id, prefix, hash, name, last_used_at, created_at)
skills         (id, org_id, namespace, name, description, latest_version,
                created_at, updated_at)         UNIQUE (org_id, namespace, name)
skill_versions (id, skill_id, version, manifest, storage_key, created_at)
                                                UNIQUE (skill_id, version)
invocations    (id, org_id, user_id, skill_id, version, status,
                input, output, error_message, started_at, finished_at, latency_ms)
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

## Sandbox security (MVP)

- Run as `--user nobody`, `--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges`.
- `--network=none` by default; per-skill manifest may declare allowed hosts (a
  small egress proxy whitelists those domains).
- Hard limits: `--memory`, `--cpus`, `--pids-limit`, timeout enforced by the
  dispatcher (not just by docker), output size cap.
- Secrets are passed via env vars and never logged.

## Observability

- Structured JSON logs (zerolog or slog).
- Prometheus `/metrics` (request count / latency, invocation count / latency by
  status, per-runtime dispatch latency).
- OpenTelemetry traces for `request → registry → dispatcher → runtime` (P1).
