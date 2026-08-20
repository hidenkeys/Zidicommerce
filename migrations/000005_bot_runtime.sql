CREATE TABLE IF NOT EXISTS conversation_sessions (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE RESTRICT,
    bot_version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE RESTRICT,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    external_conversation_id TEXT NOT NULL,
    current_step_key TEXT NOT NULL DEFAULT '',
    expected_input TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    system_context JSONB NOT NULL DEFAULT '{}'::jsonb,
    lock_version INTEGER NOT NULL DEFAULT 1,
    last_message_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_sessions_status_check CHECK (status IN ('active', 'completed', 'handoff', 'expired', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_sessions_external
    ON conversation_sessions(organization_id, channel_id, external_conversation_id);

CREATE INDEX IF NOT EXISTS idx_conversation_sessions_status
    ON conversation_sessions(organization_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS conversation_messages (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    external_message_id TEXT,
    direction TEXT NOT NULL,
    message_type TEXT NOT NULL,
    sender TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_messages_direction_check CHECK (direction IN ('inbound', 'outbound'))
);

CREATE INDEX IF NOT EXISTS idx_conversation_messages_session
    ON conversation_messages(organization_id, session_id, created_at ASC);

CREATE TABLE IF NOT EXISTS processed_messages (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    session_id UUID REFERENCES conversation_sessions(id) ON DELETE SET NULL,
    external_message_id TEXT NOT NULL,
    external_conversation_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processed',
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT processed_messages_status_check CHECK (status IN ('processing', 'processed', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_processed_messages_external
    ON processed_messages(organization_id, channel_id, external_message_id);

CREATE TABLE IF NOT EXISTS runtime_events (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    session_id UUID REFERENCES conversation_sessions(id) ON DELETE SET NULL,
    bot_id UUID REFERENCES bots(id) ON DELETE SET NULL,
    bot_version_id UUID REFERENCES bot_versions(id) ON DELETE SET NULL,
    channel_id UUID REFERENCES channels(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    step_key TEXT NOT NULL DEFAULT '',
    action_key TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_runtime_events_session
    ON runtime_events(organization_id, session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_runtime_events_type
    ON runtime_events(organization_id, event_type, created_at DESC);
