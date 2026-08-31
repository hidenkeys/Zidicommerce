# Deterministic Commerce Action Engine

Phase F keeps commerce mutations inside Zidi services. AI, runtime flows, admin screens, and future channel adapters can request commerce work, but the database-backed commerce service remains the source of truth for prices, stock, payment status, order state, fulfilment state, and idempotency.

Phase G adds a tenant-scoped configuration and merchant operations projection around this boundary. It does not add external channels, provider expansion, billing, subscriptions, or a second workflow engine. See [Merchant Operations and Commerce Workflows](merchant-operations-workflows.md).

## Action Boundary

Runtime actions are classified in `internal/authz`:

- `read`: reads commerce or knowledge state.
- `safe_deterministic`: deterministic writes such as cart updates, notifications, complaints, and handoff creation.
- `commerce_mutation`: order and invoice actions that must run through commerce services.
- `high_risk_commerce`: payment verification and cancellation paths.

The runtime registry still rejects AI-originated write actions. AI can guide a user and use read-only tools, but write actions must pass through configured deterministic runtime/admin paths and the commerce service authorization layer.

The registry also checks the active organization's `commerce_workflow_configurations` record before executing an action. Merchant settings can disable ordering, payment, handoff, fulfilment modes, or individual registered actions. This gate never replaces commerce service permissions or state-machine validation.

## Order State Machine

Supported order states are:

- `awaiting_payment`
- `paid`
- `processing`
- `ready`
- `out_for_delivery`
- `completed`
- `cancelled`
- `refunded`

Allowed deterministic transitions are:

- `awaiting_payment -> paid`
- `awaiting_payment -> cancelled`
- `paid -> processing`
- `processing -> ready`
- `ready -> out_for_delivery`
- `ready -> completed`
- `out_for_delivery -> completed`

Automatic cancellation is only allowed before payment. Paid or fulfilled orders require manual review/refund handling instead of silent inventory restoration.

## Payment Boundary

Payment status is never accepted from customer wording. A payment becomes `paid` only when:

- the configured provider verifies the reference, or
- a verified webhook is processed and the provider verification matches the stored payment amount, currency, and reference.

Payment initialization is only allowed while the order is `awaiting_payment`.

## Inventory Safety

Order creation performs stock validation and guarded inventory decrement in the same database transaction. Cart prices or caller-supplied order item prices are never authoritative at checkout; order item snapshots use the current product variant price from the database.

Cancelling an `awaiting_payment` order restores inventory once through the existing idempotent order transition path. Paid-order cancellation is blocked pending a manual/refund workflow.

## Commerce Events

Phase F adds `commerce_events` as a durable event stream for downstream deterministic automation. Existing `order_events`, audit logs, notifications, and webhook records remain intact.

Current event types:

- `order.created`
- `order.status_changed`
- `order.cancelled`
- `order.completed`
- `payment.confirmed`
- `fulfilment.created`
- `fulfilment.status_changed`

Events are tenant-scoped by `organization_id`, linked to their resource IDs, and include source metadata:

- `human`
- `system`
- `runtime`
- `external_event`

Non-empty event idempotency keys are de-duplicated per organization and event type.

## Human Handoff

Human handoff remains the support boundary for ambiguous or risky situations. Phase F does not create a new handoff product; it preserves the Phase E conversation/handoff model and routes unsafe commerce decisions away from automatic mutation.

## Phase G Operations Projection

`GET /v1/orders/:id/operations` combines the authoritative order, latest payment, fulfilment, commerce events, linked conversation, and next allowed operator action. It is tenant- and store-scoped and does not infer payment state from customer messages or frontend status mappings.

## Deferred

- arbitrary workflow automation execution
- external channel integrations
- refunds and paid-order cancellation policy
- payment-provider expansion
- billing/subscriptions
- post-payment fulfilment automation beyond event emission
