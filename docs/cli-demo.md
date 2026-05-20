# `skill` CLI — end-to-end walkthrough

This document walks through the full developer loop that M3 unlocks:

1. start the platform (one command)
2. provision an org + API key (curl, just to show the wire format)
3. `skill login` to save credentials
4. `skill init` to scaffold a hello-world skill
5. build the docker image
6. `skill push` to publish the manifest
7. `skill list` / `skill call` / `skill logs` / `skill stats` to exercise it

The whole flow runs against the docker-compose dev stack and takes ~2
minutes once the docker image cache is warm.

## 0. Prerequisites

- Docker 24+
- Go 1.23+ (only needed to build the CLI from source)
- `curl` and `jq`

## 1. Start the platform

```bash
git clone https://github.com/yangjj-iso/skill-cloud.git
cd skill-cloud
docker compose up -d
```

`docker compose up` starts:

- `server` on `http://localhost:8080`
- `postgres` on `localhost:5432` (database `skillcloud`)
- `minio` on `localhost:9000` (unused today, reserved for the M2
  source-upload follow-up)

The server container bind-mounts `/var/run/docker.sock` so the runtime
dispatcher can launch skill containers on the host's Docker daemon —
the spawned skill containers run as **siblings** of the server, not
nested inside it. This is convenient for local development but grants
the server container root-equivalent access to the host; do not deploy
the bundled compose file untouched to a shared host.

Wait for the server's healthcheck to pass:

```bash
curl -s http://localhost:8080/healthz
# {"status":"ok"}
```

## 2. Build the CLI

```bash
cd cli/skill
go build -o /usr/local/bin/skill ./cmd/skill
skill --help
```

The CLI is a single static Go binary; you can copy it anywhere on
`$PATH`. `go install ./cli/skill/cmd/skill@latest` once the module is
published will work too.

## 3. Bootstrap an org + API key

The platform doesn't ship a sign-up UI yet — operators use the
bootstrap endpoints directly. (M4 will add a Web UI.)

```bash
# Create an org. Both slug (URL-safe) and human name are required.
ORG_ID=$(curl -s -X POST http://localhost:8080/v1/auth/orgs \
  -H 'Content-Type: application/json' \
  -d '{"slug":"acme","name":"Acme"}' | jq -r .id)

# Create a user in that org.
USER_ID=$(curl -s -X POST http://localhost:8080/v1/auth/users \
  -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$ORG_ID\",\"email\":\"dev@acme.example\"}" | jq -r .id)

# Mint an API key. The plaintext `token` value is only shown once; save it.
TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/api_keys \
  -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$ORG_ID\",\"user_id\":\"$USER_ID\",\"name\":\"laptop\"}" | jq -r .token)
echo "TOKEN=$TOKEN"
```

## 4. `skill login`

```bash
skill login --host http://localhost:8080 --api-key "$TOKEN"
# server health check passes, then:
# saved credentials to /home/<you>/.skillcloud/config.yaml
```

`skill login` runs a `/healthz` probe so you don't save credentials
that point at a wrong host. The file is written with `0600`
permissions because it holds a bearer token.

Alternatively, skip `login` and set
`SKILLCLOUD_HOST` + `SKILLCLOUD_API_KEY` in the environment — useful
for CI / scripts.

## 5. `skill init`

```bash
skill init hello --namespace acme
# scaffolded acme/hello in /home/<you>/hello
# next steps:
#   cd hello
#   docker build -t acme/hello:0.1.0 .
#   skill push
```

The scaffold produces:

```
hello/
├── Dockerfile      # python:3.12-slim + your code under /app
├── README.md       # developer docs (how to iterate, publish, invoke)
├── app/
│   └── main.py     # reads JSON on stdin, writes JSON on stdout
└── skill.yaml      # manifest the server registers
```

`skill.yaml` is the platform's source of truth: namespace, name,
version, runtime config, input/output schema.

## 6. Build the image

```bash
cd hello
docker build -t acme/hello:0.1.0 .
```

The image only needs to be reachable from the docker daemon the
**server** uses. With `docker compose up`, that's the same daemon, so
a local `docker build` is enough. (When you run the server on a
separate host, push the image to a registry both can pull from. The
M2 follow-up will let `skill push` upload the source and have the
server build the image for you.)

## 7. `skill push`

From the scaffold directory:

```bash
skill push
# published acme/hello@0.1.0
```

`skill push` reads `./skill.yaml`, validates it locally, and POSTs it
to `/v1/skills`. The CLI also accepts `--file path/to/skill.yaml` if
you keep the manifest elsewhere.

## 8. `skill list`

```bash
skill list
# NAMESPACE/NAME  VERSION  RUNTIME  DESCRIPTION
# acme/hello      0.1.0    docker   a sample skill called hello
```

`skill list` only shows skills in your org. The runtime details
(`image`, `entrypoint`, …) are deliberately omitted from the public
view; see `docs/architecture.md` for the anti-theft rationale.

## 9. `skill call`

```bash
skill call acme/hello --input '{"name":"world"}'
# {
#   "skill": "acme/hello",
#   "status": "ok",
#   "output": {
#     "message": "hello, world"
#   }
# }
```

`skill call` reads input from `--input` (inline), `--input-file`
(path), or stdin (when piped). The CLI exits with a non-zero code
when `status != "ok"` so it composes well in shell pipelines.

A failure path you can trigger by stopping the docker daemon:

```bash
skill call acme/hello --input '{}'
# {
#   "skill": "acme/hello",
#   "status": "error",
#   "error": "docker run failed (exit 125): ..."
# }
# Error: invocation status: error
```

## 10. `skill logs` and `skill stats`

After a few calls:

```bash
skill logs acme/hello
# STARTED                STATUS  LATENCY  IN/OUT   CALLER_IP   ERROR
# 2026-05-19T20:18:11Z   ok      87ms     12/22    127.0.0.1
# 2026-05-19T20:18:14Z   error   45ms     2/0      127.0.0.1   skill exited 1: ...
```

```bash
skill stats acme/hello
# {
#   "total": 2,
#   "last_24h": 2,
#   "last_invoked_at": "2026-05-19T20:18:14Z",
#   "last_caller_ip": "127.0.0.1"
# }
```

`logs` and `stats` only ever return rows for the calling org — see
`server/internal/api/m15_test.go::TestSkillLogsScopedToOwningOrg` for
the regression.

## What's deliberately not in M3

- Source upload via MinIO — for now `skill push` only writes the
  manifest. You build & ship the docker image yourself.
- Streaming output — calls are request/response only.
- Async invocations / webhooks — same.
- Web UI — `skill` (CLI) and the REST/MCP endpoints are the only
  surfaces.

All four land in M4 / hardening — see [`docs/roadmap.md`](./roadmap.md).
