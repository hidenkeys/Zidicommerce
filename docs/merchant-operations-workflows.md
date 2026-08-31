# Merchant Operations and Commerce Workflows

Phase G productizes merchant-controlled commerce workflows on top of the existing bot, runtime, commerce, payment, and conversation foundations. It does not introduce a second workflow engine. Published bot snapshots still determine conversational step order, while the organization workflow configuration controls which deterministic runtime actions are permitted.

## Ownership Boundaries

The customer lifecycle crosses four distinct boundaries:

1. **Conversational decisions:** welcome, intent discovery, clarification, menu choices, and handoff prompts. These are handled by the bot runtime and grounded AI.
2. **Deterministic commerce:** store, product, variant, inventory, cart, fulfilment, order, and payment initialization/verification. These execute through the runtime action registry and `commerce/core.Service`.
3. **External wait:** payment remains pending until the configured provider verifies it or a verified webhook is processed. Customer wording cannot mark a payment paid.
4. **Merchant operations:** prepare, ready, rider handoff, pickup, delivery, and completion use the order state machine and store-scoped permissions.

The LLM may interpret a request or explain a result. It does not set prices, inventory, payment status, order status, fulfilment status, or tenant identity.

## Configuration Model

`commerce_workflow_configurations` stores one tenant-scoped configuration per organization:

- active/inactive status
- assistant display name, greeting, and tone
- ordering, payment, and human-handoff toggles
- store-selection strategy
- enabled runtime actions
- supported fulfilment modes
- post-payment operational steps

The merchant admin API is:

- `GET /v1/bot-workflow`
- `PUT /v1/bot-workflow`

Updates use existing bot permissions and create an audit log. Runtime authorization loads this record using the immutable organization ID from the conversation session. A disabled action is rejected before its handler can call a commerce mutation.

The admin **Your assistant** page exposes these settings and a workflow map. The advanced Bot Builder remains available for editing versioned conversational steps and modules.

## Store Selection

Supported strategies are:

- `customer_choice`: list active stores in a deterministic order and let the customer choose.
- `single_store`: automatically choose the store only when exactly one active store exists.
- `first_available`: choose the first active store after deterministic name/address/ID ordering.
- `nearest`: reserved extension point; currently falls back to customer choice because no trusted customer-location distance policy is implemented.
- `merchant_rule`: reserved extension point; currently falls back to customer choice until a deterministic rule model exists.

The configured fulfilment modes are intersected with the selected store's enabled modes. A merchant configuration cannot enable a mode that the store itself does not support.

## Order and Conversation Continuity

`conversation_order_links` records the tenant, conversation session, order, customer, store, and source when a runtime flow creates an order. The unique organization/session/order key makes link creation idempotent.

Conversation summaries use the link to expose current order, payment, and fulfilment status. Order operations expose the linked conversation. This is a projection for operators; orders, payments, and fulfilments remain the authoritative records.

## Payment and Post-Payment Lifecycle

Payment confirmation still occurs only inside the Phase F provider verification boundary. After a payment is committed as paid, registered post-payment listeners receive the committed organization and order IDs.

The Phase G runtime listener:

- finds tenant-scoped conversation-order links
- updates session order/payment context
- records configured post-payment steps in system context
- records one idempotent runtime payment-confirmed event
- resumes a waiting, non-human-owned conversation to AI handling

It does **not** automatically mutate order or fulfilment status. The configured steps are durable intent and observability for a later deterministic automation phase. Store operators continue to execute the allowed next order transition.

## Store Operations

`GET /v1/orders/:id/operations` returns a tenant- and store-scoped projection containing:

- the authoritative order
- latest payment
- fulfilment
- durable commerce events
- linked conversation ID
- deterministic next actions

Examples include `Start preparing`, `Mark ready`, `Hand to rider`, `Mark collected`, and `Mark delivered`. Awaiting-payment orders expose a read-only wait state. The admin order page only offers cancellation before payment; paid cancellation remains blocked by the backend state machine.

Store managers and staff only see orders, inventory, and operations for assigned stores. Support roles can work conversations without receiving assistant, payment configuration, or unrelated commerce permissions.

## Handoff and Idempotency

Phase E handoff rules remain unchanged. Human-requested or human-assigned sessions pause AI. Payment projection does not override human ownership.

Idempotency remains layered:

- processed inbound messages prevent duplicate runtime execution
- runtime-derived action keys prevent duplicate order/payment writes
- commerce event keys prevent duplicate committed events
- conversation-order links prevent duplicate links
- post-payment runtime events are recorded once per session and order

## Observability

Operational evidence is available through:

- runtime action requested, authorized, denied, started, completed, and failed events
- runtime order-link and payment-confirmed events
- commerce order/payment/fulfilment events
- authorization and workflow-configuration audit logs
- admin order event timeline and conversation commerce context

Internal IDs, action policies, runtime variables, and debug metadata are not included in customer responses.

## Deferred Extensions

The following are deliberately not implemented in Phase G:

- executing arbitrary merchant-authored automation graphs
- automatic refund or paid-order cancellation
- nearest-store geospatial policy
- merchant-rule expression execution
- automatic rider assignment or third-party delivery dispatch
- external channel expansion
- billing/subscriptions

These should extend the existing configuration, action registry, commerce events, and job boundaries rather than creating another runtime or order state machine.
