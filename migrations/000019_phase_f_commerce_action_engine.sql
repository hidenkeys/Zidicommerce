CREATE TABLE IF NOT EXISTS commerce_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'system',
    resource_type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    order_id UUID REFERENCES orders(id) ON DELETE CASCADE,
    payment_id UUID REFERENCES payments(id) ON DELETE CASCADE,
    fulfilment_id UUID REFERENCES fulfilments(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_commerce_events_org_created
    ON commerce_events(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_commerce_events_type
    ON commerce_events(organization_id, event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_commerce_events_order
    ON commerce_events(organization_id, order_id, created_at DESC)
    WHERE order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_commerce_events_payment
    ON commerce_events(organization_id, payment_id, created_at DESC)
    WHERE payment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_commerce_events_fulfilment
    ON commerce_events(organization_id, fulfilment_id, created_at DESC)
    WHERE fulfilment_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_commerce_events_idempotency
    ON commerce_events(organization_id, event_type, idempotency_key)
    WHERE idempotency_key <> '';
