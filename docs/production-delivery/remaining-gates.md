# Bing Chun controlled beta readiness · 2026-11-11

| Gate | State | Evidence or next check |
|---|---|---|
| Immutable candidate and migrations | Pass | [Release manifest](release-manifest.md), API and Admin healthy, 32/32 staging and fresh-install migrations |
| Backup and restoration | Pass in staging | Disposable restore and row-count checks; automated retention and backup alert remain unverified |
| Application rollback | Open | Previous artifacts recorded; rehearse rollback and return on an isolated target or agreed staging window |
| WhatsApp configuration | Pass for configuration | Meta phone connected, subscribed, valid token, signed challenge path verified, allowlist enabled, CRM callback unchanged |
| Real-device WhatsApp, test payment and simulated pickup | Prior staging pass | Signed inbound, outbound delivery and simulated pickup proven in Phase 3; fresh complete device run on the final candidate remains open |
| Paystack first delivery and replay | Prior staging pass | First provider-delivered webhook paid ZC-20261110-88697F27 without Check payment; six concurrent previously unseen signed deliveries caused one paid transition in the prior staging probe |
| Commerce safety | Pass in staging | Insufficient stock rejected, duplicate order ID reused, manual Paid and unpaid fulfilment rejected, cancellation restored stock |
| Staff revocation, job isolation, reference ambiguity, secret scan | Prior staging pass | Phase 3 evidence; coverage includes disabled/demoted staff tokens and notifications, outbound delivery and dispatch timeouts. Log scan was sampled |
| Groq grounding and human handoff | Partial | Live Groq and deployed Admin test-chat passed; live provider failure fallback, policy/FAQ freshness and WhatsApp handoff still need acceptance |
| Browser and role acceptance | Partial | All 13 local Playwright journeys passed, including mobile and Viewer restrictions. Staging desktop admin smoke passed; Administrator, Manager, Support and Viewer desktop/mobile matrix not signed off |
| WhatsApp messaging policy | Open | Verify service-window, opt-out, approved template path, duplicate/malformed events and read receipts on deployed staging |
| Monitoring | Partial | API and Admin health checks and controlled Railway build-failure email verified. Verify recurring runtime, webhook/payment, queue, provider and backup alerting |
| Production decision | Hold | Close open staging gates and obtain signed UAT and explicit promotion approval |
