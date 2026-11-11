# Zidi Commerce controlled beta

**Phase 1 scope proposal · 2026-11-10**

Zidi Commerce gives African merchants one place to run sales and support across stores. The initial release brings together WhatsApp, configurable customer journeys, answers grounded in merchant data, verified order and payment state, and a staff inbox for exceptions.

Start with one merchant that already has a catalogue, maintained stock and a named fulfilment lead. Bing Chun is the proposed pilot because it is familiar from earlier testing. Confirm the merchant and eligible stores before onboarding. Start with designated testers, a dedicated beta tenant and test orders. Use Paystack test mode and rehearse fulfilment without dispatching goods. The end-of-week target is the strongest candidate that clears the gates below. A blocked launch remains a staging candidate.

## Customer journeys

| Journey | Beta scope | Required evidence |
|---|---|---|
| Guided WhatsApp commerce | Browse categories and variants, select quantities, see current price and stock, review the cart, select pickup or supported delivery, confirm order and receive its generated Paystack test checkout URL | Approved WhatsApp device and staging admin show the same tenant, items, totals and order. Stale prices or insufficient stock trigger revalidation. Retries create one order |
| Payment and fulfilment | Complete provider test checkout, verify payment, process the order, mark ready, dispatch or hand over for pickup, and complete | Valid signed event and independent provider verification match reference, amount and currency. False claims and invalid callbacks leave the order unpaid. Replays cause no duplicate effects. Cancellation releases stock once. Invalid transitions are rejected |
| Grounded AI | Ask about current products, stores, policies and order status where the customer is authorized. Route commerce writes through the validated journey or staff workflow | Tenant isolation, freshness, unknown answer, model failure and unauthorized order lookup checked through the deployed path |
| Support and complaints | Escalate to staff, claim, reply, release and resume. Use an existing complaint journey where it passes acceptance | Staff actions appear in the tenant inbox and audit trail. Automation respects handoff ownership |
| Native WhatsApp Flow | Separate sandbox acceptance track, disabled for launch pending Meta and device acceptance | Provider validation, encrypted data exchange, real-device navigation, replay protection and provider test payment through fulfilment |
| Other channels and live commerce | Disabled or excluded | Explicit environment and routing checks confirm the release boundary |

## Admin capability matrix

| Area | Beta capability | Required evidence |
|---|---|---|
| Organization and staff | Controlled bootstrap, login, invitations, roles and scoped staff access | Permitted actions succeed. Unauthorized roles and cross-tenant reads and writes are denied |
| Stores and catalogue | Create stores, products and variants, change prices, deactivate items and adjust inventory | Changes reach customer-facing reads. Deactivation and stock changes are enforced at checkout |
| Customers and inbox | Find customers, review conversations, manage handoff and answer support | Tenant-scoped records and audited ownership changes |
| Knowledge | Manage structured entries and review document chunks for approval | Active knowledge is fresh. Draft and archived knowledge stay out of grounding. Unapproved chunks stay out of grounding |
| Journey Studio | Configure guided and grounded experiences, preview, publish, pause and roll back | Published versions and session behavior match expected routing. Preview evidence is recorded separately from actual commerce |
| Orders, payments and fulfilment | Inspect test payments and move eligible orders through fulfilment | Server authorization and state transition checks, verified provider payment and traceable audit records |
| Operations | View audit history and check health | Monitors, alert delivery, restoration and rollback evidence available to named operators |

## Priorities and release gates

- **P0.** Verify tenant isolation in API, jobs and integration boundaries, role enforcement, secret handling, signed webhooks, checkout integrity, safe AI fallback and release-wide test-only payment enforcement. Disable unsupported channels, unverified Flows and unsafe seed or bypass settings. Require approved immutable artifacts, migration verification, backup and restore rehearsal, tested rollback, signed UAT and explicit production approval.
- **P1.** Resolve pilot usability and operating gaps found in UAT, including mobile admin, handoff, complaint escalation where supported, and clear payment and fulfilment state. Triage any issue that jeopardizes the promised journey as P0.
- **Deferred.** Live payments, unaccepted Flows, Instagram, Messenger, TikTok, Telegram, storefront and marketplace, mobile app, abandoned cart recovery, advanced campaigns, coupons and loyalty, affiliates, price negotiation, voice commerce, expense and credit-sales tracking, logistics integrations and automated billing.

## Delivery plan

| Phase | Exit gate |
|---|---|
| 1. Product scope | Review scope, evidence boundaries, owners and priorities. Obtain approval before Phase 2 |
| 2. Repository audit | Map implementation, migrations, environment variables and tests. Rank verified security and readiness gaps |
| 3. Stabilization | Fix confirmed release blockers, run required Go and frontend checks, present release diff for approval before commit, push, merge or tag |
| 4. Infrastructure | Attest isolated services, backups, domains, secrets and health. Choose an AI configuration using latency, cost, reliability and resource evidence |
| 5. Staging | Deploy approved immutable candidate, migrate a clean database, bootstrap pilot tenant and record deployment and rollback identifiers |
| 6. Acceptance | Complete deployed UI, backend, device and provider testing, including negative and concurrency cases, with traceable evidence |
| 7. Operations | Rehearse restore and rollback, verify alert delivery, document retention and incident response, obtain signed UAT |
| 8. Launch | Obtain explicit written approval, verify release and backup, onboard only approved pilots, smoke-test and monitor for seven days |
