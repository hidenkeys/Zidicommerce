CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_organization_customer
    ON customers(organization_id, id);

CREATE TABLE IF NOT EXISTS whatsapp_contact_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    customer_id UUID,
    external_customer_id_hash TEXT NOT NULL,
    masked_phone TEXT NOT NULL DEFAULT '',
    consent_status TEXT NOT NULL DEFAULT 'unknown'
        CHECK (consent_status IN ('opted_in', 'opted_out', 'unknown')),
    consent_source TEXT NOT NULL DEFAULT 'inbound_message'
        CHECK (consent_source IN ('inbound_message', 'manual', 'import', 'checkout', 'support', 'system')),
    opted_in_at TIMESTAMPTZ,
    opted_out_at TIMESTAMPTZ,
    last_inbound_at TIMESTAMPTZ,
    last_outbound_at TIMESTAMPTZ,
    service_window_expires_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, external_customer_id_hash),
    CONSTRAINT whatsapp_contact_states_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT whatsapp_contact_states_tenant_customer_fk
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers(organization_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_contact_window
    ON whatsapp_contact_states(organization_id, channel_connection_id, consent_status, service_window_expires_at);

CREATE TABLE IF NOT EXISTS whatsapp_message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    provider_template_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT 'en',
    category TEXT NOT NULL CHECK (category IN ('utility', 'authentication', 'marketing', 'service')),
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'pending', 'approved', 'rejected', 'paused', 'disabled', 'archived')),
    body TEXT NOT NULL,
    header_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    footer_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    buttons_metadata JSONB NOT NULL DEFAULT '[]'::jsonb,
    variable_schema JSONB NOT NULL DEFAULT '[]'::jsonb,
    sample_values JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_synced_at TIMESTAMPTZ,
    rejection_reason TEXT NOT NULL DEFAULT '',
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, name, language),
    CONSTRAINT whatsapp_message_templates_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_templates_status
    ON whatsapp_message_templates(organization_id, channel_connection_id, status, category, updated_at DESC);

CREATE TABLE IF NOT EXISTS whatsapp_policy_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    contact_state_id UUID REFERENCES whatsapp_contact_states(id) ON DELETE SET NULL,
    template_id UUID REFERENCES whatsapp_message_templates(id) ON DELETE SET NULL,
    message_type TEXT NOT NULL,
    decision TEXT NOT NULL,
    allowed BOOLEAN NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, idempotency_key),
    CONSTRAINT whatsapp_policy_decisions_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_policy_recent
    ON whatsapp_policy_decisions(organization_id, channel_connection_id, allowed, created_at DESC);

CREATE TABLE IF NOT EXISTS whatsapp_operational_metrics_daily (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    metric_date DATE NOT NULL,
    freeform_send_count BIGINT NOT NULL DEFAULT 0,
    template_send_count BIGINT NOT NULL DEFAULT 0,
    policy_blocked_count BIGINT NOT NULL DEFAULT 0,
    window_closed_blocked_count BIGINT NOT NULL DEFAULT 0,
    rate_limit_count BIGINT NOT NULL DEFAULT 0,
    invalid_recipient_count BIGINT NOT NULL DEFAULT 0,
    invalid_credential_count BIGINT NOT NULL DEFAULT 0,
    provider_delivery_latency_ms BIGINT NOT NULL DEFAULT 0,
    provider_delivery_samples BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, channel_connection_id, metric_date),
    CONSTRAINT whatsapp_operational_metrics_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id) ON DELETE CASCADE
);
