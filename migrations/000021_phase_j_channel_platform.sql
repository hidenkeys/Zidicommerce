ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS ownership_model TEXT NOT NULL DEFAULT 'merchant_managed',
    ADD COLUMN IF NOT EXISTS environment TEXT NOT NULL DEFAULT 'sandbox',
    ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS last_connected_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMPTZ;

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_status_check;
ALTER TABLE channels ADD CONSTRAINT channels_status_check CHECK (status IN (
    'draft', 'active', 'inactive', 'disabled',
    'not_connected', 'setup_required', 'connecting', 'connected', 'healthy',
    'degraded', 'disconnected', 'requires_attention', 'archived'
));

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_ownership_model_check;
ALTER TABLE channels ADD CONSTRAINT channels_ownership_model_check
    CHECK (ownership_model IN ('merchant_managed', 'zidi_managed'));

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_environment_check;
ALTER TABLE channels ADD CONSTRAINT channels_environment_check
    CHECK (environment IN ('sandbox', 'production'));

UPDATE channels
SET capabilities = '["inbound_messages","outbound_messages","media","templates","delivery_receipts"]'::jsonb
WHERE provider = 'whatsapp' AND capabilities = '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_channels_org_status_provider
    ON channels(organization_id, status, provider, created_at DESC);

CREATE TABLE IF NOT EXISTS channel_provider_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_account_id TEXT NOT NULL DEFAULT '',
    business_name TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    ownership_model TEXT NOT NULL DEFAULT 'merchant_managed'
        CHECK (ownership_model IN ('merchant_managed', 'zidi_managed')),
    status TEXT NOT NULL DEFAULT 'setup_required',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_provider_accounts_external
    ON channel_provider_accounts(organization_id, provider, provider_account_id)
    WHERE provider_account_id <> '';

CREATE INDEX IF NOT EXISTS idx_channel_provider_accounts_connection
    ON channel_provider_accounts(organization_id, channel_connection_id, created_at DESC);

CREATE TABLE IF NOT EXISTS channel_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    identity_type TEXT NOT NULL
        CHECK (identity_type IN ('phone_number', 'instagram_business_account', 'web_widget', 'page')),
    display_name TEXT NOT NULL DEFAULT '',
    provider_identity_id TEXT NOT NULL DEFAULT '',
    external_handle TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'setup_required',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_identities_external
    ON channel_identities(organization_id, provider, provider_identity_id)
    WHERE provider_identity_id <> '';

CREATE INDEX IF NOT EXISTS idx_channel_identities_connection
    ON channel_identities(organization_id, channel_connection_id, created_at DESC);

CREATE TABLE IF NOT EXISTS channel_credential_references (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    ownership_model TEXT NOT NULL DEFAULT 'merchant_managed'
        CHECK (ownership_model IN ('merchant_managed', 'zidi_managed')),
    secret_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'missing'
        CHECK (status IN ('missing', 'present', 'expiring', 'expired', 'revoked', 'requires_reauthorization')),
    expires_at TIMESTAMPTZ,
    last_validated_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, credential_type)
);

CREATE INDEX IF NOT EXISTS idx_channel_credentials_connection_status
    ON channel_credential_references(organization_id, channel_connection_id, status);

CREATE TABLE IF NOT EXISTS channel_health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'failed')),
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    latency_ms BIGINT NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_channel_health_connection_checked
    ON channel_health_checks(organization_id, channel_connection_id, checked_at DESC);

CREATE TABLE IF NOT EXISTS channel_provider_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    event_type TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    normalized_status TEXT NOT NULL DEFAULT 'received',
    idempotency_key TEXT NOT NULL,
    payload_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    processing_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_provider_events_external
    ON channel_provider_events(organization_id, provider, provider_event_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_provider_events_idempotency
    ON channel_provider_events(organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_channel_provider_events_connection_received
    ON channel_provider_events(organization_id, channel_connection_id, received_at DESC);

CREATE TABLE IF NOT EXISTS channel_metrics_daily (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    metric_date DATE NOT NULL,
    inbound_count BIGINT NOT NULL DEFAULT 0,
    outbound_count BIGINT NOT NULL DEFAULT 0,
    failed_outbound_count BIGINT NOT NULL DEFAULT 0,
    delivered_count BIGINT NOT NULL DEFAULT 0,
    read_count BIGINT NOT NULL DEFAULT 0,
    conversations_started BIGINT NOT NULL DEFAULT 0,
    conversations_human_handled BIGINT NOT NULL DEFAULT 0,
    conversations_ai_handled BIGINT NOT NULL DEFAULT 0,
    average_response_ms BIGINT NOT NULL DEFAULT 0,
    provider_error_count BIGINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, metric_date)
);

CREATE INDEX IF NOT EXISTS idx_channel_metrics_org_date
    ON channel_metrics_daily(organization_id, metric_date DESC, channel_connection_id);
