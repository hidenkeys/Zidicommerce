# Bing Chun controlled beta readiness · 2026-11-11

| Gate | State | Evidence or next check |
|---|---|---|
| Immutable candidate and migrations | Pass | [Release manifest](release-manifest.md), API and Admin healthy, 32/32 staging and fresh-install migrations |
| Backup and restoration | Pass in staging | Disposable restore and row-count checks; Railway Pro restriction blocks a native daily schedule on this workspace. Recurring retention and failure alert remain open |
| Application rollback | Pass in staging | Pinned API and Admin digests restored, smoke checks passed, then exact candidate digests restored |
| WhatsApp configuration | Pass for configuration | Meta phone connected, subscribed, valid token, signed challenge path verified, allowlist enabled, CRM callback unchanged |
| Real-device WhatsApp, test payment and simulated pickup | Pass on staging candidate | ZC-20261111-897BB040 paid exactly once via the signed test webhook, completed simulated pickup, and received all four WhatsApp notifications on the checkout channel. |
| Paystack first delivery and replay | Prior staging pass | First provider-delivered webhook paid ZC-20261110-88697F27 without Check payment; six concurrent previously unseen signed deliveries caused one paid transition in the prior staging probe |
| Commerce safety | Pass in staging | Insufficient stock rejected, duplicate order ID reused, manual Paid and unpaid fulfilment rejected, cancellation restored stock |
| Staff revocation, job isolation, reference ambiguity, secret scan | Prior staging pass | Phase 3 evidence; coverage includes disabled/demoted staff tokens and notifications, outbound delivery and dispatch timeouts. Log scan was sampled |
| Groq grounding, freshness and human handoff | Partial | Live Groq, Admin test-chat, same-session knowledge update/archive, price and stock freshness passed. Injected fallback passed locally. Live staff claim, reply delivery and release passed. Deployed AI outage and bot response after release remain open. |
| Browser, roles and store scope | Pass for tested routes | Eight deployed desktop/mobile checks passed after the store-detail permission fix. Assignment changes, revocation, demotion and cross-tenant store reads verified. |
| WhatsApp messaging policy | Open | Invalid signatures and malformed JSON rejected. WhatsApp Web confirmed opt-out blocking and opt-in restoration with delivered messages in the open window. The deployed closed-window probe blocked free-form dispatch and incremented the policy metric. The tester timestamp was restored. Duplicate inbound handling and template validation passed in targeted tests. Staging has no approved template, and no read callback was observed. |
| Monitoring | Partial | API and Admin health checks and controlled Railway build-failure email verified. Verify recurring runtime, webhook/payment, queue, provider and backup alerting |
| Production decision | Hold | Conditional owner authorization recorded. Close every staging gate and record pilot acceptance before promotion. |
