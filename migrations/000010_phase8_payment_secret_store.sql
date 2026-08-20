CREATE TABLE IF NOT EXISTS payment_provider_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL,
    secret_name TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    nonce TEXT NOT NULL,
    key_version TEXT NOT NULL DEFAULT 'v1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, provider, secret_name)
);

CREATE INDEX IF NOT EXISTS idx_payment_provider_secrets_org_provider
    ON payment_provider_secrets(organization_id, provider);
