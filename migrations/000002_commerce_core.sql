ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS logo_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'NGN',
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Africa/Lagos';

CREATE TABLE IF NOT EXISTS store_hours (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    opens_at TIME,
    closes_at TIME,
    is_closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, store_id, day_of_week)
);

CREATE TABLE IF NOT EXISTS store_fulfilment_modes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('pickup', 'customer_rider', 'merchant_rider')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    delivery_fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee_minor >= 0),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, store_id, mode)
);

CREATE TABLE IF NOT EXISTS cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    cart_id UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, cart_id, variant_id)
);

ALTER TABLE carts
    ADD COLUMN IF NOT EXISTS store_id UUID REFERENCES stores(id) ON DELETE SET NULL;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'awaiting_payment';
ALTER TABLE orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending', 'awaiting_payment', 'paid', 'processing', 'ready', 'out_for_delivery', 'completed', 'cancelled', 'refunded'));

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_fulfilment_type_check;
ALTER TABLE orders ADD CONSTRAINT orders_fulfilment_type_check
    CHECK (fulfilment_type IN ('pickup', 'customer_rider', 'merchant_rider'));

CREATE TABLE IF NOT EXISTS order_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, order_id, idempotency_key)
);

ALTER TABLE fulfilments DROP CONSTRAINT IF EXISTS fulfilments_type_check;
ALTER TABLE fulfilments ADD CONSTRAINT fulfilments_type_check
    CHECK (type IN ('pickup', 'customer_rider', 'merchant_rider'));

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS authorization_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_idempotency
    ON payments(organization_id, order_id, provider, idempotency_key)
    WHERE idempotency_key <> '';

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS phone_number_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS display_number TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS secret_config JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_provider_phone
    ON channels(provider, phone_number_id)
    WHERE phone_number_id <> '';

CREATE INDEX IF NOT EXISTS idx_store_hours_store ON store_hours(organization_id, store_id);
CREATE INDEX IF NOT EXISTS idx_store_fulfilment_modes_store ON store_fulfilment_modes(organization_id, store_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_cart ON cart_items(organization_id, cart_id);
CREATE INDEX IF NOT EXISTS idx_order_events_order ON order_events(organization_id, order_id);
