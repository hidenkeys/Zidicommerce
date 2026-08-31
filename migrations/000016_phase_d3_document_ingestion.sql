CREATE TABLE IF NOT EXISTS merchant_document_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('upload', 'url', 'pasted_text', 'manual')),
    status TEXT NOT NULL CHECK (status IN ('draft', 'processing', 'extracted', 'review_required', 'active', 'archived', 'failed')),
    original_filename TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    source_label TEXT NOT NULL DEFAULT '',
    mime_type TEXT NOT NULL DEFAULT 'text/plain',
    storage_key TEXT NOT NULL DEFAULT '',
    raw_text TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_document_sources_org_status_type
    ON merchant_document_sources(organization_id, status, source_type);

CREATE INDEX IF NOT EXISTS idx_document_sources_org_updated
    ON merchant_document_sources(organization_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS merchant_document_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    document_source_id UUID NOT NULL REFERENCES merchant_document_sources(id) ON DELETE CASCADE,
    knowledge_entry_id UUID REFERENCES merchant_knowledge_entries(id) ON DELETE SET NULL,
    chunk_index INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    heading TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'review_required', 'approved', 'archived')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, document_source_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_document_chunks_org_source_status
    ON merchant_document_chunks(organization_id, document_source_id, status);

CREATE INDEX IF NOT EXISTS idx_document_chunks_org_updated
    ON merchant_document_chunks(organization_id, updated_at DESC);
