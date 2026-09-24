CREATE TABLE IF NOT EXISTS channel_oauth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    requested_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    provider TEXT NOT NULL,
    state_hash TEXT NOT NULL,
    pkce_verifier_ref TEXT NOT NULL DEFAULT '',
    redirect_uri TEXT NOT NULL,
    requested_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    granted_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    denied_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    review_approved BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL CHECK (status IN (
        'initiated', 'processing', 'awaiting_asset_selection',
        'completed', 'cancelled', 'failed', 'expired'
    )),
    failure_code TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    callback_claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_oauth_sessions_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_oauth_sessions_state_hash
    ON channel_oauth_sessions(state_hash);

CREATE INDEX IF NOT EXISTS idx_channel_oauth_sessions_connection_status
    ON channel_oauth_sessions(organization_id, channel_connection_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_oauth_sessions_expiry
    ON channel_oauth_sessions(status, expires_at);

CREATE TABLE IF NOT EXISTS channel_oauth_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_connection_id UUID NOT NULL,
    oauth_session_id UUID NOT NULL REFERENCES channel_oauth_sessions(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_asset_id TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    external_handle TEXT NOT NULL DEFAULT '',
    eligible BOOLEAN NOT NULL DEFAULT FALSE,
    ineligible_reason TEXT NOT NULL DEFAULT '',
    selected BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    selected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_oauth_assets_tenant_connection_fk
        FOREIGN KEY (organization_id, channel_connection_id)
        REFERENCES channels(organization_id, id)
        ON DELETE CASCADE,
    UNIQUE (organization_id, channel_connection_id, provider_asset_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_oauth_assets_session
    ON channel_oauth_assets(organization_id, oauth_session_id, display_name);
