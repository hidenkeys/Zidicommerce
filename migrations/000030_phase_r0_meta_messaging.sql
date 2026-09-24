CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_meta_identity_global
    ON channel_identities(provider, provider_identity_id)
    WHERE provider IN ('instagram', 'facebook') AND provider_identity_id <> '';

CREATE TABLE IF NOT EXISTS channel_meta_connection_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    provider TEXT NOT NULL CHECK (provider IN ('instagram', 'facebook')),
    webhook_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (webhook_status IN ('pending', 'verified', 'active')),
    last_signature_verified_at TIMESTAMPTZ,
    last_signature_rejected_at TIMESTAMPTZ,
    signature_rejection_count BIGINT NOT NULL DEFAULT 0,
    last_inbound_at TIMESTAMPTZ,
    last_outbound_at TIMESTAMPTZ,
    last_provider_error TEXT NOT NULL DEFAULT '',
    last_provider_error_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_meta_connection_states_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE,
    UNIQUE (organization_id, channel_connection_id)
);

CREATE TABLE IF NOT EXISTS channel_meta_contact_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    provider TEXT NOT NULL CHECK (provider IN ('instagram', 'facebook')),
    external_customer_id TEXT NOT NULL,
    last_inbound_at TIMESTAMPTZ NOT NULL,
    conversation_window_ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_meta_contact_states_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE,
    UNIQUE (organization_id, channel_connection_id, external_customer_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_meta_contact_window
    ON channel_meta_contact_states(organization_id, channel_connection_id, conversation_window_ends_at);
