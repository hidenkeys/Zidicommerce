CREATE TABLE IF NOT EXISTS support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ticket_type TEXT NOT NULL DEFAULT 'complaint',
    status TEXT NOT NULL DEFAULT 'open',
    subject TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    media_url TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT support_tickets_status_check CHECK (status IN ('open', 'assigned', 'resolved', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_support_tickets_org_status
    ON support_tickets(organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_support_tickets_customer
    ON support_tickets(organization_id, customer_id, created_at DESC);

CREATE TABLE IF NOT EXISTS support_handoff_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    handoff_id UUID NOT NULL REFERENCES support_handoffs(id) ON DELETE CASCADE,
    actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_support_handoff_notes_handoff
    ON support_handoff_notes(organization_id, handoff_id, created_at ASC);
