CREATE TABLE IF NOT EXISTS channel_provider_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    credential_type TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    nonce TEXT NOT NULL,
    key_version TEXT NOT NULL DEFAULT 'v1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, credential_type),
    CONSTRAINT channel_provider_secrets_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS channel_whatsapp_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    phone_number_id TEXT NOT NULL DEFAULT '',
    whatsapp_business_account_id TEXT NOT NULL DEFAULT '',
    meta_business_account_id TEXT NOT NULL DEFAULT '',
    display_phone_number TEXT NOT NULL DEFAULT '',
    graph_api_version TEXT NOT NULL DEFAULT 'v20.0',
    webhook_status TEXT NOT NULL DEFAULT 'not_configured'
        CHECK (webhook_status IN ('not_configured', 'pending', 'verified', 'failed')),
    last_webhook_verified_at TIMESTAMPTZ,
    last_inbound_at TIMESTAMPTZ,
    last_outbound_at TIMESTAMPTZ,
    last_provider_failure_at TIMESTAMPTZ,
    rate_limited_until TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id),
    CONSTRAINT channel_whatsapp_configs_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_whatsapp_phone_number
    ON channel_whatsapp_configs(phone_number_id)
    WHERE phone_number_id <> '';

CREATE INDEX IF NOT EXISTS idx_channel_whatsapp_org_status
    ON channel_whatsapp_configs(organization_id, webhook_status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_provider_secrets_connection
    ON channel_provider_secrets(organization_id, channel_connection_id, credential_type);
