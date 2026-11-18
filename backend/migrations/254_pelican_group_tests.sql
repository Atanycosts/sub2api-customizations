-- Group-routed Pelican observations have independent settings and accounting.
CREATE TABLE pelican_config (
 id INTEGER PRIMARY KEY CHECK (id=1), enabled BOOLEAN NOT NULL DEFAULT FALSE,
 daily_budget_usd NUMERIC(14,6) NOT NULL DEFAULT 0 CHECK(daily_budget_usd>=0)
);
INSERT INTO pelican_config(id) VALUES(1);
CREATE TABLE pelican_plans (
 id BIGSERIAL PRIMARY KEY, group_id BIGINT NOT NULL REFERENCES groups(id),
 configuration JSONB NOT NULL, enabled BOOLEAN NOT NULL, published BOOLEAN NOT NULL,
 interval_minutes INTEGER NOT NULL CHECK(interval_minutes BETWEEN 30 AND 10080),
 next_run_at TIMESTAMPTZ NOT NULL, last_run_at TIMESTAMPTZ, running_until TIMESTAMPTZ,
 deleted_at TIMESTAMPTZ
);
CREATE TABLE pelican_runs (
 id UUID PRIMARY KEY, plan_id BIGINT NOT NULL REFERENCES pelican_plans(id),
 requested_by BIGINT NOT NULL, group_id BIGINT NOT NULL, published BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, finished_at TIMESTAMPTZ,
 state TEXT NOT NULL CHECK(state IN ('running','completed','failed','unknown')),
 started_at TIMESTAMPTZ, result JSONB NOT NULL,
 reserved_usd NUMERIC(14,6) NOT NULL CHECK(reserved_usd>0), cost_usd NUMERIC(14,6),
 payload_pruned BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX pelican_runs_listing ON pelican_runs(created_at DESC);
CREATE INDEX pelican_runs_plan ON pelican_runs(plan_id,created_at DESC);
CREATE INDEX pelican_runs_budget ON pelican_runs(created_at) INCLUDE(reserved_usd,cost_usd);
