-- Skill Cloud — initial schema.
--
-- Multi-tenancy: every tenant-owned row carries `org_id`. Tenant queries
-- must filter on `org_id` (enforced in the registry layer).

CREATE TABLE IF NOT EXISTS orgs (
    id          UUID PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS org_members (
    org_id  UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    TEXT NOT NULL DEFAULT 'member',
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- short, non-secret prefix (e.g. "sc_live_") shown in UI for identification.
    prefix          TEXT NOT NULL,
    -- bcrypt hash of the full secret. The raw secret is shown to the user
    -- exactly once at creation time and is never persisted.
    hash            TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_prefix_idx ON api_keys (prefix);

-- Skills are uniquely identified by (org_id, namespace, name) so each org can
-- own its own namespaces without colliding with other tenants.
CREATE TABLE IF NOT EXISTS skills (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    namespace       TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    latest_version  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, namespace, name)
);

CREATE TABLE IF NOT EXISTS skill_versions (
    id              UUID PRIMARY KEY,
    skill_id        UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version         TEXT NOT NULL,
    manifest        JSONB NOT NULL,
    storage_key     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, version)
);

CREATE TABLE IF NOT EXISTS invocations (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    skill_id        UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version         TEXT NOT NULL,
    status          TEXT NOT NULL,
    input           JSONB NOT NULL DEFAULT '{}'::jsonb,
    output          JSONB,
    error_message   TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ,
    latency_ms      INTEGER
);

CREATE INDEX IF NOT EXISTS invocations_org_started_idx ON invocations (org_id, started_at DESC);
