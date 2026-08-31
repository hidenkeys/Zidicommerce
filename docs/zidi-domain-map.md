# Zidi Commerce Domain Map

This map compares the current repository to the target Zidi Commerce platform vision.

| Domain | Existing | Partial | Missing | Existing files | Proposed work |
|---|---:|---:|---:|---|---|
| Organization | Yes |  |  | `internal/organization`, `internal/commerce/core/service.go`, `organizations` | Keep organization as tenant root. Clarify multi-org membership/session behavior. |
| Authentication | Yes |  |  | `internal/auth`, `users` | Keep JWT auth. Later add refresh/session management if needed. |
| Membership / Staff | Yes | Partial |  | `organization_memberships`, `organization_invitations`, `Team.tsx` | Add permission model and role-specific workspace behavior. |
| RBAC / Permissions |  | Partial | Yes | `internal/authz/roles.go`, handler `RequireRole` calls | Add centralized permission/resource policy and tests. |
| Store Management | Yes |  |  | `stores`, `store_hours`, `store_fulfilment_modes`, `Stores.tsx` | Harden store-level permissions and storekeeper UX. |
| Store Staff Assignment | Yes | Partial |  | `store_user_assignments`, `Team.tsx`, commerce store/order queries | Expand test coverage and shared policy helpers. |
| Catalogue | Yes |  |  | `products`, `product_variants`, `catalogue_categories`, `Catalogue.tsx` | Keep current model; add richer product ops later only when needed. |
| Inventory | Yes | Partial |  | `inventory_levels`, `Inventory.tsx`, commerce service | Add adjustment audit and storekeeper stock workflows. |
| Customers | Yes | Partial |  | `customers`, `Customers.tsx` | Add `customer_identities` / channel identities later. |
| Carts | Yes | Partial |  | `carts`, `cart_items`, commerce service, runtime actions | Add AI action orchestration and idempotency around cart writes. |
| Orders | Yes | Partial |  | `orders`, `order_items`, `order_events`, `commerce_events`, `conversation_order_links`, `Orders.tsx` | Phase G adds store-scoped operations projections and conversation continuity; richer exception/refund workflows remain deferred. |
| Fulfilment | Yes | Partial |  | `fulfilments`, store fulfilment modes | Add richer fulfilment statuses, assignment, and tracking only after storekeeper phase. |
| Merchant Customer Payments | Yes | Partial |  | `payments`, `payment_configurations`, `payment_provider_secrets`, `payment_webhook_events` | Keep deterministic boundary. Improve reconciliation UI and eventing. |
| SaaS Billing |  |  | Yes | none | Add plans, subscriptions, invoices, usage, wallet/credits in a separate billing phase. |
| Channels | Yes | Partial |  | `channels`, runtime WhatsApp handlers, `WhatsApp.tsx` | Refactor provider adapter boundary before adding external channels. |
| Conversations | Yes | Partial |  | `conversation_sessions`, `conversation_messages`, `Conversations.tsx` | Productize state model, filters, assignment, and agent workflows. |
| Human Handoff | Yes | Partial |  | `support_handoffs`, `support_handoff_notes`, runtime service | Add agent inbox, SLA, private/public notes, resolution taxonomy. |
| Complaints | Yes | Partial |  | `support_tickets`, runtime `create_complaint` action | Add dedicated complaints workspace and assignment workflow. |
| Bot Builder | Yes | Partial |  | `internal/bot`, `bots`, `bot_steps`, `commerce_workflow_configurations`, `Assistant.tsx` | Phase G adds merchant workflow guardrails and a lifecycle map over the existing versioned bot model; arbitrary automation execution is deferred. |
| Bot Runtime | Yes | Partial |  | `internal/runtime`, `conversation_sessions`, `processed_messages` | Keep one runtime. Add AI action bridge and policy checks. |
| AI Customer Service | Yes | Partial |  | `internal/ai`, `AIChat.tsx`, AI tests/evals | Preserve architecture. Add deterministic commerce action orchestration. |
| AI Providers | Yes | Partial |  | `internal/ai/provider`, config, `cmd/api/main.go` | Keep Ollama/Groq abstraction. Add provider config later if needed. |
| Merchant Knowledge | Yes | Partial |  | `merchant_knowledge_entries`, `bot_faqs`, `KnowledgeRetriever`, `Knowledge.tsx` | Build UI/API for structured knowledge entries; defer pgvector. |
| Audit Logs | Yes | Partial |  | `audit_logs`, `order_events`, `commerce_events`, `runtime_events` | Add standard audit semantics for AI actions, permissions, billing, channel secrets. |
| Background Jobs | Yes | Partial |  | `background_jobs`, `internal/jobs` | Add dashboards, metrics, dead-letter handling, tenant quotas. |
| Notifications | Yes | Partial |  | `commerce_notifications`, outbound jobs | Make notification templates/workflows configurable later. |
| Imports | Yes | Partial |  | `merchant_import_jobs`, `imports.go`, admin advanced import | Keep as merchant config/import utility; improve validation later. |
| Field Service | Yes | Partial |  | `internal/fieldservice`, `apps/field`, `service_*` tables | Treat as pilot/vertical module, not core commerce dependency. |
| Observability |  | Partial | Yes | request logger, runtime/order events, AI reports | Add metrics, traces, dashboards, latency and queue monitoring. |
| Load Testing |  |  | Yes | none | Add harness after runtime/channel architecture is stabilized. |
| pgvector / Documents |  |  | Yes | none | Defer until structured knowledge UI and lifecycle are stable. |
| WhatsApp Production |  | Partial |  | existing WhatsApp runtime/webhook code | Do not expand now. Refactor channel boundary first. |
| Instagram / Meta |  |  | Yes | none | Defer until channel abstraction and policy model are ready. |

## Entity Ownership Notes

Most tenant-owned tables already include `organization_id`. This is good and should remain the default for new domains.

Tables that need special attention in future phases:

- `users`: currently has nullable `organization_id` while memberships also exist. Decide the long-term source of organization context.
- `channels`: configuration and credentials should move toward secret references for sensitive values.
- `background_jobs`: idempotency is global by job type/key. That is workable if keys are globally generated, but tenant-aware uniqueness may be safer for future multi-tenant workloads.
- `bot_faqs` and `merchant_knowledge_entries`: both are currently supported. The product should move toward merchant knowledge as the canonical knowledge surface.

## Tenant Boundary Summary

Organization-scoped and structurally aligned:

- stores
- catalogue
- inventory
- customers
- carts
- orders
- payments
- fulfilments
- bots
- runtime sessions
- conversations
- support handoffs/tickets
- channels
- merchant knowledge
- audit logs

Needs future model:

- billing
- usage
- customer channel identities
- permission grants
- workflow event/action configuration
- channel credential references
