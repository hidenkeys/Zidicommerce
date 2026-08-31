# Zidi Commerce Product Experience

This document defines the Phase H design system, Phase I merchant workflow experience, Phase J channel-platform foundation, the WhatsApp adapter, and the Phase J.1 Embedded Signup experience. Billing and additional provider integrations remain deferred.

## Design principles

1. Put work requiring attention before reporting and configuration.
2. Show only facts available from Zidi APIs; never manufacture dashboard metrics.
3. Use merchant language in the interface and keep implementation terminology in code and advanced tools.
4. Preserve context between customers, conversations, orders, stores, payments, and fulfilment.
5. Reveal advanced configuration progressively. The default path should support common daily operations.
6. Make status and ownership visible before presenting actions.
7. Keep destructive actions deliberate, reversible where possible, and protected by confirmation.
8. Keep the desktop workspace efficient while retaining complete mobile navigation and usable responsive layouts.

The visual system uses a neutral operational palette with amber as the primary action accent, restrained status colors, a compact spacing scale, 8px-or-smaller surface radii, minimal shadows, and consistent focus treatment. Shared components own buttons, icon buttons, status badges, tabs, filters, dialogs, drawers, loading states, empty states, alerts, and success toasts.

## Admin information architecture

The primary navigation is grouped by merchant task:

| Group | Current destinations |
| --- | --- |
| Home | Overview, Setup |
| Commerce | Orders, Catalogue, Inventory, Stores, Customers, Payments |
| Engagement | Assistant, Conversations, Business knowledge; AI test workspace for administrators only |
| Operations | Team, Audit log |
| Organization | Business settings, Channels |
| Advanced | Existing technical bot, payment, delivery, and import tools |

Navigation is permission-gated. Groups with no permitted destinations are removed. Store managers, storekeepers, and support agents therefore receive a smaller workspace than organization administrators. Advanced functionality remains available to authorized users without competing with common tasks.

## Role-based experience

- **Administrator** receives organization-wide setup, readiness, stores, people, assistant, knowledge, settings, channel preparation, and operational visibility.
- **Store Manager** runs assigned stores through orders, catalogue, inventory, stores, conversations, and a read-only view of organization-managed configuration.
- **Storekeeper** receives a deliberately small workspace for assigned stores, orders, fulfilment, inventory visibility, and relevant conversations.
- **Support Agent** works from Conversations, Customers, and read-only Orders and Knowledge. Store administration, setup, and AI diagnostics are hidden.
- **Viewer** receives read-only access to permitted business and configuration views. Mutation controls are not rendered.

Backend authorization remains authoritative. Navigation visibility is a usability layer, not a security boundary. Store scope continues to come from backend store assignments.

## Core workflows

### Getting started and readiness

Readiness is calculated from current tenant-scoped database records. Required steps are business profile, active store, sellable catalogue, available inventory, enabled payment configuration, active business knowledge, and a published assistant. Team setup and customer channel connection are visible but optional. A future channel can join this model without blocking local product testing before an external integration exists.

Each item explains its business purpose and links to the owning page. Operational system checks remain separate from merchant setup. Another organization's data can never satisfy a readiness item.

### New organization

The supported setup sequence is Business profile -> Store -> Catalogue -> Inventory -> Payments -> Business knowledge -> Assistant -> Team -> Readiness review. Merchants may move between steps, but the readiness screen remains the single source for what is required.

### Daily order operations

The customer-facing sequence is Product -> Cart -> Order -> Verified payment -> Processing -> Fulfilment -> Completion. The Orders workspace exposes only transitions valid for the current order state and the signed-in role. Read-only roles can inspect customer, items, store, payment, fulfilment, activity, and conversation context without receiving mutation controls.

### Human support

The supported sequence is AI conversation -> handoff request -> agent claim or assignment -> customer-visible reply and internal notes -> resolution -> optional explicit release to AI. Human ownership remains authoritative. The assistant stays paused until a permitted agent deliberately releases it.

### Store operations

Store managers and storekeepers see only assigned-store data from backend store scope. Their normal loop is Overview -> action queue -> Order -> verified payment and fulfilment context -> permitted next action -> Orders queue. Store assignment is a security boundary enforced by the API, not a client-side filter.

### Assistant configuration

Administrators review identity and flow, customer capabilities, business knowledge, commerce behavior, payment verification, fulfilment, human handoff, test conversation, and publication. Non-managing roles receive a read-only explanation of current behavior. Provider, orchestration, retrieval, database, and authorization internals remain outside the merchant workflow.

### Overview

The first section is an attention queue for unpaid orders, paid orders awaiting preparation, fulfilment, human handoffs, low stock, assistant publication, and configuration readiness. Today's order, confirmed revenue, fulfilment, and unread-message totals follow. Recent orders and setup status are secondary. All figures come from current APIs, and optional data failures do not erase otherwise permitted dashboard data.

### Orders

The order workspace presents searchable status queues and a stable detail sequence: customer, items, store, payment, fulfilment, activity, and linked conversation. Payment wording distinguishes provider verification from an unverified customer claim. Only actions returned by the commerce operations API are promoted as next actions. The API filters those actions by role, so a read-only user cannot receive a state-changing next action even if the order has one.

### Conversations

The support inbox uses three areas: conversation list, message timeline, and commerce context. AI ownership and human ownership are visually distinct. Internal notes are explicitly marked as team-only. Claim, transfer, resolve, reopen, and release-to-AI actions continue to use the existing conversation and handoff APIs.

### Assistant

The merchant view is split into identity and flow, capabilities, and preview/testing. The flow distinguishes conversational AI from verified commerce, provider waits, store operations, and human support. Runtime action keys and implementation details remain in the authorized Advanced tools.

### Knowledge

Business knowledge is separated into structured entries, reviewed documents, and legacy FAQ compatibility. Active, draft, and archived states remain explicit. Embedding states use merchant language such as "Ready for AI" and "Uses standard matching." Document review and approval rules are unchanged.

## Product state behavior

Major pages distinguish loading, empty, partial-data, success, permission, and destructive-confirmation states. Common transport and server failures are translated into merchant-facing guidance. Detailed diagnostics remain in server logs. A `404` is treated as potentially stale data and prompts a refresh; `409` explains that the record changed while the user was working.

Frontend permission visibility mirrors the backend role matrix for usability, while backend authorization remains authoritative. Direct navigation cannot bypass tenant, role, store, conversation-ownership, or commerce-state checks.

## Channel platform UX

The existing `/settings/whatsapp` route remains for URL compatibility and renders the provider-neutral **Channels** workspace. WhatsApp connections expose production setup and health controls. Instagram, web chat, and future providers remain visibly unavailable.

Each channel detail uses this structure:

1. **Overview**: connection owner, connection status, account status, phone or identity status, maintenance responsibility.
2. **Metrics**: generic message, delivery, conversation, response-time, and error totals that exist in the database.
3. **Health**: credential-reference state, adapter health, provider errors, and last check time.
4. **Configuration**: Connect with Meta, assisted setup, display name, ownership model, environment, webhook readiness, health check, explicit operator/legacy recovery, disconnect/reconnect, and safe archive action.

No future metric should render until its provider supplies verifiable data. Unsupported or unavailable data must be labelled unavailable, not represented as zero.

## Meta ownership models

Both models are stored on the channel connection and are visible independently from connection and health status. Phase J.1 uses Meta Embedded Signup for authorization and selection of existing or Meta-created business assets. Zidi validates and stores the resulting connection; it does not reproduce or bypass Meta's business verification and phone requirements.

### Merchant-managed Meta

The merchant owns the Meta Business account, WhatsApp Business Account, phone number, and related assets. **Connect with Meta** launches Meta's hosted flow. Zidi receives a short-lived code, validates the selected WABA/phone on the server, encrypts the resulting token, and reports connection health without showing technical IDs or secrets in the normal merchant view.

### Zidi-managed Meta

**Let Zidi help** creates an assisted request with `requested`, `in_progress`, `awaiting_merchant_action`, `blocked`, `connected`, and `completed` states. It does not mark a connection authorized until Meta authorization succeeds. The UI identifies when merchant action is required and does not imply that Zidi can bypass Meta or silently transfer legal asset ownership.

## Channel metrics foundation

The Phase J schema supports messages sent, delivered, read, and failed; conversation volume; AI/human handling; response time; and provider error totals. Values appear only after an adapter writes a real daily metric row. Template status, account quality, provider limits, and cost data remain deferred.

These belong under `Channels > Connection`. Phase K writes verified WhatsApp traffic, delivery, failure, and health values. Dashboard summaries may consume them only with a defined freshness policy. Synthetic metrics and channel billing remain prohibited. See [Channel Platform and WhatsApp Adapter](channel-platform.md).

## Deferred billing UX

Billing is not implemented. The future Organization information architecture may add Plan, Subscription, Usage, Monthly channel fee, Message usage, Credits or wallet, Invoices, Payment status, and Usage limits. These screens must use provider-backed data and must not confuse merchant billing with customer order payments. No placeholder balances, plans, limits, or invoices should be displayed before the billing domain exists.

## Accessibility and responsive behavior

Dialogs and drawers expose dialog semantics, names, Escape handling, focus restoration, and background scroll locking. Icon-only controls have accessible names and tooltips. Focus states are visible. Tables use horizontal containment on narrow screens; filters, forms, action groups, store/customer fact grids, and conversation panels collapse into single-column layouts. The sidebar becomes a labelled modal-style navigation panel on tablet and mobile.

## Phase Q merchant-pilot experience

The Bing Chun pilot demonstrates the merchant experience using real catalogue and store data without importing private customers or order history. Admin shows imported stores, all 28 active products, per-store inventory, active and draft knowledge, the published 43-step assistant, conversations, and healthy WhatsApp operations. Legacy sample products and stores remain visible only as inactive records where an advanced operator needs rollback context.

Customer answers remain database-grounded. Known product prices and stock come from commerce services; unknown products are rejected instead of matched on generic words such as “tea.” Requests to speak to a person resolve through the support FAQ and handoff path. Unconfirmed commercial policies remain drafts and cannot be presented as merchant commitments.

The Channels workspace contains long callback URLs, technical tables, and a horizontally scrollable section selector within the page boundary on mobile. Connected-state presentation recognizes both the legacy `active` status and channel-platform health statuses so the Admin does not report a healthy pilot as disconnected.

The live workflow now treats a WhatsApp conversation as a durable channel, not an order identity. Each cart produces its own order idempotency scope, and the resulting order owns payment and cancellation idempotency. A customer can explicitly cancel an active cart or an awaiting-payment order even after the published workflow reaches its terminal step; the runtime then returns to the main menu. Consent commands remain separate from commerce commands so an exact provider opt-out cannot be confused with cancelling a purchase.

The isolated pilot's payment experience now uses a merchant-scoped, encrypted Paystack test configuration. Provider-hosted checkout initialization, signed webhook verification, authoritative paid order state, and a delivered WhatsApp payment confirmation were observed in the controlled acceptance run. This validates the test-mode customer journey only; live-mode credentials, charges, and production promotion require separate approval.
