# Merchant Knowledge/RAG Foundation

Phase D adds structured, tenant-scoped merchant knowledge to the existing Zidi AI grounding path. Phase D4 adds an optional pgvector-ready embedding foundation for approved active knowledge only, but it still does not add WhatsApp, Meta, Paystack, checkout actions, external URL crawling, file upload storage, or unreviewed document search.

## Architecture

The AI layer keeps commerce facts and merchant knowledge separate.

Commerce facts come from the commerce database tables and remain the source of truth for:

- products
- variants
- prices
- inventory
- stores
- orders

Merchant knowledge comes from tenant-scoped knowledge rows and is used for:

- policies
- FAQs
- business information
- delivery and shipping details
- returns and refunds
- warranty details
- location explanations
- payment information

The retrieval flow is:

```text
Customer message
  -> retrieval plan
  -> commerce retrieval when product/store/inventory/order facts are needed
  -> merchant knowledge retrieval when policy/FAQ/business info is needed
  -> GroundingContext
  -> deterministic fallback or grounded provider response
  -> deterministic validation
```

## Storage

Structured knowledge is stored in `merchant_knowledge_entries`.

Important columns:

- `organization_id`: tenant boundary. Retrieval always filters by this value.
- `kind`: one of `faq`, `policy`, `business_info`, `delivery`, `returns`, `warranty`, `location`, `payment_info`.
- `category`: a more specific category such as `delivery`, `returns`, `warranty`, or `opening_hours`.
- `title`: merchant-facing label.
- `question`: optional customer-facing question shape.
- `answer`: the authoritative answer the AI may use.
- `keywords`: JSON array used by lexical retrieval.
- `status`: only `active` rows are used by AI retrieval.
- `source_type`: currently `manual`; later phases can add document-derived sources.
- document-derived knowledge uses `source_type = document_chunk`.

Existing `bot_faqs` rows are still supported. The Phase D retriever reads both `merchant_knowledge_entries` and legacy `bot_faqs` so Phase C behavior remains compatible.

## Document Ingestion Lifecycle

Phase D3 introduces two tenant-scoped tables:

- `merchant_document_sources`: one pasted/manual source document per merchant.
- `merchant_document_chunks`: deterministic extracted chunks from that source.

The lifecycle is:

```text
Merchant pastes/manual-enters document text
  -> source row is created
  -> deterministic heading/paragraph chunking
  -> chunks are marked review_required
  -> merchant edits/reviews chunks
  -> approved chunk creates or updates merchant_knowledge_entries
  -> AI continues to retrieve only active structured knowledge
```

Unapproved chunks are never used by AI grounding. Draft, review-required, and archived document chunks remain review material only. The AI sees document-derived content only after a permitted user approves a chunk into an active `merchant_knowledge_entries` row.

Document ingestion intentionally avoids semantic document search, summarization jobs, and external crawlers. Pasted/manual text is chunked by headings and paragraphs with a maximum chunk size so humans can review each fact before activation.

Document source statuses:

- `draft`
- `processing`
- `extracted`
- `review_required`
- `active`
- `archived`
- `failed`

Document chunk statuses:

- `draft`
- `review_required`
- `approved`
- `archived`

Approving a chunk copies the chunk text into a structured knowledge entry. The approval form should choose the most specific `kind`, `category`, customer-facing `question`, and keywords. After approval, all existing structured retrieval/ranking rules apply.

## Document-To-Knowledge Traceability

Phase D3.5 makes approved document knowledge traceable without changing the AI retrieval architecture.

Every knowledge entry created from an approved document chunk uses:

- `source_type = document_chunk`
- `merchant_document_chunks.knowledge_entry_id` as the relational link
- `merchant_knowledge_entries.metadata` for human-readable trace metadata

Trace metadata includes:

- source document ID
- source chunk ID
- source title
- source label
- chunk title and index
- approval timestamp
- approving user ID

Approving the same chunk again updates the existing linked knowledge entry instead of creating duplicate active knowledge. This keeps the chunk-to-knowledge relationship stable while allowing a reviewer to correct the type, category, question, keywords, or chunk text.

## Structured Retrieval And Ranking

Phase D2 uses deterministic lexical ranking. Phase D4 keeps lexical ranking as the default and primary path. Vector search is optional and disabled unless explicitly configured.

Retrieval still starts from PostgreSQL and always filters by:

- `organization_id`
- `status = active`

Draft and archived entries are never used by AI grounding.

For every customer policy/FAQ/business-info question, the retriever:

1. Normalizes the query.
2. Infers the likely knowledge category.
3. Scores active structured knowledge and active legacy FAQs.
4. Selects the highest ranked relevant entries.
5. Reports debug metadata for tests and development.

The score considers:

- exact title/question match
- title/question phrase match
- keyword match
- category/type route match
- answer/content term match
- updated time as a tie-breaker only

Category routing gives preference to entries that match the customer's intent:

| Customer asks about | Preferred knowledge |
|---|---|
| delivery, shipping, riders | `delivery` |
| returns, refunds, exchanges | `returns`, then `policy` |
| warranty or guarantee | `warranty` |
| opening hours, branches, locations | `location`, `business_info` |
| card, transfer, cash, POS | `payment_info` |

Generic wording is capped when the category is wrong. For example, "What is your warranty policy?" must not match "What is your return policy?" just because both questions contain "what is your policy".

## Optional Pgvector Embeddings

Phase D4 stores embedding lifecycle metadata on `merchant_knowledge_entries`, the approved AI-facing knowledge table. Raw document chunks are not embedded for AI retrieval and unapproved chunks are still never grounded.

Embedding fields:

- `embedding`: vector-ready storage for the approved knowledge text.
- `embedding_status`: `disabled`, `pending`, `ready`, or `failed`.
- `embedding_model`: provider/model name used to generate the vector.
- `embedding_content_hash`: hash of the exact knowledge text used for embedding.
- `embedding_error`: last isolated embedding error, if generation failed.
- `embedded_at`: timestamp for the ready embedding.

Feature flags:

```bash
AI_EMBEDDINGS_ENABLED=false
AI_VECTOR_SEARCH_ENABLED=false
AI_EMBEDDING_PROVIDER=disabled
AI_EMBEDDING_MODEL=text-embedding-3-small
AI_EMBEDDING_DIMENSIONS=1536
AI_VECTOR_SEARCH_THRESHOLD=0.72
```

Supported local provider in this foundation:

- `AI_EMBEDDING_PROVIDER=local_hash`: deterministic local hash embeddings for development and tests.

Production external embedding providers are deliberately not wired yet. Normal tests use mocks and do not call live APIs.

The migration attempts to enable pgvector and promote `merchant_knowledge_entries.embedding` to `vector(1536)`. If pgvector is unavailable, the column remains text-compatible and vector search should stay disabled until the database supports the extension.

Embedding lifecycle:

```text
Active knowledge created/updated/approved
  -> embedding_status = pending
  -> knowledge.embedding.refresh job is queued when embeddings are enabled
  -> provider generates a vector from kind/category/title/question/answer/keywords
  -> embedding_status = ready
```

Draft and archived knowledge are marked `disabled`, their stored vector is cleared, and they are never vector candidates. Provider failures mark the entry `failed` but do not block normal AI answers; retrieval falls back to lexical ranking.

When vector search is enabled, retrieval is hybrid:

- lexical candidates remain available and exact lexical matches still win
- vector candidates can add approved active tenant-scoped knowledge
- vector matches include debug metadata for tests/development
- conflicts are still detected before grounding
- vector provider or pgvector errors fall back to lexical retrieval

Vector debug metadata is never shown to customers.

## Debug Metadata

The retrieval result includes development-only debug fields such as:

- score
- matched fields
- inferred category
- selected/skipped status
- skipped reason

This metadata is for tests and developer tooling. It is not included in customer-facing answers or prompt text.

## Conflict Handling

If multiple active entries from the same source strongly match the same customer question and have different answers, the retriever reports a conflict instead of silently choosing one.

When a conflict is detected:

- conflicting answers are withheld from authoritative grounding
- the grounding receives a `knowledge_conflict` unknown fact
- deterministic fallback tells the customer that the merchant has conflicting information

Current conflict detection is intentionally conservative. It focuses on strong duplicate or overlapping active entries with different answers, especially answers with different numbers such as "7 days" vs "30 days".

Legacy `bot_faqs` and structured `merchant_knowledge_entries` can coexist during migration. Conflicts are not raised across those two sources yet; structured knowledge should gradually become the canonical surface.

## Grounding

Merchant knowledge is added to `GroundingContext.Knowledge`. For compatibility with the existing validator and deterministic fallback, matching knowledge rows are also represented as grounded FAQ-style policy facts.

The AI may answer a policy/FAQ/business-info question only from matched tenant knowledge. If no matching row exists, it must answer that the merchant has not provided that information yet.

Commerce facts still override merchant knowledge text. For example, if a knowledge entry contains an old price note but the customer asks for the current price, the response must use the product variant price from the commerce database.

## Writing Good Entries

Merchants should write entries as direct facts the assistant may safely repeat.

Good entry shape:

- Use the most specific `kind`.
- Keep `category` short and predictable, such as `delivery`, `returns`, `warranty`, `opening_hours`, or `payment`.
- Put the customer wording in `question`.
- Put the exact approved response in `answer`.
- Add 2 to 6 keywords customers may use.
- Keep only one active answer for each policy question.
- Archive stale entries instead of leaving contradictory active entries.

Example:

- kind: `warranty`
- category: `warranty`
- title: `Warranty policy`
- question: `What is your warranty policy?`
- answer: `Sneakers have a 30-day warranty for verified manufacturing defects with receipt proof.`
- keywords: `warranty`, `guarantee`, `manufacturing defects`, `receipt`
- status: `active`

Updating the `answer`, `keywords`, or `status` takes effect on the next customer turn because lexical retrieval reads PostgreSQL directly. If embeddings are enabled, the row is marked `pending` for re-embedding, but missing or failed embeddings do not stop the current database-backed answer path.

For document-derived knowledge, edit either:

- the approved structured knowledge entry when the fact is already active, or
- the source/chunk and approve again when the document review text needs to become the new active fact.

## Archive Behavior

Archiving a document source always archives the source and its chunks.

By default, archiving a document source does not archive structured knowledge entries that were previously approved from those chunks. This is deliberate: after approval, `merchant_knowledge_entries` is the AI-facing source of truth.

To remove approved document-derived facts from AI grounding, archive linked knowledge explicitly:

```http
DELETE /v1/document-sources/{id}?archive_linked_knowledge=true
```

Without `archive_linked_knowledge=true`:

- the document source becomes archived
- its chunks become archived
- linked active knowledge remains active
- AI can still use those approved facts

With `archive_linked_knowledge=true`:

- the document source becomes archived
- its chunks become archived
- linked non-archived knowledge entries become archived
- AI stops using those linked facts on the next turn

The admin UI exposes these as separate actions so active AI knowledge is not silently removed.

## Regression Coverage

Phase D tests live in `apps/api/internal/ai/phase_d_knowledge_test.go`, `apps/api/internal/ai/phase_d2_knowledge_ranking_test.go`, `apps/api/internal/ai/phase_d3_document_ingestion_test.go`, `apps/api/internal/ai/phase_d4_embeddings_test.go`, `apps/api/internal/bot/service_test.go`, and `apps/api/internal/bot/document_ingestion_test.go` and prove:

- merchant knowledge is tenant-scoped
- unknown policy questions refuse instead of hallucinating
- updates are reflected immediately
- commerce prices and inventory still come from commerce tables
- policy grounding stays separate from product/inventory grounding
- exact title/question matches beat weak content matches
- keyword matches beat generic answer text
- category routing prefers delivery, warranty, and payment-specific entries
- draft and archived entries are ignored
- conflicting active entries are detected
- document sources and chunks are tenant-scoped
- unapproved chunks are not grounded
- approved chunks can become active structured knowledge
- archived chunks are ignored
- chunking is deterministic
- document updates refresh extracted chunks
- approved knowledge records source/chunk trace metadata
- approving the same chunk twice does not create duplicate active knowledge
- source-only archive keeps linked knowledge active
- explicit linked archive archives linked knowledge
- linked archive remains tenant-scoped
- AI ignores archived linked knowledge
- embeddings are disabled by default
- only active knowledge is marked pending for embedding
- draft/archived knowledge is not embedded for retrieval
- stale ready embeddings can be refreshed
- embedding provider failures are isolated
- vector retrieval ignores cross-tenant candidates
- vector retrieval falls back to lexical retrieval on provider failure
- hybrid retrieval can improve a semantic query in deterministic mock tests
- conflict handling still applies to vector candidates

Run:

```bash
cd apps/api
go test ./internal/bot -run Document
go test ./internal/ai -run 'TestPhaseD|TestPhaseD2|TestPhaseD3|TestPhaseD4'
```
