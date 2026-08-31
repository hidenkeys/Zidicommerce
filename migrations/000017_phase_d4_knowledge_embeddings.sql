DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION
    WHEN undefined_file THEN
        RAISE NOTICE 'pgvector extension is not installed; vector search will remain disabled until it is available';
END $$;

ALTER TABLE merchant_knowledge_entries
    ADD COLUMN IF NOT EXISTS embedding TEXT,
    ADD COLUMN IF NOT EXISTS embedding_status TEXT NOT NULL DEFAULT 'disabled',
    ADD COLUMN IF NOT EXISTS embedding_model TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embedding_content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embedding_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embedded_at TIMESTAMPTZ;

ALTER TABLE merchant_knowledge_entries
    DROP CONSTRAINT IF EXISTS merchant_knowledge_entries_embedding_status_check;

ALTER TABLE merchant_knowledge_entries
    ADD CONSTRAINT merchant_knowledge_entries_embedding_status_check
    CHECK (embedding_status IN ('disabled', 'pending', 'ready', 'failed'));

CREATE INDEX IF NOT EXISTS idx_merchant_knowledge_embedding_status
    ON merchant_knowledge_entries(organization_id, status, embedding_status, updated_at DESC);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        EXECUTE $ddl$
            ALTER TABLE merchant_knowledge_entries
                ALTER COLUMN embedding TYPE vector(1536)
                USING CASE
                    WHEN embedding IS NULL OR embedding = '' THEN NULL
                    ELSE embedding::vector
                END
        $ddl$;

        EXECUTE $ddl$
            CREATE INDEX IF NOT EXISTS idx_merchant_knowledge_embedding_vector
                ON merchant_knowledge_entries
                USING hnsw (embedding vector_cosine_ops)
                WHERE status = 'active' AND embedding_status = 'ready' AND embedding IS NOT NULL
        $ddl$;
    ELSE
        RAISE NOTICE 'pgvector extension is not active; skipped merchant knowledge embedding vector column and index';
    END IF;
END $$;
