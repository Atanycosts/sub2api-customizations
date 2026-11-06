-- Durable memory events. This queue contains bounded conversation text and
-- must be retained only as long as needed for delivery/reconciliation.
CREATE TABLE IF NOT EXISTS memory_archive_tenants (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS memory_archive_queue (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES memory_archive_tenants(user_id) ON DELETE CASCADE,
    event_id VARCHAR(128) NOT NULL,
    generation BIGINT NOT NULL CHECK (generation > 0),
    payload JSONB,
    payload_sha256 CHAR(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN (
        'pending', 'processing', 'submitted', 'retry_wait',
        'needs_reconciliation', 'completed', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    reconciliation_checks INTEGER NOT NULL DEFAULT 0 CHECK (reconciliation_checks >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_token UUID,
    worker_id TEXT,
    last_error_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, event_id),
    CHECK (payload IS NULL OR octet_length(payload::text) <= 4194304),
    CHECK ((lease_token IS NULL) = (lease_until IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_memory_archive_queue_ready
    ON memory_archive_queue (available_at, id)
    WHERE state IN ('pending', 'retry_wait', 'needs_reconciliation');
CREATE INDEX IF NOT EXISTS idx_memory_archive_queue_leases
    ON memory_archive_queue (lease_until)
    WHERE lease_until IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_archive_queue_user_state
    ON memory_archive_queue (user_id, state);
COMMENT ON TABLE memory_archive_queue IS
    'Durable memory delivery queue; submitted leases recover only into reconciliation, never automatic replay';
