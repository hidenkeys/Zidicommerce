CREATE TABLE IF NOT EXISTS commerce_workflow_configurations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    bot_display_name TEXT NOT NULL DEFAULT 'Store assistant',
    greeting TEXT NOT NULL DEFAULT 'Welcome. How can I help you today?',
    tone TEXT NOT NULL DEFAULT 'helpful',
    ordering_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    payment_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    human_handoff_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    store_selection_strategy TEXT NOT NULL DEFAULT 'customer_choice'
        CHECK (store_selection_strategy IN ('customer_choice', 'single_store', 'first_available', 'nearest', 'merchant_rule')),
    enabled_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    supported_fulfilment_modes JSONB NOT NULL DEFAULT '["pickup","customer_rider","merchant_rider"]'::jsonb,
    post_payment_steps JSONB NOT NULL DEFAULT '["notify_customer","notify_store","merchant_prepares","enable_tracking"]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id)
);

CREATE TABLE IF NOT EXISTS conversation_order_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    conversation_session_id UUID NOT NULL REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    source TEXT NOT NULL DEFAULT 'runtime',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, conversation_session_id, order_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_order_links_order
    ON conversation_order_links(organization_id, order_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_order_links_session
    ON conversation_order_links(organization_id, conversation_session_id, created_at DESC);
