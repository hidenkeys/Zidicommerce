# Current status

**2026-11-10 · Phase 1 CONDITIONAL PASS · Production promotion blocked**

Inspected the branch and worktree, recent commits, admin pages and backend package boundaries, migration directory tail, package scripts, Journey Studio, Phase R/R1, Phase C evaluation and staging acceptance records. The current branch is `codex/whatsapp-customer-experience` at `e0019e6` at inspection. The older production-readiness record predates later migrations. Local migrations reach `000032`. Production and staging schema state require fresh attestation.

The November 4 staging record identifies API revision `29fbd8e`. Phase R1 describes implemented guided chat, previews, grounded routing and simulated checkout. Current code includes the subsequent explicit checkout dispatch fix `742b2a5`. The designated Meta test recipient was verified after the repository note was written, according to the prior session. Meta password reauthentication blocked app-secret disclosure. Current credential validity, test WABA routing, actual message delivery and Paystack test checkout through fulfilment remain unverified. The prior Phase C provider report is historical evidence.

Existing changes in `docs/operator-guide.md`, `docs/whatsapp-adapter.md`, `docs/production-readiness.md` and `scripts/production-readiness-check.sh` were preserved. This phase adds only the six tracking records.

## Phase 2 update · 2026-11-10

Bing Chun, designated testers, Paystack test payments and simulated fulfilment are confirmed. The [repository audit](phase-2-audit.md) documents confirmed payment control gaps and the remaining validation gates. **FAIL for release promotion.** Phase 3 requires approval to implement the stabilization backlog. Production and the main website were untouched. Domain, acceptance lead and operations owner remain open for later phases.

## Phase 3 update · 2026-11-10

[Stabilization candidate](phase-3-stabilization.md) prepared for review. Code is uncommitted. The staging Bing Chun checkout and simulated pickup passed on the deployed staging release. The Phase 3 candidate still needs its own deployment and acceptance run. Production promotion remains blocked.

### Manual sandbox onboarding

Embedded Signup and App Review are deferred. The fresh Meta token worked on the designated test WhatsApp Business Account. The callback override was applied to the sandbox account only, leaving the CRM app-level callback unchanged. Staging recorded the webhook challenge as verified at 05:55:36 UTC on November 10. Meta accepted the sandbox test send at 05:55:44 UTC and reported delivery for the same message at 05:55:47 UTC. Staging received a signed customer reply at 05:57:26 UTC, but the conversation runtime failed with `NO_PUBLISHED_VERSION: Published bot snapshot not found`. The sandbox tenant has no Journey Studio journeys and its active local test bot points to a version whose runtime snapshot could not be loaded. Onboarding remains incomplete. Published the Bing Chun pilot journey and bound its guided order trigger to the staging WhatsApp sandbox. The pilot uses the staging QA store and catalogue, and the configured Paystack credential is in test mode. Staging processed the designated tester’s order message and completed onboarding. The manual setup had left outbound text and interactive messages at setup required. Those two capability records were repaired for the verified staging sandbox only, and Meta confirmed delivery of the store prompt after retry. Paystack verified NGN 7,500 for staging order ZC-20261110-8F479987. The operator moved the order through processing, ready, and completed using the staging APIs. Both the order and pickup records were confirmed completed. A matching inbound WhatsApp message had moved the onboarding label back to inbound test received after it was connected, so the operator completed onboarding again after checkout. The Phase 3 review patch is separate from the local Embedded Signup changes.
