# Zidi Commerce Final Product Architecture And Roadmap

This document is an implementation architecture plan for evolving the current ZidiCommerce monorepo into a standalone multi-tenant commerce, conversation, AI, and workflow platform.

It is based on the current repository state. It does not instruct implementation of WhatsApp, Meta, Instagram, Paystack, pgvector, SaaS billing, or new channel integrations in the immediate next phase.

## 1. Current State Assessment

### What Exists

The active product lives in `Zidicommerce` as a modular monolith:

- Go/Fiber API in `apps/api`.
- React/Vite merchant admin in `apps/admin`.
- React/Vite field-service workspace in `apps/field`.
- SQL migrations in `migrations`.
- PostgreSQL as the source of truth.
- Tenant root through `organizations` and organization-scoped domain tables.

Implemented backend domains:

- `auth`: login, registration, JWT, request user context.
- `authz`: roles, centralized permissions, runtime action policy.
- `organization`: organizations, memberships, invitations, audit logs.
- `commerce/core`: stores, catalogue, variants, inventory, customers, carts, orders, payments, fulfilment, channels, imports.
- `bot`: bot setup, versions, questions, actions, conditions, steps, published snapshots, FAQs, structured knowledge model.
- `runtime`: deterministic bot runtime, conversation sessions, messages, processed inbound messages, runtime actions, outbound records, support handoffs, support tickets.
- `ai`: grounded customer-service AI with security classification, retrieval planning, deterministic retrieval, entity resolution, grounding, response validation, provider abstraction, live evaluations.
- `jobs`: database-backed background jobs.
- `fieldservice`: vertical pilot domain, separate from core commerce.

Implemented product surfaces:

- Admin shell and pages for overview, setup, stores, catalogue, inventory, customers, orders, payments, assistant, AI chat, conversations, knowledge, team, business settings, WhatsApp settings, imports, and audit logs.
- Role and permission-aware navigation is present as a UX layer.
- Backend permission checks are enforced in services and route middleware.

### What Is Partially Implemented

- Merchant knowledge exists as structured PostgreSQL rows in `merchant_knowledge_entries`, but merchant-facing CRUD and quality controls are still thin.
- Bot builder persistence exists, including versions, steps, actions, conditions, and immutable published snapshots, but the visual builder is not yet productized.
- Runtime actions exist and can execute commerce operations deterministically, but AI-triggered write orchestration is intentionally blocked until confirmation and idempotency rules are stronger.
- Human handoff tables and service flows exist, but the agent inbox needs better assignment, filtering, SLA, and resolution workflows.
- Payments exist for merchant-customer transactions, provider abstraction, secret storage, webhook idempotency, and reconciliation, but Zidi SaaS billing, wallet, usage, and subscriptions do not exist yet.
- Channels exist as data/config plus some existing WhatsApp-related runtime/public handlers, but the future adapter boundary should be hardened before adding production channels.

### What Is Missing

- Dedicated billing/subscriptions/wallet domain for Zidi platform fees and usage.
- Customer channel identity model for mapping one customer across WhatsApp, Instagram, web chat, and future channels.
- Production-grade channel adapter boundary and provider rate-limit management.
- Workflow engine abstraction above bot steps for merchant-configurable deterministic business flows.
- Visual drag-and-drop builder over the workflow/bot data model.
- Merchant-facing structured knowledge CRUD with review status, history, source metadata, and conflict checks.
- Document ingestion and embeddings.
- Usage quotas, tenant limits, provider budget controls, and analytics.
- Load testing harness for runtime, AI, and channel traffic.

### Where AI And Merchant Knowledge Fit

The current AI layer is the right foundation. It should remain a grounded reasoning layer, not a business action executor.

Current AI flow:

```text
Customer message
  -> conversation session
  -> immutable organization ID
  -> security classification
  -> conversation state
  -> retrieval planning
  -> deterministic retrieval from PostgreSQL
  -> entity resolution
  -> GroundingContext
  -> LLM response generation
  -> deterministic response validation
  -> retry or deterministic fallback
  -> response
  -> state update
```

Merchant knowledge extends grounding for policies, FAQs, delivery, returns, warranty, business info, payment info, and location explanations. Commerce tables remain authoritative for products, prices, inventory, stores, orders, fulfilment, and payments.

## 2. Recommended Domain Architecture

Keep the modular monolith for now. Split into services only when operational pressure proves a need.

Target backend domain ownership:

| Domain | Owns | Current home | Direction |
|---|---|---|---|
| Organizations / tenancy | tenant root, org profile, current org context | `organization`, `tenant`, `commerce/core` | Keep org ID as root scope for every request. |
| Users / roles / permissions | auth, memberships, invitations, permissions | `auth`, `authz`, `organization` | Keep central permission matrix; later add DB grants if needed. |
| Stores | stores, hours, fulfilment modes, staff assignments | `commerce/core` | Harden store-scoped UX and policy checks. |
| Products / catalogue / inventory | categories, products, variants, prices, stock | `commerce/core` | Keep database source of truth. Add inventory audit later. |
| Customers | customer profile and default address | `commerce/core` | Add channel identities later. |
| Orders | carts, orders, order items, order events | `commerce/core` | Tighten state machine and idempotency for automated flows. |
| Payments / transactions | merchant payment config, payments, webhooks, reconciliation | `commerce/core` | Keep separate from Zidi SaaS billing. |
| Wallet / billing / subscriptions | Zidi fees, plans, invoices, usage, credits | missing | Add as a new `billing` domain later. |
| Conversations / messages | sessions, messages, processed message idempotency | `runtime` | Productize inbox and channel-neutral message model. |
| Human handoff / agent inbox | handoffs, assignment, notes, escalation | `runtime` | Expand to dedicated support workspace. |
| Complaints | support tickets, complaint lifecycle | `runtime` | Add complaint status taxonomy and dashboards. |
| Audit logs | security, authz, sensitive config, AI/runtime actions | `organization`, commerce audit helpers | Standardize event names and metadata. |
| Bot settings | bots, versions, modules, questions, actions, conditions, snapshots | `bot` | Reuse for builder foundation. |
| Workflow engine | deterministic graph execution, triggers, confirmation gates | partly `bot` and `runtime` | Add workflow abstraction without replacing runtime. |
| Merchant knowledge / RAG | structured knowledge, documents, embeddings later | `bot`, `ai` | Productize structured knowledge first; defer vector search. |
| Channel adapters | WhatsApp, Instagram, web chat translation | partly `runtime`, `commerce/core` channels | Harden adapter contract before external rollout. |
| Analytics / observability | metrics, traces, usage, provider latency, queue health | partial logs/reports/jobs | Add operational metrics after core flows stabilize. |

Target dependency rule:

```text
Channels -> Conversation Engine -> AI / Runtime -> Domain Services -> PostgreSQL
                                      |
                                      -> Jobs / Outbound Queue
```

AI can read grounded facts and propose intent. Runtime/workflow/domain services execute deterministic actions.

## 3. Multi-Tenant Scaling Strategy

Zidi should not create one bot container per organization.

Per-organization containers would create operational sprawl: duplicated idle resources, inconsistent deployments, complex incident response, slow tenant onboarding, and harder provider-rate-limit coordination. The platform should scale shared stateless services horizontally while scoping every request by organization ID.

Recommended scaling model:

```text
Load balancer
  -> API instances, stateless
  -> PostgreSQL, tenant-scoped data
  -> Redis or queue infrastructure
  -> worker pools
       -> channel workers
       -> AI workers
       -> workflow workers
       -> billing/usage workers
```

API instances:

- Stateless, horizontally scaled.
- Authenticate requests and resolve organization context.
- Avoid in-memory tenant state except short-lived caches with org-aware keys.

Queue workers:

- Process outbound messages, retries, webhooks, reconciliation, imports, document ingestion, usage aggregation.
- Use idempotency keys and organization IDs in job payloads.

AI workers:

- Optional separation once latency and concurrency require it.
- Same grounding contract as API.
- Organization quotas and provider keys should be enforced before provider calls.

Workflow workers:

- Execute deterministic long-running steps such as wait-for-payment, fulfilment progression, notification retries, and delayed handoff checks.

Redis or queue infrastructure:

- Use for rate limiting, distributed locks, delayed jobs, and fast session coordination only where PostgreSQL is too slow.
- Keep PostgreSQL as durable truth.

PostgreSQL tenant scoping:

- Every tenant-owned table keeps `organization_id`.
- Index common queries starting with `organization_id`.
- Service methods never trust client-supplied org IDs for merchant requests.
- Platform-admin operations remain explicit and audited.

Provider-level rate limits:

- Track per-provider and per-model limits.
- Queue or shed traffic before hitting hard 429s.
- Separate provider/runtime failures from AI answer failures in evaluations.

Organization-level quotas:

- Add usage meters for messages, AI calls, tokens, documents, storage, and channels.
- Enforce soft warnings before hard blocks.

Future enterprise isolation:

- Start with shared app and shared database with strong tenant scoping.
- Offer dedicated DB/schema or dedicated worker pools only for high-value enterprise tenants with compliance needs.
- Do not fork code paths by tenant.

## 4. Conversation And AI Architecture

Target message flow:

```text
Channel adapter
  -> normalize inbound event
  -> resolve organization/channel/customer identity
  -> Conversation Engine
  -> idempotency check
  -> append customer message
  -> security classification
  -> AI retrieval and grounding
  -> intent / action plan
  -> deterministic workflow decision
  -> confirmation gate if mutation is needed
  -> domain service execution
  -> response generation
  -> outbound adapter
  -> append assistant/system messages
```

Channel adapters should only:

- Verify provider signatures.
- Normalize provider payloads into internal message commands.
- Map provider customer IDs to internal customer identities.
- Send outbound messages.
- Record delivery events.

AI should:

- Classify customer intent.
- Ask clarifying questions.
- Answer from grounded commerce and knowledge facts.
- Suggest a deterministic action plan.
- Explain next steps in natural language.

AI must not directly execute dangerous business actions.

Deterministic services must execute:

- inventory checks
- cart creation and mutation
- order creation
- payment link generation
- fulfilment updates
- refunds and cancellations
- wallet or billing changes

Mutation pattern:

```text
AI proposes ActionIntent
  -> validator checks grounding and permissions
  -> workflow creates PendingAction
  -> customer confirms
  -> deterministic executor calls domain service
  -> result is persisted and audited
  -> AI summarizes result from ActionResult
```

## 5. Workflow / Bot Builder Architecture

The repo already has the data foundation for bot versions, steps, questions, actions, conditions, modules, integrations, and published snapshots. The future builder should build on this instead of introducing a second workflow engine.

Core concepts:

- Workflow definition: merchant-owned flow definition.
- Workflow version: draft, validated, published, archived.
- Node/step: one executable or display unit.
- Edge/transition: next step, conditional branch, fallback.
- Trigger: inbound message, intent, event, payment success, order status change.
- Condition: deterministic boolean rules.
- Action: deterministic service call or safe outbound event.
- Confirmation gate: required pause before state-changing operations.
- Published snapshot: immutable runtime artifact.
- Test simulator: run flow with fake events and inspect state.

Suggested node types:

- `send_message`
- `ask_question`
- `collect_input`
- `classify_intent`
- `search_product`
- `check_inventory`
- `create_cart`
- `generate_payment_link`
- `wait_for_payment`
- `create_order`
- `assign_rider`
- `update_fulfillment`
- `handoff_to_agent`
- `close_conversation`

Builder safety rules:

- Draft versions are editable.
- Published versions are immutable.
- Runtime executes only published snapshots.
- State-changing nodes require explicit capability policy.
- Payment success nodes can only be driven by verified payment events.
- AI classifier nodes can route, but cannot mutate commerce directly.

## 6. Role-Based Dashboards

Organization admin:

- Overview, setup checklist, stores, catalogue, inventory, customers, orders, conversations, complaints, knowledge, bot settings, staff, audit logs, settings.
- Can configure organization, stores, staff, bot, knowledge, channels, and payments.

Store manager:

- Assigned-store orders, assigned-store inventory, products/catalogue view, customers, conversations related to assigned stores, handoffs, operational reports.
- Can update assigned-store inventory and order states.

Storekeeper / shopkeeper:

- Focused operational queue: new orders, preparation status, pickup/delivery readiness, inventory availability, order/customer contact context.
- No global settings, billing, payment configuration, or organization-wide staff management.

Support agent:

- Conversation inbox, open and assigned handoffs, customer/order context, complaint tickets, internal notes, resolution actions.
- Read-only commerce context, no payment configuration or bot publishing.

Billing/admin owner:

- Subscription plan, invoices, wallet/credits, usage, channel costs, AI/token usage, payment status.
- This should become part of merchant-admin capability or a future billing-specific role.

## 7. Data Model Roadmap

Already exists:

- Organizations, users, memberships, invitations, audit logs.
- Stores, store hours, fulfilment modes, store staff assignments.
- Catalogue categories, products, variants, product images.
- Inventory levels.
- Customers.
- Carts and cart items.
- Orders, order items, order events.
- Fulfilments.
- Payments, payment configuration, provider secrets, webhook events, reconciliation.
- Channels.
- Bots, bot versions, modules, variables, questions, actions, conditions, integrations, steps, published snapshots.
- Bot FAQs and merchant knowledge entries.
- Conversation sessions, messages, processed messages, runtime events, outbound messages.
- Support handoffs, support tickets, support handoff notes.
- Background jobs.

Add soon:

- `customer_identities`: organization ID, customer ID, channel, provider customer ID, profile metadata.
- `knowledge_entry_revisions`: optional, for audit/history after CRUD exists.
- `workflow_triggers`: if current bot steps cannot express event-driven flows cleanly.
- `pending_actions`: if conversation variables become too weak for confirmed mutations.
- `inventory_adjustments`: explicit adjustment audit for stock changes.
- `conversation_assignments` or richer handoff assignment metadata if current handoffs become limiting.

Add later:

- `plans`, `plan_features`, `organization_subscriptions`, `subscription_events`.
- `usage_meters`, `usage_events`, `invoices`, `invoice_items`.
- `wallet_accounts`, `wallet_ledger_entries`, `credit_grants`.
- `document_sources`, `document_chunks`, `embedding_jobs`.
- pgvector columns or vector indexes after structured knowledge is productized.
- `provider_rate_limits` and `organization_quota_counters`.
- analytics rollups for conversations, response times, AI usage, orders, and revenue.

Defer:

- Dedicated per-tenant schemas.
- Per-tenant containers.
- Multi-region data partitioning.
- Channel-specific commerce state tables unless provider behavior forces them.

## 8. Phase Roadmap

### Phase D1: Structured Merchant Knowledge Hardening

Goal: make current PostgreSQL knowledge reliable and merchant-manageable.

Work:

- CRUD API for `merchant_knowledge_entries`.
- Admin knowledge UI for FAQ, policy, business info, delivery, returns, warranty, locations, payment info.
- Validation for kind, category, title, answer, keywords, status.
- Audit events for create/update/archive.
- Preserve legacy `bot_faqs` compatibility.
- Regression tests for tenant scope, unknowns, freshness, and commerce-fact precedence.

### Phase D2: Knowledge Retrieval Quality And Ranking

Goal: improve structured retrieval before vector search.

Work:

- Better lexical scoring and category routing.
- Synonym tables per domain.
- Conflict detection between knowledge entries.
- Top-k explainability in debug output.
- Tests for ambiguous policy questions and multi-policy questions.

### Phase D3: Document Ingestion Foundation

Goal: support documents without embeddings first.

Work:

- Document source records.
- Upload/import metadata.
- Text extraction job interface.
- Chunk table without vector index.
- Manual review before chunks become active knowledge.

### Phase D4: pgvector Embeddings

Goal: semantic retrieval after structured lifecycle is stable.

Work:

- Embedding provider abstraction.
- Chunk embeddings.
- Hybrid lexical plus vector retrieval.
- Reindex jobs.
- Evaluation suite for document-grounded answers.

### Phase E: Conversation Engine And Human Handoff

Goal: productize conversations and support workflows.

Work:

- Agent inbox filters.
- Handoff assignment, claim, transfer, resolve, reopen.
- Internal/public notes.
- AI pause/resume while human owns conversation.
- Customer and order context panel.
- SLA and priority metadata.

### Phase F: Deterministic Workflow Engine

Goal: formalize deterministic execution and confirmation gates.

Work:

- Typed action intents.
- Pending action model.
- Idempotent deterministic executor.
- Confirmation gates for mutations.
- Event-driven triggers.
- Workflow test simulator.

### Phase G: Bot Builder UI / Backend

Goal: expose workflows visually.

Work:

- Canvas layout metadata.
- Node catalog.
- Edge validation.
- Draft/validate/publish UX.
- Runtime preview.
- Immutable published snapshots.

### Phase H: Payments / Orders / Fulfillment Automation

Goal: automate commerce safely after workflow gates exist.

Work:

- Payment-link generation as deterministic action.
- Verified payment events drive order/fulfilment changes.
- Storekeeper preparation flow.
- Delivery/pickup state machine.
- Refund/cancellation policies with strict permissions.

### Phase I: Channel Adapters: WhatsApp, Instagram, Web

Goal: add production channels only after conversation, workflow, and quotas are ready.

Work:

- Channel adapter interface.
- Provider signature verification.
- Customer identity mapping.
- Outbound queue and delivery receipts.
- Provider rate limits.
- Web chat first if a controlled internal channel is useful.
- WhatsApp and Instagram after adapter hardening.

### Phase J: Production Hardening, Billing, Quotas, Observability

Goal: operate the platform as SaaS.

Work:

- Plans, subscriptions, invoices, wallet/credits.
- Usage meters for messages, AI calls, tokens, documents, channels.
- Quotas and soft/hard limits.
- Metrics, tracing, dashboards, alerting.
- Load testing and capacity targets.
- Security reviews for secrets, audit, and admin operations.

## 9. Testing Strategy

Preserve the current AI quality bar:

- Keep deterministic AI regression tests.
- Keep live provider evaluations for Ollama and Groq.
- Keep separate reporting for semantic, hallucination, tenant isolation, and runtime/provider failures.
- Add every real AI regression as a deterministic test first.

Expand tests by layer:

- Tenant isolation: every new domain gets cross-tenant read/write denial tests.
- Permissions: matrix tests for admin, store manager, store staff, support agent, viewer, platform admin.
- Store scope: assigned-store and unassigned-store query/mutation tests.
- Knowledge: CRUD, status filtering, ranking, unknown refusal, immediate freshness.
- Workflow: graph validation, published snapshot immutability, deterministic step execution, confirmation gates.
- Orders/payments: idempotency keys, duplicate message protection, stock reservation behavior, payment webhook replay.
- Channels: signature verification, inbound idempotency, outbound retries, delivery status mapping.
- Runtime: pending action state, human handoff pause/resume, support assignment scope.
- Load testing: simulate channel inbound events, AI calls, runtime actions, and outbound queues.

Initial mock throughput target:

- 50 inbound customer messages per second across tenants without AI provider calls.
- 10 AI-grounded messages per second with a mocked provider.
- 2 to 5 live-provider messages per second per provider account, depending on provider limits.
- Queue backlog should drain within 60 seconds after a 5-minute burst at 2x normal traffic.

Load tests must report:

- p50/p95/p99 latency.
- DB query time.
- queue depth and drain rate.
- provider latency.
- provider 429 count.
- per-organization quota behavior.

## 10. Recommended Immediate Next Step

The next implementation phase should be Phase D1: Structured Merchant Knowledge Hardening.

Reason:

- The database table and AI grounding path already exist.
- It is low-risk compared with channels, payments, or workflow mutation.
- It strengthens the current AI value without weakening tenant isolation.
- It gives merchants a real way to manage the facts the AI is allowed to use.
- It preserves the Phase C evaluation behavior because commerce facts remain separate and authoritative.

Do not jump to WhatsApp yet. The channel layer should wait until knowledge, conversation/handoff, deterministic workflow confirmation, and quota boundaries are stronger.

## Recommended Next Codex Prompt

```text
Implement Phase D1 structured merchant knowledge hardening in ZidiCommerce.

Constraints:
- Do not add WhatsApp, Meta, Instagram, Paystack, pgvector, billing, or new channel integrations.
- Do not rewrite the AI architecture.
- Preserve Phase C AI evaluation behavior, tenant isolation, and database-as-source-of-truth behavior.

Tasks:
1. Add backend CRUD API for merchant_knowledge_entries with tenant scoping, permissions, validation, and audit logs.
2. Add admin UI support for structured knowledge entries: FAQ, policy, business_info, delivery, returns, warranty, location, payment_info.
3. Keep bot_faqs compatibility but make merchant_knowledge_entries the preferred surface.
4. Add deterministic tests for tenant isolation, unknown policy refusal, immediate update freshness, and commerce fact precedence.
5. Run go test ./internal/ai, go test ./..., go test -race ./..., and npm run build:admin.
```
