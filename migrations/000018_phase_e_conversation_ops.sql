ALTER TABLE conversation_sessions
    ADD COLUMN IF NOT EXISTS store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS conversation_status TEXT NOT NULL DEFAULT 'ai_handling',
    ADD COLUMN IF NOT EXISTS assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS priority TEXT NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS handoff_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS unread_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_message_body TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_message_direction TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_read_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS human_owned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS human_released_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

UPDATE conversation_sessions
SET conversation_status = CASE
        WHEN status = 'handoff' THEN 'human_requested'
        WHEN status = 'completed' THEN 'resolved'
        WHEN status = 'active' THEN 'ai_handling'
        ELSE 'open'
    END
WHERE conversation_status = '';

ALTER TABLE conversation_sessions
    DROP CONSTRAINT IF EXISTS conversation_sessions_conversation_status_check;

ALTER TABLE conversation_sessions
    ADD CONSTRAINT conversation_sessions_conversation_status_check
    CHECK (conversation_status IN ('open', 'ai_handling', 'human_requested', 'human_assigned', 'pending', 'waiting', 'resolved', 'reopened'));

ALTER TABLE conversation_sessions
    DROP CONSTRAINT IF EXISTS conversation_sessions_priority_check;

ALTER TABLE conversation_sessions
    ADD CONSTRAINT conversation_sessions_priority_check
    CHECK (priority IN ('low', 'normal', 'high', 'urgent'));

ALTER TABLE conversation_messages
    DROP CONSTRAINT IF EXISTS conversation_messages_direction_check;

ALTER TABLE conversation_messages
    ADD CONSTRAINT conversation_messages_direction_check
    CHECK (direction IN ('inbound', 'outbound', 'internal'));

ALTER TABLE support_handoffs
    ADD COLUMN IF NOT EXISTS priority TEXT NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS released_at TIMESTAMPTZ;

UPDATE support_handoffs
SET priority = 'normal'
WHERE priority = '';

ALTER TABLE support_handoffs
    DROP CONSTRAINT IF EXISTS support_handoffs_status_check;

ALTER TABLE support_handoffs
    ADD CONSTRAINT support_handoffs_status_check
    CHECK (status IN ('open', 'assigned', 'released', 'resolved', 'reopened'));

ALTER TABLE support_handoffs
    DROP CONSTRAINT IF EXISTS support_handoffs_priority_check;

ALTER TABLE support_handoffs
    ADD CONSTRAINT support_handoffs_priority_check
    CHECK (priority IN ('low', 'normal', 'high', 'urgent'));

DROP INDEX IF EXISTS idx_support_handoffs_open_session;

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_handoffs_active_session
    ON support_handoffs(organization_id, session_id)
    WHERE status IN ('open', 'assigned', 'reopened');

CREATE INDEX IF NOT EXISTS idx_conversation_sessions_ops
    ON conversation_sessions(organization_id, conversation_status, assigned_user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_sessions_unread
    ON conversation_sessions(organization_id, unread_count, updated_at DESC)
    WHERE unread_count > 0;
