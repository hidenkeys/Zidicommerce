# ZidiCommerce operator guide

This guide is for the person who operates and develops ZidiCommerce. It describes **this** product, not generic SaaS.

Related docs:

- [Architecture](architecture.md)
- [Runtime](runtime.md)
- [Development](development.md)
- [Merchant deployment](merchant-deployment.md)
- [How-to walkthroughs](how-to.md)
- [Bot modules](modules/README.md)
- [Product experience, merchant workflows, and future channel UX](product-experience.md)
- [Channel platform foundation](channel-platform.md)
- [Meta WhatsApp connection](meta-whatsapp-connection.md)

The conversation runtime is production-critical. Do not change action semantics, provider boundary handling, published snapshots, payment verification, inventory reservation, or customer isolation unless you fully understand the consequences.

Admin navigation is permission-aware:

- **Home:** Overview, Setup
- **Commerce:** Orders, Catalogue, Inventory, Stores, Customers, Payments
- **Engagement:** Assistant, AI workspace, Conversations, Business knowledge
- **Operations:** Team, Audit log
- **Organization:** Business settings, Channels
- **Advanced:** Bot Builder, Payment tools, Delivery lookup, Import data

IDs and secrets belong in Advanced or API responses, not in the normal merchant UI.

Phase I narrows this navigation by job:

- **Administrator:** complete product setup, daily operations, people, assistant, knowledge, settings, and deferred channel preparation.
- **Store Manager:** assigned-store orders, catalogue, inventory, stores, conversations, and viewable organization context.
- **Storekeeper:** assigned-store Overview, Orders, Inventory, Stores, and Conversations only.
- **Support Agent:** Overview, Orders, Customers, Conversations, and Business knowledge.
- **Viewer:** permitted read-only pages with no mutation controls.

Backend permissions and store assignments remain authoritative. Navigation is not a security boundary.

## Merchant readiness workflow

Open **Home -> Setup** as an organization administrator. Readiness checks current tenant data in this order:

1. Complete the business profile.
2. Add an active store.
3. Add an active product and sellable option.
4. Set available inventory at a store.
5. Enable and test a payment configuration.
6. Publish active business knowledge, either structured knowledge or a compatible FAQ.
7. Test and publish the assistant.
8. Optionally invite team members and assign stores.
9. Optionally review the existing customer channel area.

The customer channel is deliberately non-blocking until external channel foundation work begins. Operational notification, outbound-message, and support queues are health signals, not merchant setup steps.

## Validated daily workflows

- **Orders:** use Needs action, open an order, verify payment and fulfilment context, perform only the displayed next action, and return to the queue.
- **Storekeepers:** confirm the store shown is assigned, process permitted orders, and review stock. The API scopes stores, inventory, and orders by assignment.
- **Support:** claim or accept an assigned handoff, review linked customer and order context, send customer-visible replies, keep internal notes separate, resolve, and explicitly release to AI only when appropriate.
- **Assistant:** review business-facing capabilities, verified commerce stages, payment verification, fulfilment, and human handoff; test before publishing.
- **Viewers:** verify operational state without being offered status changes, replies, stock edits, staff changes, or configuration forms.

Phase J.1 uses Meta Embedded Signup for merchant authorization and asset selection, with a separate assisted state for Zidi-managed setup. Secure credential rotation and manual references remain only for legacy/operator recovery. Billing still has no subscription state, usage balances, invoices, or synthetic metrics.

Phase M production pilots must use a separate API deployment and an approved test identity. Verify public health plus fail-closed GET/POST webhook behavior first. Obtain explicit approval before changing Meta webhook settings and again before sending a real message. Never promote setup state or health manually when provider evidence is missing.

Phase N applies WhatsApp policy before every provider request. In **Organization -> Channels**, use Messaging policy to review 24-hour windows and consent; use Templates to mirror exact Meta-approved records; use Recent policy blocks to diagnose sends stopped before Meta. A normal support reply is free-form only while the customer-service window is open. Outside it, select an approved template with every required variable. A local `approved` status is an operator assertion, not a Meta approval workflow.

The conversation inbox shows the contact's current WhatsApp consent and service-window state. When free-form messaging is blocked, the composer is disabled and directs the operator to use an approved template from Channels; the server-side policy guard remains authoritative for every send.

---

## How to read each section

Every module below answers:

1. What it is
2. Why it exists
3. What problem it solves
4. Who should use it
5. Where to find it in Admin
6. What information it requires
7. What happens when you configure it
8. What happens at runtime
9. How it interacts with other modules
10. How to test it
11. How to troubleshoot it
12. Common mistakes
13. What to change carefully
14. What is developer-only
15. What should never be manually changed in production

---

## Organizations

1. **What:** The tenant. Every commerce and bot record is scoped to an `organization_id`.
2. **Why:** ZidiCommerce is multi-tenant. Bing Chun is one organization, not hardcoded logic.
3. **Problem:** Keep one merchant’s customers, orders, inventory, and WhatsApp number isolated from another.
4. **Who:** Platform admins create orgs. Merchant owners edit the business profile.
5. **Where:** Settings → Business. Platform → Organizations (platform admin only).
6. **Requires:** Name, currency (usually `NGN`), timezone (usually `Africa/Lagos`), contact details.
7. **Configure:** `PATCH /v1/organizations/:id` updates the profile. `POST /v1/onboarding/organization` creates the org during first signup.
8. **Runtime:** The JWT’s `organization_id` is the tenant boundary. Webhooks resolve the org from the WhatsApp `phone_number_id`.
9. **Interacts with:** Everything. Stores, catalogue, bots, channels, payments all hang off the org.
10. **Test:** Sign in, open Overview, confirm the business name.
11. **Troubleshoot:** Wrong org usually means the wrong user/token. `GET /v1/auth/me` shows the org.
12. **Mistakes:** Editing another tenant in SQL. Mixing Postgres-jiuk (ZidiCommerce) with Postgres-4AYM (legacy Zidi).
13. **Careful:** Currency and timezone affect money display and hours.
14. **Developer-only:** Slug, metadata JSON, platform create-org API.
15. **Never:** Change `organization_id` on existing orders, customers, or channels.

## Team, roles, and store access

1. **What:** Memberships, invitations, roles, and store assignments.
2. **Why:** Not everyone should see every store or publish the bot.
3. **Problem:** A store staff member should only work their location.
4. **Who:** Merchant owner (`merchant_admin`).
5. **Where:** Team → People.
6. **Requires:** Email, name, role, optional store list.
7. **Configure:** Invite via `POST /v1/organizations/current/invitations`. Assign stores via `PUT /v1/organizations/current/members/:id/stores`.
8. **Runtime:** API policy middleware enforces roles. Store staff/manager queries join `store_user_assignments`.
9. **Interacts with:** Stores, orders, inventory, support.
10. **Test:** Invite a staff user, assign one store, confirm they only see that store’s orders.
11. **Troubleshoot:** Empty order lists for staff usually means no store assignment.
12. **Mistakes:** Pasting store UUIDs (the UI now uses store names). Giving `merchant_admin` to everyone.
13. **Careful:** Deactivating the last owner.
14. **Developer-only:** Invitation tokens, audit log event payloads.
15. **Never:** Hand-edit `organization_memberships` or assignments in production SQL unless recovering a lockout.

Roles: `platform_admin`, `merchant_admin` (Owner), `store_manager`, `store_staff`, `support_agent`, `viewer`.

## Stores

1. **What:** A sellable location with address, hours, and fulfilment modes.
2. **Why:** Catalogue is org-wide; stock and pickup/delivery are per store.
3. **Problem:** “Which branch is this order for?”
4. **Who:** Owner.
5. **Where:** Commerce → Stores.
6. **Requires:** Name, address, hours, fulfilment modes (`pickup`, `customer_rider`, `merchant_rider`).
7. **Configure:** `POST /v1/stores` and `PATCH /v1/stores/:id`. Activate/deactivate with dedicated endpoints.
8. **Runtime:** Order and Track Order modules ask the customer to pick a store from this list. Inventory is checked against the chosen store.
9. **Interacts with:** Inventory, orders, fulfilment, team access, bot `get_store` / list-stores actions.
10. **Test:** Create a store, confirm it appears in the assistant test when ordering.
11. **Troubleshoot:** Bot has no stores → status not `active`, or no stores in the org.
12. **Mistakes:** Duplicate store codes on import. Leaving all fulfilment modes off.
13. **Careful:** Deactivating a store that still has open orders.
14. **Developer-only:** Store `code`, lat/long, metadata.
15. **Never:** Reassign historical orders to another store to “fix” reports.

## Catalogue, categories, products, variants, images

1. **What:** What customers can buy. Category → Product → Variant (SKU, price) → Images.
2. **Why:** The bot does not invent products. It lists this catalogue.
3. **Problem:** Merchants need to add and price items without thinking about SKUs.
4. **Who:** Owner.
5. **Where:** Commerce → Catalogue.
6. **Requires:** Name, category, price in naira (stored as kobo/`price_minor`), optional image URL, availability.
7. **Configure:** Create product with a first variant. Edit product fields with `PATCH /v1/catalogue/products/:id`. Change price with `PATCH /v1/catalogue/variants/:id`. Add images with `POST /v1/catalogue/products/:id/images`.
8. **Runtime:** Order module lists active products/variants. Prices on new orders come from the variant, not from the customer’s message.
9. **Interacts with:** Inventory (per variant per store), orders (line items copy name/price).
10. **Test:** Add a product, set stock, test the assistant order flow.
11. **Troubleshoot:** Product missing in the bot → status `inactive`, no variant, or no stock at the selected store.
12. **Mistakes:** Entering kobo in the naira field. Creating a product with no variant.
13. **Careful:** Changing price does not rewrite old orders. Published bot snapshots do not freeze catalogue; they call live commerce.
14. **Developer-only:** Slugs, SKUs, metadata JSON.
15. **Never:** Delete variants that appear on historical order items. Prefer `inactive`.

## Inventory

1. **What:** On-hand, reserved, and reorder threshold per store + variant.
2. **Why:** Prevent overselling.
3. **Problem:** “What do I have, and what should I restock?”
4. **Who:** Owner, store manager.
5. **Where:** Commerce → Inventory.
6. **Requires:** Store, product/option, quantity, low-stock threshold.
7. **Configure:** `POST /v1/inventory` upserts. `PATCH /v1/inventory/:id` with `set_on_hand` or `adjustment`.
8. **Runtime:** Creating an order reserves quantity. Payment/cancel paths release or consume stock according to existing commerce logic. Available = on-hand − reserved.
9. **Interacts with:** Orders, Order module, Overview low-stock warnings.
10. **Test:** Set stock to 1, place a test order, confirm reserved/on-hand.
11. **Troubleshoot:** Bot says out of stock → reserved equals on-hand, or inventory row missing for that store.
12. **Mistakes:** Adjusting on-hand while ignoring reserved. Setting stock on the wrong store.
13. **Careful:** Manual SQL edits that skip reservation.
14. **Developer-only:** Inventory row IDs. There is no inventory history API yet.
15. **Never:** Decrement `reserved` by hand to “unstick” an order. Use order cancel/transition.

## Customers

1. **What:** People who messaged or ordered. Created from WhatsApp sender phone.
2. **Why:** Orders, tracking, and support need a stable customer record.
3. **Problem:** Identify a person across chats without exposing UUIDs.
4. **Who:** Owner, support.
5. **Where:** Commerce → Customers.
6. **Requires:** Name, phone, optional email/address (often filled by the bot).
7. **Configure:** Manual create exists; production customers usually come from WhatsApp.
8. **Runtime:** Webhook resolves or creates the customer from the sender number, scoped to the org.
9. **Interacts with:** Orders, conversations, complaints, carts.
10. **Test:** Message the live number, then find the customer by phone.
11. **Troubleshoot:** Duplicate customers → same person used different numbers, or a test sender.
12. **Mistakes:** Merging customers across organizations.
13. **Careful:** Phone format. WhatsApp senders are stored as the provider sends them.
14. **Developer-only:** Customer metadata JSON.
15. **Never:** Change `customer_id` on existing orders.

## Carts

1. **What:** A temporary basket for one customer at one store.
2. **Why:** The Order module adds items before `create_order`.
3. **Problem:** Multi-item checkout on WhatsApp.
4. **Who:** Runtime, not merchants. There is no merchant cart UI.
5. **Where:** Not in Admin. API: `/v1/carts`.
6. **Requires:** Customer, store, currency.
7. **Configure:** Nothing in Admin. Bot actions `get_or_create_cart` / `create_cart` / cart item actions.
8. **Runtime:** Carts stay org-scoped. Order creation reads the cart and inventory.
9. **Interacts with:** Order module, inventory, payments.
10. **Test:** Assistant test: add two items, confirm the order total.
11. **Troubleshoot:** Empty cart → session variables lost, or customer started a new session.
12. **Mistakes:** Treating carts as orders.
13. **Careful:** Clearing carts that a session still references.
14. **Developer-only:** All cart APIs.
15. **Never:** Manually insert cart items that skip inventory checks.

## Orders

1. **What:** The merchant’s source of truth after checkout.
2. **Why:** Kitchen/fulfilment and Track Order both read this.
3. **Problem:** See what was bought, paid, and what to do next.
4. **Who:** Owner, store staff, support.
5. **Where:** Commerce → Orders.
6. **Requires:** Created by the bot or `POST /v1/orders`. Merchants transition status.
7. **Configure:** `POST /v1/orders/:id/transition` with `{ status, idempotency_key }`. Allowed path includes paid → processing → ready → out_for_delivery → completed, plus cancel.
8. **Runtime:** Status is what Track Order shows. Payment webhooks move awaiting_payment → paid when Paystack confirms.
9. **Interacts with:** Payments, inventory, fulfilment, notifications, Track Order, complaints.
10. **Test:** Place a WhatsApp order, pay if required, move status in Admin, ask the bot to track it.
11. **Troubleshoot:** Stuck on awaiting_payment → Paystack webhook not reaching ZidiCommerce, or test vs live key mismatch.
12. **Mistakes:** Entering order UUIDs. Skipping statuses the kitchen still needs.
13. **Careful:** Cancel vs refund. Cancel is an order status. Refund is a payment status plus order `refunded`.
14. **Developer-only:** Idempotency keys, metadata, order events table.
15. **Never:** SQL-update order status. Always use the transition API so inventory stays consistent.

Human statuses: Awaiting payment, Paid, Processing (preparing), Ready, Out for delivery, Completed, Cancelled, Refunded.

## Payments and Paystack

1. **What:** Payment records plus a per-org Paystack configuration.
2. **Why:** Collect money without putting secrets in the bot graph.
3. **Problem:** Initialize, verify, and reconcile payments safely.
4. **Who:** Owner configures. Runtime charges. Advanced Payment tools for initialize/verify/reconcile.
5. **Where:** Settings → Payments. Advanced → Payment tools.
6. **Requires:** Enabled Paystack config. Secret from merchant store or `PAYSTACK_SECRET_KEY` env fallback.
7. **Configure:** `POST /v1/payment-configurations`. Test: `POST /v1/payment-configurations/paystack/test`. Secrets are write-only.
8. **Runtime:** Order module can require payment. `initialize_payment` returns an authorization URL. Paystack calls `POST /v1/payments/paystack/webhook`. Verification is authoritative and idempotent.
9. **Interacts with:** Orders, Order module, notifications.
10. **Test:** Settings → Payments → Run test. Then a real test-mode checkout on WhatsApp.
11. **Troubleshoot:** “Payments not connected” → config missing or disabled. Webhook 401 → wrong secret. Paid in Paystack, unpaid in Admin → webhook URL not ZidiCommerce, or shared **test** key still pointed at legacy Zidi.
12. **Mistakes:** Pasting live keys into test mode. Exposing `sk_` in chat, logs, or Git.
13. **Careful:** The Paystack **test** secret may be shared with legacy Zidi. Do not silently retarget that webhook.
14. **Developer-only:** `secret_source`, provider metadata, reconcile API.
15. **Never:** Mark payments paid in SQL. Never log raw webhook secrets or authorization signatures.

## Fulfilment

1. **What:** Delivery/pickup record for an order (`GET/PATCH /v1/fulfilment/:order_id`).
2. **Why:** Capture how the order leaves the store.
3. **Problem:** Pickup vs customer rider vs store delivery.
4. **Who:** Staff via order flow. Advanced delivery lookup still exists for the raw record.
5. **Where:** Order detail (method). Advanced → Delivery lookup.
6. **Requires:** Order, fulfilment type from the store’s enabled modes.
7. **Configure:** Store fulfilment modes and fees. Patch fulfilment details when needed.
8. **Runtime:** Order module asks fulfilment mode. Merchant rider may add a fee from the store config.
9. **Interacts with:** Stores, orders, Track Order copy.
10. **Test:** Place pickup and delivery orders.
11. **Troubleshoot:** Mode missing → disabled on the store.
12. **Mistakes:** Looking up fulfilment by UUID in the main UI (moved to Advanced).
13. **Careful:** Changing fulfilment after the rider has the order.
14. **Developer-only:** Fulfilment metadata.
15. **Never:** Delete fulfilment rows for completed orders.

## Channels

1. **What:** Tenant-scoped connection, provider account, identity, secure credential-reference, health, provider-event, and daily-metric records. WhatsApp Cloud API is the first production adapter.
2. **Why:** WhatsApp, Instagram, web chat, and later providers must enter the same conversation runtime through adapters.
3. **Problem:** Track connection ownership and operational health without hard-coding provider concepts into conversations or commerce.
4. **Who:** Organization administrators manage. Viewers may inspect. Store and support roles do not receive organization channel configuration.
5. **Where:** Organization -> Channels.
6. **Requires:** The Zidi deployment needs an approved/configured Meta app, Embedded Signup configuration, explicit Graph version, shared webhook configuration, encrypted channel secret key, and public HTTPS API URL. Merchants provide authorization through Meta rather than pasting an access token.
7. **Configure:** Prepare a WhatsApp record and choose **Connect with Meta**, or choose **Let Zidi help** for assisted setup. Mirror an approved Meta template for the first controlled outbound test when no customer-service window exists. After Meta completes authorization, send the controlled test, confirm the matching signed reply, complete validation, and run Check health. Manual identifiers/credentials are an operator or legacy recovery path only.
8. **Runtime:** The WhatsApp adapter verifies and normalizes provider activity into `channelplatform.InboundEvent`, which enters the existing runtime. Outbound responses remain provider-neutral until the WhatsApp policy guard checks consent, window, and template requirements before adapter translation.
9. **Interacts with:** Conversations, outbound jobs, assistant, customers, audit logs, and future provider adapters.
10. **Test:** Complete Meta's hosted flow, confirm the connected business/phone label, controlled outbound acceptance, matching signed inbound reply, completion, and health. Confirm tenant-scoped delivery, safe event, metric, and audit records.
11. **Troubleshoot:** An unavailable Connect with Meta button means one or more server-side Meta values or encrypted secret storage is missing. Authorization `failed` requires a new Meta launch after fixing the external prerequisite. `requires_attention` means credentials, signatures, or delivery need review.
12. **Mistakes:** Configuring a stale/guessed Graph version, omitting the WABA `messages` subscription, using the wrong app/configuration ID, assuming assisted setup bypasses merchant consent, or interpreting missing metrics as zero activity.
13. **Careful:** Ownership changes and archival are audited. Existing conversation history keeps its channel ID.
14. **Developer-only:** Idempotency keys, safe provider metadata, adapter payload translation, signature bypass, and legacy credential migration internals.
15. **Never:** Log or return credentials, enable signature bypass in production, change another tenant's channel ID, or manufacture connected/healthy state.

The runtime package still compiles its legacy WhatsApp implementation for compatibility, but production wiring uses `internal/channelplatform/whatsapp` and the generic runtime sender bridge. `channels.secret_config` is read only through explicit migration compatibility. See [Meta/WhatsApp Production Adapter](whatsapp-adapter.md).

For pilot rollback, pause/archive the connection, remove or replace the Meta webhook subscription, revoke the upstream access token, rotate any exposed secret, remove the test recipient, and only then retire the isolated deployment. Archiving in Zidi does not revoke a credential in Meta. Preserve redacted events and audit history; never export raw credential values.

## Bot Builder, versions, publishing

1. **What:** Organization-owned bots, draft versions, validation, immutable published snapshots.
2. **Why:** Change conversation design without rewriting Go, and pin live chats to a snapshot.
3. **Problem:** Merchants need “teach the assistant”; developers still need a graph.
4. **Who:** Owners use **Your assistant**. Developers use **Advanced → Bot Builder**.
5. **Where:** Assistant → Your assistant. Advanced → Bot Builder.
6. **Requires:** A bot, a draft version, enabled modules, valid graph before publish.
7. **Configure:** Self-service create `POST /v1/bots/self-service`. Draft `POST /v1/bots/:id/versions`. Publish `POST /v1/bot-versions/:id/validate` then `/publish`.
8. **Runtime:** New WhatsApp sessions load the **published** snapshot. Existing sessions stay on the version they started with.
9. **Interacts with:** All modules, knowledge, channels (`bot_id`), commerce actions.
10. **Test:** Assistant → Test assistant (simulator). Then a real WhatsApp conversation after publish.
11. **Troubleshoot:** Changes not live → forgot to publish, or you are still in an old WhatsApp session (`menu` / `restart`). Validate errors → missing start step or broken next keys.
12. **Mistakes:** Editing the published version (it is immutable). Expecting graph edits to affect in-flight chats.
13. **Careful:** Publishing during a busy period. Keep a known-good published version.
14. **Developer-only:** Steps, actions, conditions, start_step_key, snapshots table.
15. **Never:** Edit published snapshot rows in SQL. Never delete the version a live session still references.

## Modules

See [docs/modules/README.md](modules/README.md). Modules are capabilities on a **bot version**: Order, Track Order, FAQ, Complaint, Contact Support, Human Handoff, Welcome.

Enable/disable and reorder in **Your assistant**. That writes module metadata/sort_order, then you **publish**.

Runtime executes the published module graph and commerce actions. Disabling a module hides it from **new** published snapshots; old sessions keep the old snapshot.

## Variables, questions, conditions, actions

1. **What:** Builder primitives. Variables store collected data (name, store, order number). Questions prompt. Conditions branch. Actions call commerce (`get_or_create_cart`, `create_order`, `get_customer_orders`, `match_faq`, `handoff_to_agent`, …).
2. **Why:** The graph is data, not Go `if` statements per merchant.
3. **Problem:** Flexible bots without deploying code.
4. **Who:** Developers. Merchants should not start here.
5. **Where:** Advanced → Bot Builder tabs.
6. **Requires:** Valid keys, mappings like `customer_id: session.customer_id`.
7. **Configure:** POST/PATCH on `/v1/bot-versions/:id/*`.
8. **Runtime:** Template resolution is constrained. Actions go through the registry in `apps/api/internal/runtime/actions.go`.
9. **Interacts with:** Every module.
10. **Test:** Simulator + unit tests under `internal/runtime`.
11. **Troubleshoot:** Empty variables → mapping typo. Action error → commerce validation (missing store, empty cart).
12. **Mistakes:** Mapping UUIDs from customer-typed text. Changing action input names.
13. **Careful:** Any change to action semantics.
14. **Developer-only:** This entire layer.
15. **Never:** Change `internal/runtime/actions.go` to special-case one merchant.

## Knowledge / FAQ

1. **What:** Org-level Q&A used by the FAQ module (`/v1/bot-faqs`).
2. **Why:** Answer hours, location, allergens without a person.
3. **Problem:** Repeated questions.
4. **Who:** Owner.
5. **Where:** Assistant → Knowledge.
6. **Requires:** Question + answer. Optional keywords.
7. **Configure:** Create/patch FAQs. Matching is keyword/score based, not embeddings.
8. **Runtime:** `match_faq` scores active FAQs. Low scores return not found; the module should recover.
9. **Interacts with:** FAQ module, publish not required for FAQ **content** (FAQs are live org data). Module enable/disable still needs publish.
10. **Test:** Knowledge → Test, then Assistant test, then WhatsApp.
11. **Troubleshoot:** No match → rephrase the stored question toward customer language.
12. **Mistakes:** Writing developer notes in answers. Expecting LLM behavior.
13. **Careful:** Contradictory FAQs.
14. **Developer-only:** Match scores, keywords JSON.
15. **Never:** Point FAQ matching at another org’s rows.

## Conversations and support handoff

1. **What:** Sessions, messages, claimable handoffs, assignments, read state, internal notes, complaint tickets.
2. **Why:** Some customers need a human and AI must pause while a person owns the conversation.
3. **Problem:** A channel-neutral support inbox, not a session debugger.
4. **Who:** Support and owners.
5. **Where:** Assistant → Conversations.
6. **Requires:** A live or test session. Handoff created by `handoff_to_agent`, complaint flows, or `POST /v1/runtime/conversations/:id/handoff`.
7. **Configure:** Reply, assign, release, resolve, reopen, mark read/unread, and notes are available on `/v1/runtime/conversations/:id/*`. Handoff-specific operations remain on `/v1/runtime/support-handoffs/:id/*`.
8. **Runtime:** `human_requested` and `human_assigned` pause AI. Releasing with `resume_bot=true` returns the session to `ai_handling`.
9. **Interacts with:** Human Support module, complaints, customers, orders (if linked in session variables).
10. **Test:** Trigger handoff in a test session, claim it in Admin, send another customer message, verify AI stays silent, then release to AI.
11. **Troubleshoot:** Reply does nothing → check channel status and outbound dispatch logs.
12. **Mistakes:** Resolving without answering the customer.
13. **Careful:** Reopen vs starting a new channel conversation.
14. **Developer-only:** Session `current_step_key`, lock_version, system_context.
15. **Never:** Delete conversation sessions to “clean up” production.

Statuses in the inbox: Open, Waiting, Resolved.

## Complaints

1. **What:** Support tickets created by the Complaint module (`create_complaint`).
2. **Why:** Track problems beyond a single chat.
3. **Problem:** “This order was wrong.”
4. **Who:** Support.
5. **Where:** Customer profile and Conversations. API `GET /v1/runtime/support-tickets`.
6. **Requires:** Customer + message, optional order in variables.
7. **Configure:** Handled by the module; merchants enable Complaint on the assistant.
8. **Runtime:** Creates a ticket, often with handoff.
9. **Interacts with:** Customers, orders, handoff.
10. **Test:** Enable Complaint, run the flow, confirm a ticket and inbox Waiting state.
11. **Troubleshoot:** No ticket → module disabled or not published.
12. **Mistakes:** Treating complaints as FAQ.
13. **Careful:** PII in ticket bodies.
14. **Developer-only:** Ticket metadata.
15. **Never:** Drop tickets to hide incidents.

## Merchant import

1. **What:** Validation-first JSON upsert for stores, catalogue, inventory, channels.
2. **Why:** Bulk onboard a merchant (e.g. Bing Chun files under `merchant-config/`).
3. **Problem:** Don’t click 200 products by hand.
4. **Who:** Developers / operators.
5. **Where:** Advanced → Import. API `POST /v1/merchant-imports/configuration`.
6. **Requires:** Natural keys: store `code`, category `slug`, product `slug`, variant `sku`.
7. **Configure:** Same payload twice updates rather than duplicates.
8. **Runtime:** None directly. It writes commerce/channel rows the bot will use.
9. **Interacts with:** All commerce setup.
10. **Test:** Import in a non-prod org first.
11. **Troubleshoot:** Validation errors list missing keys. Channel secrets in the payload are stored but never returned.
12. **Mistakes:** Committing real secrets in JSON. Importing Bing Chun into the wrong org.
13. **Careful:** Channel upsert can overwrite WhatsApp config.
14. **Developer-only:** This whole feature.
15. **Never:** Run a channel import against production with placeholder secrets.

## Production readiness / setup

1. **What:** `GET /v1/bot-setup/status` checklist.
2. **Why:** Know if a merchant can go live.
3. **Problem:** Missing store, catalogue, WhatsApp, payments, or published bot.
4. **Who:** Owners see merchant steps on Home/Setup. Worker/outbound/support items are operational.
5. **Where:** Home → Setup. Overview banner.
6. **Requires:** Nothing to configure; it reads current data.
7. **Configure:** Completing other screens flips the checks.
8. **Runtime:** Not used by the bot. Worker/outbound checks look at notification and outbound queues.
9. **Interacts with:** All setup surfaces.
10. **Test:** Open Setup after each onboarding step.
11. **Troubleshoot:** Support check fails while the store is live → open handoffs, not a broken bot.
12. **Mistakes:** Treating worker/outbound as merchant homework.
13. **Careful:** “Ready” is necessary but not sufficient (Meta webhook must also point here).
14. **Developer-only:** database/worker/outbound items.
15. **Never:** Fake checklist rows in the database.

## Background jobs

1. **What:** In-process worker started by the API (`jobService.Start`). Queues commerce notifications and outbound WhatsApp sends.
2. **Why:** Retry delivery without a separate worker service.
3. **Problem:** Paystack paid / order events should notify even if WhatsApp blips.
4. **Who:** Developers.
5. **Where:** Not a merchant nav item. Setup flags stuck `queued` / `retry_pending` / `failed` outbound.
6. **Requires:** API process running.
7. **Configure:** None in Admin.
8. **Runtime:** Worker drains job rows. Do not run a second worker against the same DB.
9. **Interacts with:** Notifications, WhatsApp outbound.
10. **Test:** `go test ./...` job/runtime packages. Watch API logs in production.
11. **Troubleshoot:** Stuck outbound → token, Meta errors, or worker not running (API down).
12. **Mistakes:** Assuming a separate Railway worker service exists.
13. **Careful:** Killing the API kills the worker.
14. **Developer-only:** Job payloads.
15. **Never:** Duplicate job processing with a second binary.

## Audit logs

1. **What:** Administrative events (team, access, some bot/channel/payment config).
2. **Why:** Who changed what.
3. **Problem:** Support investigations.
4. **Who:** Owner / platform.
5. **Where:** Advanced → Audit logs.
6. **Requires:** Actions already happening in Admin/API.
7. **Configure:** None.
8. **Runtime:** Not on the customer path.
9. **Interacts with:** Team, bots, channels, payments.
10. **Test:** Invite a user, refresh audit logs.
11. **Troubleshoot:** Empty logs → feature never wrote an event, or wrong org.
12. **Mistakes:** Treating it as a full request log.
13. **Careful:** PII in event payloads.
14. **Developer-only:** Raw event JSON.
15. **Never:** Delete audit rows to hide a change.

## Runtime (summary)

The runtime executes **published snapshots only**, with persistent sessions, commerce actions, greeting/menu lifecycle, and WhatsApp adapters. Details: [runtime.md](runtime.md).

## Merchant commerce workflow

Merchant admins configure the operational guardrails on **Your assistant -> Commerce workflow**:

- turn the workflow, ordering, payment, or handoff on/off
- choose deterministic store selection behavior
- restrict the fulfilment modes offered to customers
- record the operational steps expected after verified payment

Save applies the authorization configuration immediately. Publish remains necessary only for conversational Bot Builder changes. `nearest` and `merchant_rule` currently fall back to customer choice; they are extension points, not automatic routing claims.

Orders show authoritative payment/fulfilment state, committed commerce events, the linked customer conversation, and the next allowed store operation. Store staff see only assigned-store data. See [Merchant Operations and Commerce Workflows](merchant-operations-workflows.md).

Do not rewrite `apps/api/internal/runtime` for UI work. Simulator tests must keep passing: `go test ./internal/runtime ./internal/bot`.

---

## Safety

If the live WhatsApp bot works, leave these alone unless a defect is proven:

- Action registry semantics
- Paystack webhook verification
- WhatsApp signature verification and `phone_number_id` routing
- Published snapshot immutability
- Order/inventory transactions
- Customer org isolation

## Phase O pilot operations

Phase N currently runs on the isolated WhatsApp pilot deployment, not the production API. Its controlled release passed health, migration, webhook verification/signature, service-window, consent, policy-event, and advanced-metrics checks without exposing credentials or sending a real customer message.

The local template used for the pilot remains pending because Meta approval was not independently verified. Do not mark it approved or use it for a live send until Meta shows the matching name and language as approved. Authenticated visual confirmation of the Admin policy pages also remains outstanding because the available pilot login was rejected.

Rollback is limited to the isolated pilot: redeploy its preceding Phase M deployment, verify health and fail-closed webhook behavior, then restore or remove the Meta callback as appropriate. The production API must remain untouched during this rollback.

## Phase P pilot acceptance

The isolated WhatsApp pilot has now passed external transport acceptance. The supplied pilot owner account authenticated, and Channels and Conversations were checked at desktop and mobile sizes. Meta already pointed at the pilot callback with `messages` subscribed, so no external Meta setting was changed.

Meta-approved template state was mirrored locally. A real signed inbound message opened the customer service window and entered the runtime. The first attempt revealed that the pilot bot had no steps or published snapshot and that failed runtime messages were not safely reclaimable. The runtime retry path was fixed and covered by regression testing, then one pilot-only terminal acceptance step was published through the immutable snapshot flow.

The next signed inbound message created the expected tenant-scoped contact, session, and messages. The pilot acceptance step automatically replied; Meta accepted that reply and later sent a delivered callback. No additional live message was sent, and no read callback was observed. The production API remained untouched.

This deployment is suitable for adapter acceptance only. Before merchant use, replace the terminal acceptance step with the complete merchant workflow, publish it, and run order, payment, support, and policy-window acceptance. If the pilot is retired, disable traffic in Meta before removing the deployment.

## Phase Q Bing Chun workflow pilot

The isolated pilot now contains a read-only-derived copy of Bing Chun merchant data: 7 stores, 6 categories, 28 products and variants, 28 images, and 196 store inventory records. Source keys map to stable target store codes, category slugs, product slugs, and variant SKUs, so rerunning the importer updates rows instead of duplicating them. The pilot's old sample stores and products are inactive and remain available only for rollback evidence.

Fourteen structured knowledge entries and nineteen compatibility FAQs were created. Seven confirmed operational entries are active. Seven uncertain delivery, payment, return, exchange, bulk, custom-order, and warranty positions remain draft and use the wording “This has not been confirmed yet. Please contact the store for confirmation.” Draft entries are not AI grounding sources.

The active Bing Chun bot has one immutable published snapshot with 43 steps and seven modules. Admin operators can verify it under **Advanced -> Bot Builder**, then verify stores, catalogue, inventory, business knowledge, conversations, channel health, and the masked merchant payment configuration from their normal workspaces. The isolated pilot's Paystack test configuration is active and provider-tested; this does not enable live-mode charging.

Before each live scenario, obtain explicit approval. Use one scenario at a time: product discovery, known price and stock, nonexistent product, order intent, test-mode payment, fulfilment, human handoff, and policy behavior. Do not create a live payment, override an opt-out, mark a local template approved, or send outside the service window without the required approved template.

The controlled scenario pass confirmed that explicit `cancel order`, `cancel request`, `stop order`, and `end order` commands cancel the current commerce request without triggering WhatsApp opt-out. Exact `CANCEL` remains a Meta consent opt-out and exact `START` restores consent. After checkout, cancellation must use the persisted order ID; before checkout it clears the persisted cart. Order creation is idempotent per cart, while payment and cancellation are idempotent per order. Never scope these keys only to the long-lived WhatsApp conversation because that can return a previous checkout on a later order.

The pilot passed Paystack test-mode payment acceptance through the merchant-scoped, write-only encrypted secret path. Two provider-hosted test checkouts produced signed webhooks and authoritative `paid` payment/order state. The first confirmation identified an `active`-only WhatsApp channel lookup that skipped healthy channel-platform connections; the deployed fix and regression test now cover usable connection states, and the second confirmation completed its notification job with a delivered WhatsApp outbound. Live-mode keys, charges, and promotion remain separate approval gates.

For rollback, redeploy the preceding isolated pilot image and select the previous published snapshot. Recheck health and fail-closed webhook behavior. Preserve aggregate provider evidence, do not export private contact data, and leave the original Bing Chun database untouched.
