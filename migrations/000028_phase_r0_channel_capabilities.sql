CREATE TABLE IF NOT EXISTS channel_capabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    provider TEXT NOT NULL,
    capability TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'available', 'setup_required', 'awaiting_permission_review',
        'missing_permission', 'restricted', 'unsupported_by_provider', 'unverified'
    )),
    reason TEXT NOT NULL DEFAULT '',
    required_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    granted_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_capabilities_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE,
    UNIQUE (organization_id, channel_connection_id, capability)
);

CREATE INDEX IF NOT EXISTS idx_channel_capabilities_connection_status
    ON channel_capabilities(organization_id, channel_connection_id, status);

INSERT INTO channel_capabilities (
    organization_id, channel_connection_id, provider, capability, status,
    reason, required_scopes
)
SELECT c.organization_id, c.id, c.provider, capability.name,
    CASE
        WHEN c.status IN ('active', 'connected', 'healthy', 'degraded', 'requires_attention') THEN 'available'
        ELSE 'setup_required'
    END,
    CASE
        WHEN c.status IN ('active', 'connected', 'healthy', 'degraded', 'requires_attention') THEN ''
        ELSE 'Complete provider onboarding and asset verification'
    END,
    capability.required_scopes
FROM channels c
CROSS JOIN LATERAL (
    VALUES
        ('oauth_onboarding', '[]'::jsonb),
        ('inbound_text', '["whatsapp_business_messaging"]'::jsonb),
        ('outbound_text', '["whatsapp_business_messaging"]'::jsonb),
        ('inbound_media', '["whatsapp_business_messaging"]'::jsonb),
        ('outbound_media', '["whatsapp_business_messaging"]'::jsonb),
        ('interactive_messages', '["whatsapp_business_messaging"]'::jsonb),
        ('templates', '["whatsapp_business_messaging"]'::jsonb),
        ('read_receipts', '["whatsapp_business_messaging"]'::jsonb),
        ('delivery_receipts', '["whatsapp_business_messaging"]'::jsonb),
        ('profile_read', '["whatsapp_business_management"]'::jsonb),
        ('commerce_actions', '[]'::jsonb),
        ('human_handoff', '[]'::jsonb)
) AS capability(name, required_scopes)
WHERE c.provider = 'whatsapp'
ON CONFLICT (organization_id, channel_connection_id, capability) DO NOTHING;
