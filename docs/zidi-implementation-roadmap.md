# Zidi Commerce Implementation Roadmap

This roadmap is based on the current repository state. It intentionally avoids WhatsApp production rollout, Meta, Instagram, pgvector, new payment providers, and AI rewrites until the platform foundation is stronger.

## Current Implementation Status

Phases C through G foundations are now present: grounded AI validation, merchant knowledge lifecycle and hybrid retrieval, conversation/handoff operations, deterministic commerce actions/events, and merchant-configurable commerce workflow operations. Phase G remains intentionally limited to configuration, authorization, projections, and operator-driven transitions; general merchant-authored automation execution is deferred.

## Guiding Principles

- Organization is the tenant boundary.
- Never trust `organization_id` from the client or AI model.
- AI understands intent; deterministic Zidi services execute mutations.
- Payment success comes only from provider verification or webhook handling.
- Commerce lifecycle events should describe committed database facts, not model-generated text.
- Runtime and bot builder should not fork into duplicate state machines.
- Shared stateless services plus tenant-scoped data should come before per-tenant containers.

## Phase 1: Platform Governance Hardening

Objective: make the existing platform safe for role-specific operations and future AI-triggered commerce actions.

Components:

- permission matrix
- resource policy helpers
- store-scoped authorization helpers
- runtime action capability policy
- role-aware admin navigation
- tests for tenant and store isolation

Database:

- Prefer no migration initially.
- Add permission tables only if static role-to-permission code becomes insufficient.

API:

- Centralize permission checks used by commerce, runtime, bot, payments, and support.
- Ensure store staff/manager queries consistently use assigned stores.
- Ensure support agents cannot perform merchant-admin operations.

Frontend:

- Hide routes by role.
- Prepare separate storekeeper/support views inside `apps/admin`.

Tests:

- `go test ./internal/authz`
- commerce tenant/store-scope tests
- runtime action authorization tests
- API role tests

Acceptance:

- Store staff assigned to Store A cannot see or mutate Store B.
- Support agents can handle assigned conversations but cannot configure payments or publish bots.
- Runtime/AI write actions have an application-owned authorization gate.

## Phase 2: AI Deterministic Commerce Orchestration

Objective: let AI guide commerce operations without making the LLM source of truth.

Components:

- typed AI action model
- read/write action classification
- confirmation model
- pending action state
- deterministic action executor using runtime actions or commerce services
- structured `ActionResult`
- structured error codes
- AI action idempotency
- AI action audit metadata

Database:

- Start with `conversation_sessions.variables` for pending action state.
- Add an action audit table only if `audit_logs` metadata is insufficient.

API:

- Keep `/ai/test-chat` but evolve response metadata to expose action results safely.
- Do not expose internal IDs unnecessarily to customers.

Tests:

- AI action planner unit tests
- add-to-cart confirmation/idempotency tests
- order creation confirmation tests
- payment boundary tests
- tenant isolation tests

Acceptance:

- AI can add to cart only through deterministic services.
- AI cannot mark payment paid.
- Duplicate messages do not duplicate cart/order/payment writes.
- Phase C AI evaluation still passes.

## Phase 3: Conversation And Human Handoff Productization

Objective: turn existing runtime conversations into a real support workspace.

Components:

- agent inbox
- conversation filters
- handoff queue
- assignment and claiming rules
- public/internal notes
- resolution reasons
- AI pause/resume behavior
- support ticket detail view

Database:

- Extend handoff/ticket metadata only when required.
- Avoid duplicating `conversation_sessions`.

API:

- Add queue/filter endpoints if current list endpoints are insufficient.
- Enforce support-agent permissions.

Frontend:

- Dedicated support workspace for conversations and handoffs.
- Show customer/order context during handoff.

Tests:

- handoff lifecycle
- assignment permissions
- AI stops responding while human owns conversation
- tenant isolation

Acceptance:

- Human handoff can be operated end to end without merchant-admin access.

## Phase 4: Knowledge Productization

Objective: make merchant knowledge usable by merchants, not only tests or SQL.

Components:

- CRUD API for `merchant_knowledge_entries`
- admin UI for policies, FAQs, delivery, returns, warranty, business info
- legacy `bot_faqs` migration path
- source and status controls
- knowledge test coverage

Database:

- Use existing `merchant_knowledge_entries`.
- Add revision/history only if needed.

Tests:

- tenant-scoped knowledge CRUD
- unknown policy refusal
- immediate update freshness
- commerce facts override policy text

Acceptance:

- Merchants can manage structured knowledge from the dashboard.
- AI answers only from tenant knowledge.

## Phase 5: Storekeeper Workspace

Phase F foundation now exists before this phase: commerce actions are classified, direct AI write actions remain blocked, order/payment/fulfilment lifecycle events are emitted to `commerce_events`, checkout prices come from the database, payment initialization is limited to `awaiting_payment` orders, and paid-order cancellation is blocked for manual review/refund handling.

Objective: give store staff a focused operational experience.

Components:

- incoming orders queue
- order accept/reject if supported by state policy
- preparing/ready/dispatched/delivered actions
- assigned-store inventory visibility
- fulfilment detail panel
- handoff/conversation context where relevant

Database:

- Avoid new order states until the current state machine cannot express required behavior.
- Add store/order assignment metadata only when needed.

API:

- Store-scoped order and inventory endpoints can reuse current commerce service methods.

Frontend:

- Role-specific workspace in `apps/admin`, not a new app initially.

Tests:

- storekeeper cannot access unrelated stores
- order transition permission matrix
- fulfilment update permissions

Acceptance:

- A store staff user can operate assigned orders without seeing organization-wide settings.

## Phase 6: Bot Builder UX

Objective: evolve the current assistant configuration into a visual flow builder without replacing the current persistence model.

Components:

- visual node canvas
- node catalog over existing step/action/question/condition model
- validation feedback
- preview/test
- publish flow
- version diff or summary

Database:

- Reuse `bot_steps`, `bot_actions`, `bot_questions`, `bot_conditions`, `bot_variables`.
- Add layout metadata only if needed for canvas positions.

Tests:

- builder saves valid graph
- invalid graph rejected
- published snapshot remains immutable
- runtime executes published snapshot only

Acceptance:

- Merchants can configure default Zidi commerce flows visually.

## Phase 7: Billing Foundation

Objective: introduce Zidi SaaS billing as an internal domain without real provider collection yet.

Components:

- plans
- plan features
- organization subscriptions
- billing status
- usage meters
- invoices
- optional wallet/credits
- admin billing page

Database:

- Add billing tables explicitly.

API:

- Platform admin can manage plans.
- Merchant admin can view subscription, invoices, and usage.

Tests:

- plan limit checks
- usage aggregation
- tenant isolation
- billing permission tests

Acceptance:

- Platform can model subscription and usage without charging real money.

## Phase 8: Channel Abstraction

Objective: make channel providers thin adapters before expanding external integrations.

Components:

- channel adapter interface
- normalized inbound message model
- normalized outbound response model
- credential reference pattern
- fake/local/web adapter tests
- provider delivery status model

Database:

- Extend `channels` only if required.
- Consider separate secret reference storage for channel credentials.

Tests:

- fake channel adapter
- duplicate inbound idempotency
- outbound queue behavior
- tenant/channel ownership

Acceptance:

- Runtime/AI does not contain provider-specific business logic.

## Phase 9: WhatsApp Production Hardening

Objective: productionize WhatsApp only after the channel abstraction is ready.

Scope:

- webhook verification
- inbound normalization
- outbound rendering
- delivery status handling
- channel credential hardening
- operational dashboards

Out of scope until this phase:

- new WhatsApp business logic
- WhatsApp-specific AI behavior

Acceptance:

- WhatsApp uses the same conversation/AI/runtime/commerce path as local or web chat.

## Phase 10: Instagram / Meta Channels

Objective: add Instagram after proving the channel adapter model with local/web/WhatsApp.

Scope:

- adapter implementation
- credential setup
- inbound/outbound mapping
- delivery status
- tests with fake provider payloads

Acceptance:

- No duplicate AI or commerce runtime.

## Phase 11: Scaling And Observability

Objective: make the platform measurable before adding operational complexity.

Components:

- metrics
- traces
- queue dashboards
- provider latency dashboards
- AI retrieval/generation/validation metrics
- load-test harness
- tenant quotas
- worker concurrency controls

Load-test harness stages:

- 100 concurrent conversations
- 500 concurrent conversations
- 1,000 concurrent conversations
- 5,000 concurrent conversations

Metrics:

- messages/sec
- p50 latency
- p95 latency
- p99 latency
- queue depth
- database utilization
- worker utilization
- AI/provider latency

Acceptance:

- Scaling decisions are based on measured bottlenecks, not assumptions.

## Recommended Immediate Next Phase

Start with Phase 1: Platform Governance Hardening.

Reason:

- The current commerce and AI foundations are strong enough to build on.
- The biggest risk is not missing screens; it is broad permissions and privileged runtime action execution.
- AI commerce orchestration should not start mutating carts/orders/payments until authorization and action policy boundaries are clear.

## What Is Already Production-Grade

- explicit SQL migrations
- tenant-scoped core commerce tables
- transactional order creation and stock checks
- deterministic order transition service
- payment verification/webhook boundary
- encrypted payment provider secret storage
- runtime session persistence and inbound idempotency
- AI grounding/validation/evaluation harness

## What Is Structurally Weak

- permission model
- role-specific workspace behavior
- runtime action policy
- channel provider separation
- observability
- billing absence
- knowledge management product surface

## What Must Be Built Next

1. Permission/resource policy foundation.
2. Store/staff scoped workspace and tests.
3. Runtime/AI action authorization.
4. AI deterministic commerce orchestration.
5. Conversation/handoff productization.

## What Should Be Deferred

- WhatsApp production expansion
- Meta/Instagram
- pgvector/document ingestion
- real billing payment collection
- new payment providers
- container per organization
- microservices split
- full visual builder before policy hardening
