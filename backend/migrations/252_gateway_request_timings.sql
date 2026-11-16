-- Observational only: no request/response bodies, URLs, credentials or billing changes.
CREATE TABLE IF NOT EXISTS gateway_request_timings (
 trace_id UUID PRIMARY KEY,
 request_id VARCHAR(64) NOT NULL,
 started_at TIMESTAMPTZ NOT NULL,
 outcome VARCHAR(32) NOT NULL,
 trace JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gateway_timings_started ON gateway_request_timings(started_at DESC, trace_id DESC);
CREATE INDEX IF NOT EXISTS idx_gateway_timings_request ON gateway_request_timings(request_id);
-- Billing can finish before or after the HTTP trace writer; separate links make
-- both arrival orders safe without a foreign key to the best-effort trace.
CREATE TABLE IF NOT EXISTS gateway_request_timing_usage (
 trace_id UUID PRIMARY KEY,
 usage_id BIGINT,
 usage_request_id VARCHAR(64) NOT NULL,
 api_key_id BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_gateway_timing_usage_request ON gateway_request_timing_usage(usage_request_id, api_key_id);
CREATE INDEX IF NOT EXISTS idx_gateway_timing_usage_created ON gateway_request_timing_usage(created_at);
