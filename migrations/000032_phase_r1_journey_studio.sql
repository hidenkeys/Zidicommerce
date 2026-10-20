CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_tenant_id ON bots(organization_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_versions_tenant_bot_id ON bot_versions(organization_id,bot_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_versions_tenant_id ON bot_versions(organization_id,id);

CREATE TABLE journey_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    bot_id UUID NOT NULL UNIQUE,
    version_id UUID NOT NULL,
    connection_id UUID,
    mode TEXT NOT NULL CHECK (mode IN ('guided_chat','whatsapp_flow','ai')),
    trigger TEXT NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id,bot_id,version_id) REFERENCES bot_versions(organization_id,bot_id,id),
    FOREIGN KEY (organization_id,connection_id) REFERENCES channels(organization_id,id),
    UNIQUE (organization_id,connection_id,trigger)
);

CREATE TABLE journey_flow_deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    bot_id UUID NOT NULL,
    version_id UUID NOT NULL,
    connection_id UUID NOT NULL,
    provider_flow_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','validated','published','deprecated','failed')),
    revision INTEGER NOT NULL DEFAULT 1,
    errors JSONB NOT NULL DEFAULT '[]',
    warnings JSONB NOT NULL DEFAULT '[]',
    preview_url TEXT NOT NULL DEFAULT '',
    compiled_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id,bot_id,version_id) REFERENCES bot_versions(organization_id,bot_id,id),
    FOREIGN KEY (organization_id,connection_id) REFERENCES channels(organization_id,id),
    UNIQUE (organization_id,version_id,connection_id)
);

CREATE TABLE journey_preview_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    version_id UUID NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    snapshot JSONB NOT NULL,
    state JSONB NOT NULL,
    scenario TEXT NOT NULL DEFAULT 'normal',
    last_result JSONB NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id,version_id) REFERENCES bot_versions(organization_id,id),
    UNIQUE (organization_id,id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_sessions_tenant_id ON conversation_sessions(organization_id,id);
CREATE TABLE journey_flow_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    connection_id UUID NOT NULL,
    conversation_id UUID NOT NULL,
    version_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id,connection_id) REFERENCES channels(organization_id,id),
    FOREIGN KEY (organization_id,version_id) REFERENCES bot_versions(organization_id,id),
    FOREIGN KEY (organization_id,conversation_id) REFERENCES conversation_sessions(organization_id,id),
    UNIQUE (organization_id,id)
);

CREATE TABLE journey_exchange_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    session_id UUID NOT NULL,
    event_key TEXT NOT NULL,
    result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id,session_id,event_key)
);

CREATE INDEX journey_preview_expiry ON journey_preview_sessions(expires_at);
CREATE INDEX journey_flow_expiry ON journey_flow_sessions(expires_at);

CREATE TABLE journey_execution_leases (
    session_id UUID PRIMARY KEY,
    organization_id UUID NOT NULL,
    owner UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (organization_id,session_id) REFERENCES conversation_sessions(organization_id,id)
);
