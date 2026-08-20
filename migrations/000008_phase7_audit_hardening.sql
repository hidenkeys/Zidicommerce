ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency
    ON orders(organization_id, idempotency_key)
    WHERE idempotency_key <> '';

ALTER TABLE inventory_levels DROP CONSTRAINT IF EXISTS inventory_levels_nonnegative_check;
ALTER TABLE inventory_levels ADD CONSTRAINT inventory_levels_nonnegative_check
    CHECK (on_hand >= 0 AND reserved >= 0 AND on_hand >= reserved);

CREATE INDEX IF NOT EXISTS idx_background_jobs_stale_processing
    ON background_jobs(status, locked_at)
    WHERE status = 'processing';

CREATE INDEX IF NOT EXISTS idx_channel_outbound_messages_sending
    ON channel_outbound_messages(status, updated_at)
    WHERE status = 'sending';
