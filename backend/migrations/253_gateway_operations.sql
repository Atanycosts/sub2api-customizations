-- Administrator-only observations. No customer billing or scheduling ownership.
CREATE TABLE IF NOT EXISTS gateway_operations_config (
 id SMALLINT PRIMARY KEY CHECK (id=1), worker_enabled BOOLEAN NOT NULL DEFAULT FALSE,
 refresh_minutes INTEGER NOT NULL DEFAULT 30 CHECK(refresh_minutes BETWEEN 5 AND 1440),
 daily_budget_usd NUMERIC(20,10) NOT NULL DEFAULT 0 CHECK(daily_budget_usd>=0)
);
INSERT INTO gateway_operations_config(id) VALUES(1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS gateway_operations_wallets (
 id BIGSERIAL PRIMARY KEY, provider TEXT NOT NULL, base_url TEXT NOT NULL,
 remote_user_id TEXT NOT NULL, credential_cipher TEXT NOT NULL,
 configuration JSONB NOT NULL, observation JSONB NOT NULL DEFAULT '{}',
 last_notified_at TIMESTAMPTZ,
 UNIQUE(provider,base_url,remote_user_id)
);
CREATE TABLE IF NOT EXISTS gateway_operations_experiments (
 id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 requested_by BIGINT NOT NULL REFERENCES users(id), frozen JSONB NOT NULL,
 reserved_usd NUMERIC(20,10) NOT NULL CHECK(reserved_usd>0),
 state TEXT NOT NULL CHECK(state IN ('running','completed','failed','unknown')),
 result JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS gateway_operations_experiments_created ON gateway_operations_experiments(created_at DESC);
