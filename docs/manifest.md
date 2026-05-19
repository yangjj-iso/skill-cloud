# Skill Manifest specification

A skill is described by a `skill.yaml` file at the root of its directory.

## Minimal example — docker runtime

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
  memory_mb: 256

inputs:
  name:
    type: string
    required: true

outputs:
  message:
    type: string
```

## Minimal example — http_proxy runtime

```yaml
name: weather
namespace: acme
version: 0.1.0
description: "Lookup weather via an external service."

runtime:
  type: http_proxy
  url: "https://example.com/api/weather"
  timeout_seconds: 15

inputs:
  city:
    type: string
    required: true

outputs:
  temperature_c: { type: number }
  conditions:   { type: string }
```

## Field reference

| Field | Required | Type | Notes |
|---|---|---|---|
| `name` | yes | string | matches `^[a-z0-9][a-z0-9_-]{0,62}$` |
| `namespace` | yes | string | same regex as `name`; must be claimed by the publisher's org |
| `version` | yes | string | semver `MAJOR.MINOR.PATCH` |
| `description` | no | string | shown in listings and MCP `tools/list` |
| `tags` | no | string[] | used by search |
| `runtime.type` | yes | enum | `docker` \| `http_proxy` |
| `runtime.image` | docker | string | container image (any registry) |
| `runtime.entrypoint` | docker | string | shell command run inside the container |
| `runtime.url` | http_proxy | string | absolute https URL |
| `runtime.timeout_seconds` | no | int | hard cap, defaults 30 |
| `runtime.memory_mb` | no | int | docker only; defaults 512 |
| `inputs.<name>` | no | object | JSON-schema-ish field (`type`, `description`, `required`, `default`) |
| `outputs.<name>` | no | object | same as `inputs` |
| `secrets` | no | string[] | env var names the platform must inject |
| `permissions.network` | no | string[] | allowed egress hosts (docker only) |

## Calling contract — docker runtime

- Inputs are written to the container's **stdin** as a single JSON object.
- Outputs MUST be written to **stdout** as a single JSON object.
- Anything on stderr is captured into the invocation log but is not returned to the caller.
- Exit code 0 = success. Non-zero = failure; stderr is surfaced as `error_message`.

## Calling contract — http_proxy runtime

- The platform issues `POST {runtime.url}` with body = the input JSON object.
- It forwards the response body verbatim as the skill output.
- Non-2xx responses become invocation failures.
