CREATE TABLE IF NOT EXISTS merchant_knowledge_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL DEFAULT 'faq' CHECK (kind IN ('faq', 'policy', 'business_info', 'delivery', 'returns', 'warranty', 'location', 'payment_info')),
    category TEXT NOT NULL DEFAULT 'general',
    title TEXT NOT NULL,
    question TEXT NOT NULL DEFAULT '',
    answer TEXT NOT NULL,
    keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_type TEXT NOT NULL DEFAULT 'manual',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'archived')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_merchant_knowledge_org_status_kind
    ON merchant_knowledge_entries(organization_id, status, kind, category);
