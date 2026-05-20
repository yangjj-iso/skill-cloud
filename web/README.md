# Skill Cloud Web UI

A small Vite + React single-page app for inspecting Skill Cloud
deployments. Pages:

| Page | Purpose |
|---|---|
| **Overview** | Org-wide KPIs (skills total, invocations 24h / total) + the 10 most recent calls. |
| **Skills** | List every skill the API key can see. |
| **Skill detail** | Manifest + runtime type + stats + an inline "Invoke" form, plus the 25 most recent calls. |
| **Invocations** | Cross-skill recent invocations (newest first). |

Authentication: paste an API key on the login page. The key is stored
in `localStorage` and sent as `Authorization: Bearer <key>` on every
request. There is no cookie, no server-side session. Sign out clears
the key.

## Development

```bash
cd web
npm install
npm run dev   # vite dev server on :5173, proxies /v1 to :8080
```

By default the dev server proxies `/v1`, `/mcp`, `/metrics`, and
`/healthz` to `http://localhost:8080`. Point it at a remote deployment
via the `VITE_SKILLCLOUD_API` env var:

```bash
VITE_SKILLCLOUD_API=https://skills.example.com npm run dev
```

`npm run build` produces a static bundle in `dist/`; serve it behind the
same origin as the API (so cookies / headers / CORS just work).

## API surface used

The UI does **not** depend on any private endpoint — it only consumes
endpoints already published as part of `/v1`:

- `GET /v1/skills`, `GET /v1/skills/:ns/:name`
- `GET /v1/skills/:ns/:name/stats`
- `GET /v1/skills/:ns/:name/logs`
- `POST /v1/skills/:ns/:name/invoke`
- `GET /v1/overview` (BFF; added in M4-C)
- `GET /v1/invocations` (BFF; added in M4-C)

Runtime implementation details are intentionally redacted from
`GET /v1/skills/:ns/:name` for anti-theft. Owners can fetch
`/v1/skills/:ns/:name/runtime` separately; the UI does not call that
endpoint by default.
