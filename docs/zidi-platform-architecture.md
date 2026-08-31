# Zidi Commerce Platform Architecture

This document describes the current ZidiCommerce repository as inspected in the codebase and the target architecture for turning it into the standalone multi-tenant Zidi Commerce SaaS platform.

No WhatsApp, Meta, Instagram, pgvector, new payment provider, or AI rewrite is implied by this document.

## 1. Current Architecture

ZidiCommerce is currently a modular monolith:

- Go/Fiber API in `apps/api`
- React/Vite merchant admin workspace in `apps/admin`
- React/Vite field-service workspace in `apps/field`
- PostgreSQL schema migrations in `migrations`
- Domain packages under `apps/api/internal`

The core backend domains are:

- `auth`: login, registration, JWT tokens, request user context
- `authz`: coarse roles and role helper methods
- `organization`: organization, user, membership, invitation, audit models
- `commerce/core`: stores, catalogue, inventory, customers, carts, orders, payments, fulfilment, channels, imports
- `bot`: bot builder, versions, modules, actions, questions, conditions, FAQs, merchant knowledge model
- `runtime`: deterministic bot runtime, sessions, messages, actions, handoff, outbound messages
- `ai`: grounded AI chat, retrieval planning, entity resolution, validation, provider abstraction, evaluation tests
- `jobs`: persisted background job queue
- `fieldservice`: field-service pilot domain

The shape is appropriate for this stage. The platform should stay a modular monolith until operational metrics prove a split is needed.

Phase G keeps that modular boundary: `bot` owns tenant workflow configuration, `runtime` owns conversation execution and action authorization, and `commerce/core` owns orders, payments, fulfilment, events, and store-scoped operations. `conversation_order_links` is the explicit continuity bridge between runtime and commerce; it is not a duplicate order or conversation state machine.

## 2. Current Database / Domain Map

The current schema already covers a large portion of the product vision.

Core tenant tables:

- `organizations`
- `users`
- `organization_memberships`
- `organization_invitations`
- `audit_logs`

Commerce:

- `stores`
- `store_hours`
- `store_fulfilment_modes`
- `store_user_assignments`
- `catalogue_categories`
- `products`
- `product_variants`
- `product_images`
- `inventory_levels`
- `customers`
- `carts`
- `cart_items`
- `orders`
- `order_items`
- `order_events`
- `commerce_events`
- `conversation_order_links`
- `fulfilments`

Payments and reliability:

- `payments`
- `payment_configurations`
- `payment_provider_secrets`
- `payment_webhook_events`
- `payment_reconciliations`
- `commerce_notifications`
- `background_jobs`

Bot, runtime, knowledge, conversations:

- `bots`
- `bot_versions`
- `commerce_workflow_configurations`
- `bot_version_modules`
- `bot_variables`
- `bot_questions`
- `bot_actions`
- `bot_conditions`
- `bot_integrations`
- `bot_steps`
- `bot_published_snapshots`
- `bot_faqs`
- `merchant_knowledge_entries`
- `conversation_sessions`
- `conversation_messages`
- `processed_messages`
- `runtime_events`
- `channel_outbound_messages`
- `support_handoffs`
- `support_tickets`
- `support_handoff_notes`

Channels:

- `channels`

Field-service pilot:

- `service_org_settings`
- `service_pools`
- `service_providers`
- `service_provider_pools`
- `service_requests`
- `service_matches`
- `service_dispatch_attempts`
- `service_assignments`
- `service_quotes`
- `service_quote_items`
- `service_ratings`
- `service_messages`

Missing platform domains:

- plans
- subscriptions
- invoices
- usage meters
- wallet or credit ledger
- channel credential references separate from raw channel config
- customer identities / channel identities
- fine-grained permissions
- configurable post-payment workflow tables
- observability metrics tables or integrations

## 3. Existing Functionality

Already implemented:

- Multi-tenant organization records
- User auth with JWT
- Organization membership and invitation flow
- Platform admin and merchant roles
- Store creation, fulfilment modes, store staff assignment
- Catalogue, variants, images, pricing
- Inventory levels and stock checks
- Customer records and default address
- Cart creation and item mutation
- Order creation and deterministic order transitions
- Durable commerce event stream for order, payment, and fulfilment lifecycle events
- Payment initialization, verification, provider abstraction, webhook idempotency
- Merchant payment configuration and encrypted payment provider secrets
- Runtime sessions, messages, idempotent inbound message handling
- Deterministic runtime action registry for commerce and support actions
- Support handoff, support tickets, handoff notes
- Bot builder data model, validation, publishing, immutable snapshots
- AI grounding, retrieval, validation, fallback, Ollama/Groq provider abstraction
- Tenant-scoped structured merchant knowledge retrieval
- Background job queue with retries and stale processing recovery
- Admin UI pages for the major operational areas
- Field-service pilot app and backend

## 4. Missing Functionality

Critical gaps:

- Fine-grained permission model beyond coarse roles
- Role-specific admin/storekeeper/support workspaces
- AI-to-deterministic commerce action orchestration
- Unified knowledge management UI for `merchant_knowledge_entries`
- Channel abstraction boundary before adding more external channels
- Billing/subscription/usage domain
- Configurable post-payment workflow engine
- Customer identity model that supports multiple channel identities
- Metrics, traces, and production dashboards

Important gaps:

- Agent inbox with assignment rules, SLA, private notes, resolution reasons
- Storekeeper order acceptance/preparation flow
- Inventory adjustment audit model
- Rich bot visual flow builder
- Load-test harness for conversations/runtime/AI
- Platform-wide usage metering

Future gaps:

- pgvector and document ingestion
- Instagram channel
- WhatsApp production rollout hardening
- Additional AI providers
- Tenant-level quotas and rate limits
- Multi-region deployment

## 5. Areas Requiring Refactor

The current system should not be rewritten, but these areas need deliberate hardening:

- RBAC is too coarse. `authz.Role` helpers are useful, but the mature platform needs resource permissions such as `orders.read`, `orders.transition`, `inventory.adjust`, `bot.publish`, and `payments.configure`.
- Runtime commerce actions execute using an internal merchant-admin actor. Session/customer checks are present, but AI/runtime mutations need a formal action policy boundary.
- `users.organization_id` and `organization_memberships` coexist. That is workable short term, but organization selection and membership source of truth should be made explicit.
- Channel-specific WhatsApp code exists inside runtime/public handlers. Future channels need a cleaner adapter boundary.
- Admin navigation is not sufficiently role-scoped. Store staff and support agents see a broad merchant workspace even when APIs restrict some operations.
- Knowledge is split between `bot_faqs` and `merchant_knowledge_entries`. Retrieval supports both, but management UI is still FAQ-oriented.
- Payment transaction handling exists, but SaaS billing is not a domain yet.

## 6. Proposed Target Architecture

Keep the modular monolith, but enforce clearer internal domain boundaries:

```text
apps/
  api/
    internal/
      identity/
      authorization/
      organization/
      commerce/
      payments/
      customers/
      conversations/
      support/
      botbuilder/
      runtime/
      ai/
      knowledge/
      channels/
      billing/
      jobs/
      observability/
  admin/
  field/
```

The current package names do not need to be changed immediately. The important target is ownership clarity:

- Commerce owns products, inventory, carts, orders, fulfilment.
- Payments owns transaction providers, payment records, webhooks, reconciliation.
- Billing owns Zidi subscriptions, invoices, usage, and wallet/credits.
- Runtime owns deterministic conversation execution.
- AI owns intent understanding, grounding, validation, and natural-language response generation.
- Channels own provider-specific inbound/outbound translation only.

## 7. Domain Boundaries

The target rule is:

```text
AI understands intent.
Zidi services execute state changes.
Database-backed services remain source of truth.
Channels only adapt transport.
```

Commerce operations must go through `commerce/core.Service` or a future domain service. AI, runtime, channels, and frontend screens must not mutate commerce tables directly.

Payment success must come only from payment verification or webhook handling, never AI text or frontend status assumptions.

## 8. Multi-Tenancy Model

Organization is the root tenant boundary.

Every tenant-owned table should have:

- `organization_id`
- indexes that start with `organization_id` for common tenant queries
- service-level filtering by authenticated/session organization
- no reliance on client-supplied organization IDs except platform-admin scoped operations

The organization source should be:

- authenticated JWT context for admin/API use
- channel configuration to organization mapping for public channel inbound messages
- persisted conversation session organization for subsequent conversation turns

The AI model must never supply organization identity.

## 9. Authentication / Authorization Model

Current roles:

- `platform_admin`
- `merchant_admin`
- `store_manager`
- `store_staff`
- `support_agent`
- `viewer`
- `service_provider`

Target authorization should add a permission layer:

- role grants permissions
- permissions can be organization scoped
- some permissions can be store scoped
- runtime/AI action execution checks capability before mutation

Examples:

- `stores.manage`
- `catalogue.manage`
- `inventory.read`
- `inventory.adjust`
- `orders.read`
- `orders.transition`
- `payments.configure`
- `bot.publish`
- `conversations.reply`
- `handoffs.claim`
- `billing.manage`

## 10. Store / Staff Permissions

The existing `store_user_assignments` table is the right store boundary.

Target behavior:

- merchant admins see all stores
- store managers/staff see only assigned stores
- order and inventory queries join through assignments
- frontend navigation changes based on role
- store staff should not see organization-wide settings, payment config, bot publish, billing, or platform organization screens

The first implementation phase should formalize these checks into shared policy helpers and regression tests.

## 11. Conversation Architecture

Current conversation storage is strong:

- `conversation_sessions`
- `conversation_messages`
- `processed_messages`
- `runtime_events`
- `support_handoffs`
- `support_tickets`
- `support_handoff_notes`

Target conversation states should conceptually support:

- `ai_active`
- `waiting_for_human`
- `assigned`
- `human_active`
- `resolved`

The current session states are `active`, `completed`, `handoff`, `expired`, and `cancelled`. These can be mapped without a disruptive migration at first. A richer handoff status model can live in `support_handoffs`.

## 12. AI Architecture Integration

The current AI architecture is sound and should be preserved:

```text
Customer message
-> session
-> immutable organization
-> security classification
-> conversation state
-> retrieval planning
-> scoped retrieval
-> entity resolution
-> GroundingContext
-> provider generation
-> validation
-> retry or fallback
-> response
```

The next AI integration step is not another provider. It is an action boundary:

```text
Grounded intent
-> typed action plan
-> confirmation and authorization
-> deterministic runtime/commerce action
-> structured result
-> grounded AI response
```

The LLM must not directly mutate database state.

## 13. Bot Runtime Architecture

The existing runtime already executes immutable published bot snapshots. It is channel-neutral at its core and uses an action registry.

Target direction:

- keep one runtime, not a second AI runtime
- allow scripted flows and AI-assisted flows to use the same deterministic action catalog
- keep draft builder rows out of runtime execution
- keep channel adapters thin

Runtime should own deterministic workflow execution. AI should become an optional intelligence layer for interpreting flexible natural language and selecting the next safe action.

## 14. Bot Builder Architecture

The bot builder already has the correct underlying primitives:

- bot
- version
- module
- variable
- question
- action
- condition
- step
- integration
- published snapshot

The admin UI is not yet a full visual builder. The target visual builder should map UI nodes onto the existing step/action/condition model rather than inventing a new persistence model.

Node categories should evolve gradually:

- conversation: message, question, AI response, handoff
- commerce: product search, variant selection, cart, order, inventory check
- payment: create payment, wait for payment, verify payment
- fulfilment: store selection, pickup, delivery, status update
- logic: condition, variable, branch, delay
- integration: webhook or external API later

## 15. Order State Machine

Current order states are:

- `awaiting_payment`
- `paid`
- `processing`
- `ready`
- `out_for_delivery`
- `completed`
- `cancelled`

Current transitions are deterministic and should remain source of truth.

Long-term conceptual states may include:

- `draft`
- `pending_payment`
- `payment_processing`
- `paid`
- `confirmed`
- `processing`
- `ready_for_fulfilment`
- `out_for_delivery`
- `delivered`
- `cancelled`
- `failed`
- `refunded`

Do not add all states speculatively. First harden current states, add merchant workflow configuration, then add new states only when workflows require them.

## 16. Payment Lifecycle

Current payment lifecycle:

```text
order awaiting_payment
-> payment initialized
-> payment pending
-> provider verification or webhook
-> amount/currency/provider checks
-> payment paid
-> order paid
-> order event and notification
-> AfterPaymentPaid hook
```

This is structurally strong.

Target additions:

- separate SaaS billing from merchant customer payments
- expose payment reconciliation status in operations UI
- add event workflow integration after payment confirmation
- keep provider secrets encrypted
- avoid hardcoded subscription pricing

## 17. Billing Architecture

Billing is missing and should be introduced as its own domain, not mixed with order payments.

Target entities:

- `billing_plans`
- `billing_plan_features`
- `organization_subscriptions`
- `billing_invoices`
- `billing_invoice_items`
- `usage_meters`
- `usage_records`
- optional `wallet_accounts`
- optional `wallet_ledger_entries`

Billing should support configurable limits:

- max stores
- max staff
- max channels
- included AI usage
- included messages
- add-on features
- billing status

Do not connect billing to a real payment provider in the first billing phase.

## 18. Channel Abstraction

Current `channels` records and runtime channel models exist. WhatsApp public endpoints and sender logic exist, but future work should not add more provider logic directly into runtime.

Target channel adapter contract:

```text
Verify inbound request
-> normalize inbound message
-> resolve organization/channel
-> conversation/AI/runtime processing
-> normalize outbound response
-> send via provider
-> record delivery status
```

Channel types should eventually include:

- `local_test`
- `web_chat`
- `whatsapp`
- `instagram`
- `other`

Do not implement new external channels until the adapter boundary is clean.

## 19. Knowledge / RAG Architecture

The current Phase D knowledge foundation is correct:

- `KnowledgeRetriever` abstraction
- `merchant_knowledge_entries`
- legacy `bot_faqs` compatibility
- tenant-scoped PostgreSQL retrieval
- grounded AI responses

Target evolution:

1. Structured knowledge UI and APIs for merchant knowledge entries.
2. Revisions/source metadata.
3. Document upload and chunking.
4. pgvector or another semantic index.
5. Source citations and freshness controls.

Do not add pgvector until structured knowledge is productized.

## 20. Scaling Architecture

Do not create one container per organization now.

Preferred architecture:

```text
Load balancer
-> stateless API instances
-> PostgreSQL
-> background_jobs queue
-> worker pool
-> AI providers / local model services
```

Tenant isolation is logical through `organization_id`, not physical infrastructure. Physical tenant isolation can be revisited only for enterprise plans, compliance requirements, or measured noisy-neighbor problems.

Future load-test harness should measure:

- concurrent conversations
- messages per second
- p50/p95/p99 latency
- database utilization
- queue depth
- worker throughput
- AI provider latency

## 21. Worker / Queue Architecture

Current `jobs.Service` is a good foundation:

- persisted `background_jobs`
- retry handling
- stale processing recovery
- PostgreSQL `FOR UPDATE SKIP LOCKED`
- idempotency by job type and key

Current registered jobs include channel outbound, notification delivery, and field-service dispatch timeout.

Target additions:

- job metrics
- dead-letter visibility
- tenant-level quota/concurrency controls
- operational dashboards
- event-driven workflow jobs

## 22. Observability

Current observability:

- request logging
- structured logs
- audit logs
- runtime events
- order events
- AI evaluation reports

Target observability:

- request ID propagation
- organization ID in logs
- conversation/order/payment correlation IDs
- provider latency metrics
- retrieval latency metrics
- validation failure counters
- queue depth and job age metrics
- channel delivery status metrics
- payment event dashboards
- audit log search and export

Do not log secrets, raw provider credentials, system prompts, or unnecessary customer-sensitive message content.

## 23. Security Model

Current strengths:

- JWT authentication
- tenant-scoped service queries
- store assignment checks in key commerce paths
- encrypted payment provider secrets
- webhook signature verification
- inbound message idempotency
- AI tenant isolation tests
- AI validation against hallucinated commerce facts

Current risks:

- role model is broad, not permission-based
- runtime action actor is too privileged internally
- channel secret config is not as formalized as payment secret storage
- frontend navigation is not a security boundary
- customer identity is still mostly direct customer/phone based

Target security:

- centralized authorization policy
- store-scoped resource checks
- runtime action capability checks
- secret references instead of secret JSON where possible
- audit trail for sensitive changes
- AI action audit logs
- channel credential hardening

## 24. Testing Strategy

Existing tests are strong around:

- AI regression and provider evaluation
- runtime lifecycle
- commerce service behavior
- jobs
- auth/authz
- field service
- migrations

Needed tests:

- permission matrix tests
- tenant-isolation tests for every new domain
- store-scope tests for store staff/manager
- API tests for role-specific behavior
- AI action orchestration tests
- conversation/handoff state tests
- billing ledger and usage tests
- channel adapter contract tests with fake adapters
- load-test harness for conversations/runtime/AI

Do not run large load tests yet. Build a harness first.

## 25. Migration Strategy

Use explicit SQL migrations only.

Migration rules:

- do not create speculative tables before the domain is being implemented
- add indexes with tenant access patterns in mind
- preserve existing working state machines unless a migration is justified
- keep backward compatibility with existing seed/demo data
- avoid storing credentials in plain config JSON

Near-term migrations should be limited to:

- permissions if the selected implementation requires tables
- customer/channel identity tables
- billing tables when billing phase starts
- workflow event tables when post-payment workflows are implemented

## 26. Recommended Implementation Phases

Recommended sequence:

1. Platform governance hardening: permissions, store/staff workspace rules, runtime action policy.
2. AI deterministic commerce orchestration: typed action plans, confirmation, cart/order/payment action boundary.
3. Customer/conversation/handoff productization: agent inbox, status model, assignments, support workflow.
4. Knowledge productization: structured merchant knowledge UI/API, legacy FAQ migration path.
5. Storekeeper workspace: order queue, accept/prepare/ready/delivered flow, assigned store inventory.
6. Bot builder UX: visual flow builder over current bot step/action model.
7. Billing foundation: plans, subscriptions, invoices, usage meters, no real billing provider yet.
8. Channel abstraction refactor: fake/local/web adapter boundary, no new external provider.
9. WhatsApp production hardening: only after channel abstraction and policy checks are stable.
10. Instagram and additional channels.
11. Scaling and observability: metrics, load harness, quota/rate limits, worker dashboards.

## Final Assessment

Already production-grade:

- AI grounding/evaluation foundation
- deterministic payment verification boundary
- explicit migrations
- core order/inventory transactional safety
- persisted runtime sessions and inbound idempotency
- encrypted payment secret storage

Structurally weak:

- permissions and role-scoped workspaces
- runtime action authorization model
- channel adapter separation
- billing absence
- knowledge management UI/API gap
- observability depth

Must be built next:

- centralized permission/resource policy
- store/staff workspace enforcement
- AI-to-deterministic action orchestration with confirmation and idempotency

Explicitly deferred:

- WhatsApp production rollout
- Instagram/Meta
- pgvector/document ingestion
- real SaaS billing payment collection
- per-organization containers
- microservices split

Recommended target architecture:

- shared stateless services
- tenant-scoped Postgres
- persisted worker queue
- channel-neutral runtime
- grounded AI as platform capability
- deterministic commerce/payment source of truth

Exact first implementation phase recommended:

Phase 1 should be Platform Governance Hardening: permissions, store/staff scope, runtime action policy, and tests. This reduces the largest risk before AI begins triggering deterministic commerce mutations.
