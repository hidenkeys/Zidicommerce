ALTER TABLE channel_whatsapp_configs
    ADD COLUMN IF NOT EXISTS connection_method TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS authorization_status TEXT NOT NULL DEFAULT 'not_started',
    ADD COLUMN IF NOT EXISTS authorized_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS authorization_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_authorization_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS assisted_setup_status TEXT NOT NULL DEFAULT 'not_requested',
    ADD COLUMN IF NOT EXISTS assisted_setup_note TEXT NOT NULL DEFAULT '';

ALTER TABLE channel_whatsapp_configs
    DROP CONSTRAINT IF EXISTS channel_whatsapp_configs_connection_method_check,
    DROP CONSTRAINT IF EXISTS channel_whatsapp_configs_authorization_status_check,
    DROP CONSTRAINT IF EXISTS channel_whatsapp_configs_assisted_setup_status_check;

ALTER TABLE channel_whatsapp_configs
    ADD CONSTRAINT channel_whatsapp_configs_connection_method_check
        CHECK (connection_method IN ('manual', 'embedded_signup', 'assisted')),
    ADD CONSTRAINT channel_whatsapp_configs_authorization_status_check
        CHECK (authorization_status IN ('not_started', 'pending', 'authorized', 'requires_reauthorization', 'revoked', 'failed')),
    ADD CONSTRAINT channel_whatsapp_configs_assisted_setup_status_check
        CHECK (assisted_setup_status IN ('not_requested', 'requested', 'in_progress', 'awaiting_merchant_action', 'connected', 'blocked', 'completed'));

CREATE TABLE IF NOT EXISTS channel_meta_signup_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    requested_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    token_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'initiated'
        CHECK (status IN ('initiated', 'processing', 'completed', 'failed', 'expired')),
    failure_code TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_meta_signup_attempts_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_channel_meta_signup_attempts_connection
    ON channel_meta_signup_attempts(organization_id, channel_connection_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_meta_signup_attempts_expiry
    ON channel_meta_signup_attempts(status, expires_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_meta_signup_attempts_token_hash
    ON channel_meta_signup_attempts(token_hash);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_whatsapp_authorized_waba
    ON channel_whatsapp_configs(whatsapp_business_account_id)
    WHERE whatsapp_business_account_id <> '' AND authorization_status = 'authorized';
