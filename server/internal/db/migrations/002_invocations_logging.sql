-- M1.5 — Invocation auditing & anti-theft.
--
-- Every /v1/.../invoke and MCP tools/call hit writes one row here. The
-- columns capture enough to: (a) bill / quota in the future, (b) surface
-- stats (call count, last IP, last invoked at), and (c) investigate abuse.
ALTER TABLE invocations
    ADD COLUMN IF NOT EXISTS api_key_id    UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS caller_ip     INET,
    ADD COLUMN IF NOT EXISTS user_agent    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS input_bytes   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_bytes  INTEGER NOT NULL DEFAULT 0;

-- Skill-level stats queries filter by (org_id, skill_id) and order by time.
CREATE INDEX IF NOT EXISTS invocations_org_skill_started_idx
    ON invocations (org_id, skill_id, started_at DESC);
