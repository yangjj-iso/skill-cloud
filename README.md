# Skill Cloud

> **Skill Cloud** is a platform that hosts remote skills callable by local agents.
> Think of it as *"npm / Hugging Face Hub" for agent capabilities*.

[![CI](https://github.com/yangjj-iso/skill-cloud/actions/workflows/ci.yml/badge.svg)](https://github.com/yangjj-iso/skill-cloud/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A local agent (Claude Desktop, Cursor, Devin, or your own) does **not** need to ship
every tool inside its own process. Instead, it discovers skills published to
Skill Cloud and calls them remotely over HTTP / MCP.

## Features

- **Skill Registry** — publish, version (semver), tag, search skills.
- **Two runtime modes** — upload code (executed in a Docker sandbox by the platform) **or** register an external HTTP endpoint (platform proxies the call).
- **MCP-compatible** — a built-in [Model Context Protocol](https://modelcontextprotocol.io) server exposes every registered skill as an MCP tool, so any MCP-capable agent works out of the box.
- **Multi-tenant** — `org → user → api_key` model with row-level `org_id` isolation.
- **Two SDKs** — Python and TypeScript.
- **Local first** — `docker compose up` brings up the full platform (server + Postgres + MinIO).

## Architecture (high level)

```
┌─────────────────┐       ┌──────────────────────────────────────┐
│ Local Agent     │       │            Skill Cloud Platform       │
│ (Devin/Cursor/  │       │                                       │
│  Claude/own)    │       │   ┌─────────────┐    ┌────────────┐  │
│                 │ HTTPS │   │  API (Go)   │───▶│  Postgres  │  │
│  ┌───────────┐  │       │   │  REST + MCP │    └────────────┘  │
│  │  SDK      │──┼──────▶│   └──────┬──────┘                    │
│  └───────────┘  │       │          │           ┌────────────┐  │
└─────────────────┘       │          │──────────▶│  MinIO/S3  │  │
                          │          ▼           └────────────┘  │
                          │   ┌─────────────┐                    │
                          │   │  Runtime    │  docker sandbox    │
                          │   │  Dispatcher │  or http_proxy     │
                          │   └─────────────┘                    │
                          └──────────────────────────────────────┘
```

See [`docs/requirements.md`](docs/requirements.md) for the full requirements document
and [`docs/architecture.md`](docs/architecture.md) for the system design.

## Repository layout

```
skill-cloud/
├── server/                 # Go backend (gin) + MCP server
├── sdk/
│   ├── python/             # `pip install skill-cloud`
│   └── typescript/         # `npm i @skill-cloud/client`
├── cli/                    # `skill` CLI (publish, list, invoke)
├── examples/
│   ├── hello-skill/        # minimal docker-runtime skill
│   └── http-proxy-skill/   # external endpoint skill
├── docs/
├── docker-compose.yml
└── .github/workflows/      # CI
```

## Quick start

> Requires Docker and Docker Compose v2.

```bash
git clone https://github.com/yangjj-iso/skill-cloud.git
cd skill-cloud
docker compose up -d
# Server: http://localhost:8080
# MinIO console: http://localhost:9001 (minioadmin / minioadmin)
```

Create an org + API key, then call a skill from Python:

```python
from skill_cloud import Client

client = Client(base_url="http://localhost:8080", api_key="...")
print(client.call("acme/hello", name="world"))
# -> {"message": "hello, world"}
```

Or from TypeScript:

```ts
import { Client } from "@skill-cloud/client";

const client = new Client({ baseUrl: "http://localhost:8080", apiKey: "..." });
console.log(await client.call("acme/hello", { name: "world" }));
```

Or from the `skill` CLI:

```bash
skill login --host http://localhost:8080 --api-key ...
skill init hello --namespace acme
(cd hello && docker build -t acme/hello:0.1.0 . && skill push)
skill call acme/hello --input '{"name":"world"}'
skill logs acme/hello
```

See [`docs/cli-demo.md`](docs/cli-demo.md) for the full end-to-end walkthrough.

## Skill Manifest

Every skill is described by a `skill.yaml`. See [`docs/manifest.md`](docs/manifest.md)
for the full spec; minimal example:

```yaml
name: hello
namespace: acme
version: 0.1.0
description: "Say hello."
runtime:
  type: docker
  image: python:3.12-slim
  entrypoint: "python -m hello"
  timeout_seconds: 10
inputs:
  name:
    type: string
    required: true
outputs:
  message:
    type: string
```

## Status

**Alpha.** Multi-tenant registry, audit logging, anti-theft (M0–M1.5),
real Docker / HTTP-proxy runtime dispatch (M2), and the `skill` CLI
with an end-to-end demo (M3) are landed. Streaming, async, Web UI,
and Prometheus metrics are tracked in M4 — see
[`docs/roadmap.md`](docs/roadmap.md).

## License

MIT — see [LICENSE](LICENSE).
