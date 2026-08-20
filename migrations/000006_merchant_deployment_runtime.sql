CREATE TABLE IF NOT EXISTS channel_outbound_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    session_id UUID REFERENCES conversation_sessions(id) ON DELETE SET NULL,
    external_conversation_id TEXT NOT NULL DEFAULT '',
    recipient TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    message_type TEXT NOT NULL DEFAULT 'text',
    status TEXT NOT NULL DEFAULT 'queued',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    provider_message_id TEXT NOT NULL DEFAULT '',
    provider_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_outbound_messages_status_check CHECK (status IN ('queued', 'sent', 'failed', 'skipped'))
);

CREATE INDEX IF NOT EXISTS idx_channel_outbound_messages_channel
    ON channel_outbound_messages(organization_id, channel_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_outbound_messages_session
    ON channel_outbound_messages(organization_id, session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_outbound_messages_retry
    ON channel_outbound_messages(status, next_attempt_at)
    WHERE status = 'failed';

CREATE TABLE IF NOT EXISTS payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    reference TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'processing',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payment_webhook_events_status_check CHECK (status IN ('processing', 'processed', 'ignored', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_webhook_events_provider_external
    ON payment_webhook_events(provider, external_event_id);

CREATE INDEX IF NOT EXISTS idx_payment_webhook_events_reference
    ON payment_webhook_events(provider, reference);

CREATE TABLE IF NOT EXISTS commerce_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    channel_id UUID REFERENCES channels(id) ON DELETE SET NULL,
    notification_type TEXT NOT NULL,
    recipient TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    outbound_message_id UUID REFERENCES channel_outbound_messages(id) ON DELETE SET NULL,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_notifications_status_check CHECK (status IN ('queued', 'sent', 'failed', 'skipped'))
);

CREATE INDEX IF NOT EXISTS idx_commerce_notifications_order
    ON commerce_notifications(organization_id, order_id, created_at DESC);

CREATE TABLE IF NOT EXISTS merchant_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'completed',
    source TEXT NOT NULL DEFAULT 'json',
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT merchant_import_jobs_status_check CHECK (status IN ('completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_merchant_import_jobs_org
    ON merchant_import_jobs(organization_id, created_at DESC);
