CREATE TABLE IF NOT EXISTS background_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT background_jobs_status_check CHECK (status IN ('queued', 'processing', 'completed', 'retry_pending', 'failed', 'failed_permanently', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_background_jobs_idempotency
    ON background_jobs(job_type, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_background_jobs_due
    ON background_jobs(status, available_at, created_at)
    WHERE status IN ('queued', 'retry_pending');

CREATE INDEX IF NOT EXISTS idx_background_jobs_org
    ON background_jobs(organization_id, created_at DESC);

ALTER TABLE channel_outbound_messages
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ;

ALTER TABLE channel_outbound_messages DROP CONSTRAINT IF EXISTS channel_outbound_messages_status_check;
ALTER TABLE channel_outbound_messages ADD CONSTRAINT channel_outbound_messages_status_check
    CHECK (status IN ('queued', 'sending', 'sent', 'delivered', 'read', 'failed', 'retry_pending', 'failed_permanently', 'skipped'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_outbound_messages_idempotency
    ON channel_outbound_messages(organization_id, channel_id, idempotency_key)
    WHERE idempotency_key <> '';

DROP INDEX IF EXISTS idx_channel_outbound_messages_retry;
CREATE INDEX IF NOT EXISTS idx_channel_outbound_messages_retry
    ON channel_outbound_messages(status, next_attempt_at)
    WHERE status IN ('failed', 'retry_pending');

ALTER TABLE commerce_notifications
    ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS sent_at TIMESTAMPTZ;

ALTER TABLE commerce_notifications DROP CONSTRAINT IF EXISTS commerce_notifications_status_check;
ALTER TABLE commerce_notifications ADD CONSTRAINT commerce_notifications_status_check
    CHECK (status IN ('queued', 'processing', 'sent', 'failed', 'retry_pending', 'failed_permanently', 'skipped'));

CREATE INDEX IF NOT EXISTS idx_commerce_notifications_due
    ON commerce_notifications(status, next_attempt_at, created_at)
    WHERE status IN ('queued', 'retry_pending', 'failed');

ALTER TABLE merchant_import_jobs DROP CONSTRAINT IF EXISTS merchant_import_jobs_status_check;
ALTER TABLE merchant_import_jobs ADD CONSTRAINT merchant_import_jobs_status_check
    CHECK (status IN ('queued', 'processing', 'completed', 'failed', 'partially_failed'));

CREATE TABLE IF NOT EXISTS payment_reconciliations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    reference TEXT NOT NULL,
    internal_status TEXT NOT NULL,
    provider_status TEXT NOT NULL,
    status TEXT NOT NULL,
    action_taken TEXT NOT NULL DEFAULT '',
    discrepancy TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payment_reconciliations_status_check CHECK (status IN ('matched', 'auto_reconciled', 'review_required', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_payment_reconciliations_payment
    ON payment_reconciliations(organization_id, payment_id, created_at DESC);

CREATE TABLE IF NOT EXISTS support_handoffs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'open',
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT support_handoffs_status_check CHECK (status IN ('open', 'assigned', 'resolved'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_handoffs_open_session
    ON support_handoffs(organization_id, session_id)
    WHERE status IN ('open', 'assigned');

CREATE INDEX IF NOT EXISTS idx_support_handoffs_org_status
    ON support_handoffs(organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_store_status_created
    ON orders(organization_id, store_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_customer_created
    ON orders(organization_id, customer_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_payments_reference_provider
    ON payments(provider, reference);
