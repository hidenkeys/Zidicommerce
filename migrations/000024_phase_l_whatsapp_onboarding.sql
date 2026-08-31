ALTER TABLE channel_whatsapp_configs
    ADD COLUMN IF NOT EXISTS setup_state TEXT NOT NULL DEFAULT 'not_connected',
    ADD COLUMN IF NOT EXISTS last_webhook_verification_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_webhook_verification_failed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_webhook_verification_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_signature_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_signature_rejected_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_test_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_test_message_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_test_message_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS test_recipient_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS test_recipient_display TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_inbound_test_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS credential_rotated_at TIMESTAMPTZ;

ALTER TABLE channel_whatsapp_configs
    DROP CONSTRAINT IF EXISTS channel_whatsapp_configs_setup_state_check;

UPDATE channel_whatsapp_configs AS config
SET setup_state = CASE
    WHEN config.webhook_status = 'verified'
         AND config.phone_number_id <> ''
         AND (
             SELECT COUNT(DISTINCT credential_type)
             FROM channel_credential_references AS credential
             WHERE credential.organization_id = config.organization_id
               AND credential.channel_connection_id = config.channel_connection_id
               AND credential.credential_type IN ('access_token', 'app_secret', 'verify_token')
               AND credential.status = 'present'
         ) = 3 THEN 'test_message_ready'
    WHEN config.webhook_status = 'verified' AND config.phone_number_id <> '' THEN 'phone_identity_verified'
    WHEN config.webhook_status = 'verified' THEN 'webhook_verified'
    WHEN (
        SELECT COUNT(DISTINCT credential_type)
        FROM channel_credential_references AS credential
        WHERE credential.organization_id = config.organization_id
          AND credential.channel_connection_id = config.channel_connection_id
          AND credential.credential_type IN ('access_token', 'app_secret', 'verify_token')
          AND credential.status = 'present'
    ) = 3 THEN 'credentials_added'
    ELSE 'setup_started'
END;

ALTER TABLE channel_whatsapp_configs
    ADD CONSTRAINT channel_whatsapp_configs_setup_state_check CHECK (setup_state IN (
        'not_connected',
        'setup_started',
        'credentials_added',
        'webhook_verified',
        'phone_identity_verified',
        'test_message_ready',
        'test_message_sent',
        'inbound_test_received',
        'connected',
        'healthy',
        'degraded',
        'requires_attention',
        'disconnected',
        'credential_expired',
        'webhook_failed'
    ));

CREATE INDEX IF NOT EXISTS idx_channel_whatsapp_setup_state
    ON channel_whatsapp_configs(organization_id, setup_state, updated_at DESC);
