# Initial risk register

| ID | Priority | Classification | Finding and required gate |
|---|---|---|---|
| R01 | P0 | Configuration defect | Staging app secret and verify token were missing at last recorded check. Reattest credentials securely and prove test WABA routing without touching the CRM callback |
| R02 | P0 | Provider limitation | Meta password reauthentication blocked disclosure in the prior session. Complete owner reauthentication and provider tests before relying on WhatsApp |
| R03 | P0 | Application defect candidate | Journey Studio docs scope the test-key guard to the Flow pilot. Verify all guided and other reachable checkout paths reject live keys. Remediate any confirmed gap before beta |
| R04 | P0 | Product gap | Actual-device guided and AI acceptance and Paystack test checkout through fulfilment lack recorded evidence. Execute Phase 6 with authorized testers |
| R05 | P0 | Expected safety behavior | Native Flow remains gated pending provider acceptance. Validate disabled behavior and preserve a distinct sandbox test track |
| R06 | P0 | Documentation gap | Historical migration and deployment inventories describe different revisions. Reconcile repository, artifact and DB migration history before promotion |
| R07 | P0 | Infrastructure defect candidate | Backup restore, alert delivery and rollback evidence is outstanding. Rehearse on disposable infrastructure and assign owners |
| R08 | P0 | Product gap | Deployed tenant isolation, authorization and retry behavior need release-specific acceptance evidence across admin, jobs and channel entry points |
| R09 | P1 | Documentation gap | Merchant, audience, domain and operating owners require confirmation. Record approvals and named UAT sign-off before onboarding |

Phase 2 confirms R03 as an application defect for the beta scope. The [audit backlog](phase-2-audit.md#prioritized-remediation-backlog) adds payment authorization, remote checkout idempotency, incomplete verification, webhook retry and explicit WhatsApp-only enforcement. R06–R08 remain open pending release-specific evidence.

Phase 3 implements candidate remediation for A01–A07 and adds customer-ownership and generic paid-transition safeguards. A09, provider acceptance and staging operational controls remain open. An uncertain checkout requires operator reconciliation, documented in the [Phase 3 notes](phase-3-stabilization.md).
