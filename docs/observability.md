# Observability

The server exposes a Prometheus text-format endpoint at `GET /metrics`.
It is intentionally unauthenticated so the scraper does not need a
bearer token — restrict access at the network layer (e.g. only the
Prometheus pod / scrape host can reach it) in production.

The bundled `docker-compose.yml` runs a full local observability stack:

| Service | URL | Notes |
|---|---|---|
| Skill Cloud | <http://localhost:8080> | application server |
| Prometheus | <http://localhost:9090> | scrapes `server:8080/metrics` every 15 s |
| Grafana | <http://localhost:3000> | anonymous viewer; `admin/admin` to edit |

Grafana ships with a pre-provisioned Prometheus datasource and the
"Skill Cloud — overview" dashboard located at
`deploy/grafana/dashboards/skillcloud-overview.json`.

## Metric reference

All metric names are prefixed with `skillcloud_`. Cardinality is bounded
by the registry (per-skill labels) and the API surface (per-route
labels) — no per-request or per-user labels.

### `skillcloud_invocations_total` — counter

Total number of dispatcher attempts. Labels:

- `org` — org slug when resolvable, else org UUID.
- `namespace` / `name` — the skill identifier.
- `status` — `ok` / `error` / `timeout`.

Incremented from both the REST `/v1/skills/.../invoke` handler and the
MCP `tools/call` handler.

### `skillcloud_invocation_latency_seconds` — histogram

Wall-clock dispatcher latency in seconds, including container startup
or upstream HTTP. Labels: `org`, `namespace`, `name`. Buckets span
50 ms → 5 min. Use `histogram_quantile` to compute p50/p95/p99.

### `skillcloud_rate_limit_dropped_total` — counter

Requests rejected by the per-key sliding-window rate limiter. Label:
`api_key_prefix` — the first 12 characters of the API key UUID (the
secret half of the token is never observed).

### `skillcloud_skills_registered` — gauge

Number of skill versions currently registered, partitioned by `org` and
`runtime_type` (`docker` / `http_proxy`). Refreshed on every
`POST /v1/skills` and on a slow background tick.

### `skillcloud_http_requests_total` — counter

Total HTTP requests served, partitioned by `method`, `route` (the gin
route template — e.g. `/v1/skills/:namespace/:name/invoke`, not the
actual URL), and `status_code`. Unmatched routes (404) collapse to a
single `route="unmatched"` series so probe storms can't blow up the
registry.

## Example PromQL

```promql
# Requests per second over the last minute, by status.
sum by (status) (rate(skillcloud_invocations_total[1m]))

# p95 latency over the last 5 minutes (across all skills).
histogram_quantile(
  0.95,
  sum by (le) (rate(skillcloud_invocation_latency_seconds_bucket[5m]))
)

# Top 5 noisiest API keys (by 429 count over the last hour).
topk(5, sum by (api_key_prefix) (increase(skillcloud_rate_limit_dropped_total[1h])))

# Error rate as a percentage of total invocations.
sum(rate(skillcloud_invocations_total{status!="ok"}[5m]))
/
sum(rate(skillcloud_invocations_total[5m]))
```
