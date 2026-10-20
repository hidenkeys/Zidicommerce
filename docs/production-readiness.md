# Production Readiness and Promotion Plan

Date: 2026-08-31

Scope: promotion of the current Zidi Commerce release candidate from the isolated pilot to the shared production API and Admin. This document is a runbook and approval record. It does not authorize a deployment, a Meta callback change, a live Paystack charge, or a customer WhatsApp message.

> **2026-09-01 staging note:** ongoing pre-production validation now uses the isolated `zidicommerce-staging` Railway project documented in [Staging deployment](staging-deployment.md). The former WhatsApp pilot is retired historical evidence. A healthy staging run does not change this document's production approval gates.

## Current verdict

**NO-GO for production promotion.**

The application candidate has passed the Phase Q pilot and local quality gates. The remaining blockers are release immutability, production migration/configuration attestation, unsafe startup fixture flags in the production service, backup and restore evidence, monitoring/incident ownership, and explicit integration approvals.

| Area | Current evidence | Status |
|---|---|---|
| Isolated pilot | Healthy API, fail-closed webhook security, Paystack test-mode verification, order/fulfilment safety, handoff, and delivery callbacks | Ready as pilot evidence |
| Production API | Healthy, but deployed from an older source revision | Blocked |
| Release candidate | Phase Q changes pass locally but are not yet an immutable reviewed commit/tag | Blocked |
| Migrations | Repository contains `000001` through `000027`; the deployed production artifact contains only `000001` through `000014` | Blocked pending DB attestation and migration plan |
| Production startup flags | Demo seed, pilot seed profile, and one-time WhatsApp number adoption are configured | Blocked until removed or explicitly approved |
| Production WhatsApp config | Signature bypass is off, but credential encryption and public callback origin are not configured | Blocked |
| Production payments | Paystack is selected and credentials are present; test/live mode has not been safely attested | Blocked pending mode decision |
| Production AI | No explicit hosted provider is configured, so the default is local Ollama | Blocked if AI is part of launch scope |
| Shared Admin | Healthy and targets the production API, but its deployed source revision is not attestable as this release candidate | Blocked pending approved rebuild/deploy |
| Backup, monitoring, incident response | No production restore rehearsal or external alerting evidence was found | Blocked |

## Evidence boundaries

- Phase Q proves behavior in the isolated pilot. It does not prove that production has the same migrations, secrets, merchant data, Meta configuration, or payment mode.
- A public `200` from `/health` proves API process and database connectivity only. It does not prove migration completeness, provider credentials, queue health, tenant data, or Admin compatibility.
- The API applies pending SQL migrations transactionally at startup and records them in `schema_migrations`. There are no automatic down migrations.
- Production database migration state has not been queried directly during this review. Treat `000015` through `000027` as required until the production `schema_migrations` table proves otherwise.
- Provider secret values, private account identifiers, customer data, and database URLs must never be copied into this runbook or a readiness report.

## Approval gates

Each gate needs a named approver and timestamp in the change record.

| Gate | Required approval/evidence |
|---|---|
| Release | Reviewed clean commit, immutable tag or digest, and exact API/Admin artifacts |
| Database | Fresh production backup, restore procedure, migration inventory, and migration window |
| API deployment | Production owner approval and rollback artifact selected |
| Admin deployment | Shared Admin owner approval and confirmed production API target |
| Paystack | Explicit `test` or `live` decision; live mode requires separate business and finance approval |
| Meta | Callback/subscription change approved by the Meta asset owner |
| WhatsApp test | Approved test recipient, message, time window, and operator |
| Operations | Staff roster, handoff owner, monitoring owner, and incident commander |
| Go-live | Final go/no-go sign-off after smoke tests; no partial implicit approval |

## Readiness checklist

Use `Confirmed`, `Blocked`, `Pending`, or `Not applicable`. Do not mark an item confirmed from inference alone.

### Application

| Check | Current status | Promotion evidence |
|---|---|---|
| Targeted and full Go tests pass | Confirmed locally | Save command output against the release commit |
| Race suite passes | Confirmed locally | Save result and any non-fatal toolchain warnings |
| API builds | Confirmed locally | Build exact release commit |
| Admin builds | Confirmed locally | Build exact release commit |
| All required migrations are in the API artifact | Confirmed for candidate | Compare artifact latest migration with repository latest |
| All required migrations are applied in production | Pending | Query `schema_migrations` before and after deploy |
| `/health` reports API and DB healthy | Confirmed for current pilot/production | Repeat after each deployment |
| No known application blocker remains | Confirmed for Phase Q candidate | Re-run Phase Q regression and review open incidents |
| No unsafe seed/adoption fixture is enabled | Blocked | Clear demo seed, pilot profile/password, and one-time adoption variables before deploy |
| Release is clean and immutable | Blocked | Commit reviewed changes and create a release tag/digest |

### Security

| Check | Current status | Promotion evidence |
|---|---|---|
| Tracked-source secret scan is clean | Confirmed; matches are placeholders only | Repeat on release commit and CI artifact |
| Secrets are absent from logs/reports | Confirmed by current scripts | Review redacted release and pilot reports |
| Channel credentials use stable encrypted storage | Confirmed in pilot; blocked in production | Configure stable `CHANNEL_SECRET_ENCRYPTION_KEY` before storing production credentials |
| Payment credentials use stable encrypted storage | Configured | Confirm key continuity and merchant-scoped masked response |
| WhatsApp POST signatures fail closed | Confirmed in pilot/tests | Invalid signature returns `403`; bypass remains false |
| Paystack signatures fail closed | Confirmed in tests/pilot | Invalid signature rejected; provider verification required before paid state |
| JWT/session secret and TTL are approved | Secret present; policy pending | Confirm entropy, rotation owner, issuer, TTL, and CORS origin |
| RBAC and store scoping are enforced | Confirmed by tests | Verify merchant admin, store staff, support, viewer navigation |
| Tenant isolation is enforced | Confirmed by tests | Run tenant-negative API checks against release |
| Audit logs are tenant-scoped | Confirmed in code/tests | Sample production-safe create/update/channel/handoff events after deployment |
| Production secrets have rotation/revocation owners | Pending | Record owner and emergency rotation steps for each provider |

### Payments

| Check | Current status | Promotion evidence |
|---|---|---|
| Paystack mode is explicitly classified | Blocked | Record `test` or `live` without exposing the key |
| Callback URL is the production HTTPS endpoint | Pending | Confirm in Paystack dashboard |
| Correct webhook secret is configured | Pending | Verify write-only presence and rotate if provenance is uncertain |
| Signed webhook validation works | Confirmed in test mode | Repeat one controlled test-mode transaction after promotion |
| Provider verification precedes `paid` | Confirmed | Preserve amount, currency, reference, and provider verification checks |
| Test-mode checkout is complete | Confirmed in isolated pilot | Retain Phase Q evidence |
| Live mode remains disabled until approved | Confirmed as policy | Separate finance/business approval required |
| Unpaid orders cannot enter fulfilment | Confirmed | Preserve expected HTTP 400 safety smoke check |
| Refund/cancellation policy is approved | Pending | Paid cancellation remains manual; define refund owner, SLA, and evidence |
| Reconciliation owner and procedure exist | Pending | Assign daily failed/pending webhook and payment review |

### WhatsApp and Meta

| Check | Current status | Promotion evidence |
|---|---|---|
| Production callback URL is approved | Pending | Confirm exact HTTPS callback without documenting private query values |
| Verify token is configured server-side | Pending | Write-only presence check; never print it |
| App secret and access token are valid | Pending | Provider health check with masked credential state |
| Phone identity and business account are correct | Pending | Match approved merchant assets in Meta and masked Admin state |
| Required webhook subscriptions are active | Pending | Confirm `messages` subscription in Meta |
| Signed inbound is observed | Confirmed only in pilot | Run one approved production controlled message |
| Invalid signature is rejected | Confirmed in pilot/tests | Run non-customer negative probe after API deployment |
| Outbound provider acceptance is observed | Confirmed only in pilot | Controlled recipient only |
| Delivered callback is observed | Confirmed only in pilot | Preserve tenant-scoped provider event evidence |
| Service-window and consent policy checks work | Confirmed in pilot/tests | Verify opt-in state and allowed send mode before outbound |
| Template mirror matches Meta | Pending | Do not mark a local template approved without Meta evidence |
| Outbound sending can be stopped quickly | Pending operational check | Confirm channel pause/archive and operator ownership |

### Operations

| Check | Current status | Promotion evidence |
|---|---|---|
| Support handoff claim/release/resolve works | Confirmed in pilot/tests | Verify staff permissions and queue ownership |
| Staff accounts and roles are current | Pending | Remove stale accounts; test least-privilege navigation |
| Storekeeper workflow is accepted | Confirmed in pilot, production pending | Verify assigned-store order visibility and transitions |
| Order workflow is accepted | Confirmed in pilot | Verify one controlled order on production configuration |
| Fulfilment workflow is accepted | Confirmed in pilot | Verify paid-only processing, ready, and completion |
| Monitoring and alert delivery exist | Blocked | Implement minimum launch monitors below |
| Database backup exists | Blocked | Create timestamped backup immediately before promotion |
| Restore has been rehearsed | Blocked | Restore to a disposable environment and record duration/result |
| Incident response owner and channel exist | Blocked | Name incident commander, communications path, and escalation contacts |
| Rollback artifacts are selected | Pending | Pin previous API/Admin artifacts before deployment |

### Data

| Check | Current status | Promotion evidence |
|---|---|---|
| Stores, products, variants, and inventory are current | Confirmed in pilot; production pending | Merchant signs off counts and prices |
| Merchant knowledge is current and active/draft states are correct | Confirmed in pilot; production pending | Merchant signs off active facts and unknown-policy behavior |
| No private test data is included in release artifacts | Pending | Inspect seed/import fixtures and generated reports before tagging |
| Production test fixtures are disabled | Blocked | Clear all demo/pilot startup seed configuration |
| Customer data ownership is documented | Pending | Record controller/processor roles, retention, access, and deletion path |
| Export/handover process is tested | Pending | Produce a tenant-scoped export with authorization and redaction review |
| Source merchant data is not mutated by pilot tooling | Confirmed | Preserve read-only-derived import boundary |

## Migration preflight

Run the following read-only query in the production database console. Do not paste the database URL into shell history or a report.

```sql
SELECT COUNT(*) AS applied_count,
       MIN(version) AS first_version,
       MAX(version) AS latest_version
FROM schema_migrations;

SELECT version, applied_at
FROM schema_migrations
ORDER BY version;
```

The approved release currently expects 27 files ending at `000027_phase_n_whatsapp_policy.sql`. Review `000015` through `000027` for lock/size impact, create a backup, and allow the API startup migrator to apply them once. A failed migration aborts startup. Never manually insert a `schema_migrations` row to bypass SQL execution.

## Promotion sequence

### A. Pre-promotion

1. Freeze pilot and release changes; stop accepting unrelated commits.
2. Commit and review the complete Phase Q/Phase R candidate; create an immutable tag or image digest.
3. Run every local gate in this document against that exact revision.
4. Run a tracked-source and artifact secret scan; remove generated customer/provider payloads.
5. Create a production database backup and rehearse restore into a disposable database.
6. Query and record production migration status; review `000015` through `000027` and expected runtime.
7. Remove production demo/pilot seed and one-time WhatsApp adoption variables.
8. Confirm secret presence, ownership, encryption-key continuity, domains, CORS, JWT, SMTP, AI provider, and callback origins without printing values.
9. Record whether Paystack remains test mode or is separately approved for live mode.
10. Confirm the production API and Admin targets and select previous artifacts for rollback.
11. Obtain API, Admin, database, Paystack, Meta, WhatsApp-test, and operations approvals.

### B. Deploy API

1. Put the change window and incident owner on record.
2. Deploy the immutable API artifact only; do not change Meta or Paystack callbacks yet.
3. Watch startup logs for migration success without logging secret values.
4. Query `schema_migrations`; require 27 expected versions with no gaps.
5. Verify `/health` returns `200`, `status=ok`, and `checks.database=ok`.
6. Verify authenticated login, tenant identity, RBAC, and masked provider configuration.
7. Send invalid WhatsApp and Paystack signatures to the new endpoints and require fail-closed rejection.
8. Stop if migrations, health, authentication, tenant scoping, or signature checks fail.

### C. Deploy Admin

1. Build the immutable Admin artifact with the approved production API target.
2. Deploy only after API smoke checks pass.
3. Verify login and logout, session expiry behavior, and role-specific navigation.
4. Verify Channels, Orders, Conversations, Support Handoffs, Audit Logs, and merchant-scoped settings.
5. Confirm secrets remain masked and write-only.

### D. Configure integrations

1. Configure the approved Meta callback and verify token; subscribe only the required webhook fields.
2. Confirm the intended merchant phone identity/business account before activating the channel.
3. Configure the approved Paystack callback and mode. Do not introduce a live key without separate approval.
4. Confirm tenant-scoped credential references and encrypted storage.
5. Mirror only provider-approved WhatsApp templates; preserve language/category/status.
6. Select one approved test recipient and test window.

### E. Smoke tests

Run in order and stop at the first unexplained failure:

1. Public API health and database connectivity.
2. Admin login, tenant identity, and role navigation.
3. Product, price, stock, store, and unknown-product enquiries against current database facts.
4. One controlled order; verify calculated totals and inventory reservation.
5. Payment branch according to the recorded decision: test mode by default, live only with separate approval.
6. Invalid Paystack signature rejection and signed provider verification.
7. One approved WhatsApp inbound; verify tenant routing, runtime, and stored conversation.
8. One policy-compliant outbound; verify provider acceptance and delivered callback.
9. Invalid WhatsApp signature rejection without erasing valid-signature readiness.
10. Human handoff claim, note, release/resolve, and optional bot resume.
11. Unpaid fulfilment rejection, then paid processing, ready, and completion.
12. Audit/event review with customer/provider identifiers redacted from the change record.

### F. Go/no-go

Go only when every blocker is closed, monitoring is receiving data, rollback artifacts are selected, and all named approvers sign the same release record. Any unexplained tenant, signature, payment, migration, or data-integrity failure is an automatic no-go. Provider delay or rate limiting must be classified separately, but it still blocks the affected launch capability until an operational fallback is approved.

## Rollback plan

1. **Stop the rollout:** freeze Admin changes and outbound tests. Put affected channels into `requires_attention`, maintenance, paused, or archived state through the normal operator path.
2. **Stop outbound sending:** disable/archive the channel and stop notification workers if needed. Do not delete message, order, payment, or audit records.
3. **Revert Meta:** restore the previously recorded callback/subscription or remove the new subscription. Confirm Meta no longer delivers to the failed endpoint before retiring it.
4. **Revoke/rotate credentials:** revoke upstream access tokens when compromise is possible, then rotate write-only Zidi references. Archiving a Zidi channel alone does not revoke Meta credentials.
5. **Roll back Admin:** redeploy the previous immutable Admin artifact and confirm its API contract matches the running API.
6. **Roll back API:** redeploy the previous immutable API artifact only if it is compatible with the migrated schema. Do not run ad hoc down SQL.
7. **Database restore:** use only for confirmed destructive corruption and with incident-command approval. Quiesce writes first, preserve the failed database for investigation, restore to a new instance when possible, validate counts/integrity, then switch deliberately.
8. **Reconcile external state:** compare Paystack transactions, Meta message IDs/statuses, orders, inventory, and notifications before reopening traffic.
9. **Verify rollback:** health, login, tenant scoping, invalid signatures, order visibility, and channel state must pass.

Once real customers interact, outbound messages, provider deliveries, payment-provider transactions, customer replies, issued order references, consumed inventory, and audit history cannot be safely “rolled back.” Compensate and reconcile those facts; do not erase them or restore over them casually. A database restore can lose legitimate post-backup writes and therefore requires a reconciliation plan.

## Minimum launch monitoring

These are initial thresholds. Tune them from observed pilot traffic, but do not launch without an owner and an alert destination.

| Signal | Initial trigger | Owner response |
|---|---|---|
| API health / DB connectivity | Two failed checks within 2 minutes | Page platform owner; stop promotion |
| Migration status | Latest version differs from release or any gap exists | Block traffic; inspect migration logs |
| Webhook 5xx | Any sustained 5xx or more than 1% over 5 minutes | Page integration owner; retain payload metadata safely |
| Invalid WhatsApp signatures | Sudden spike or more than 5 per minute | Security review; do not disable verification |
| Outbound send failures | More than 5% over 15 minutes or any auth failure | Pause sends; inspect credential/provider health |
| Delivery failure rate | More than 10% over 30 minutes | Review recipient quality, template/policy, and provider errors |
| Paystack webhook failures | Any signature/verification mismatch or repeated 5xx | Stop fulfilment for affected payments; reconcile in provider dashboard |
| Orders stuck | Paid order not entering processing within merchant SLA | Notify merchant operations; never force payment state |
| Handoff queue age | High priority over 5 minutes or normal over 15 minutes | Alert support lead and reassign |
| AI errors/fallbacks | Provider errors over 1% or fallback/unknown spike over 5% | Check provider and grounding health; retain deterministic fallback |
| Notification/job backlog | Oldest pending item over 5 minutes or repeated retries | Inspect worker health and poison items; do not duplicate sends |

Minimum launch views should include request/error rate, latency, DB health, migration version, webhook outcomes, signature rejection count, outbound/delivery outcomes, payment webhook state, order-state age, handoff age, AI provider/fallback state, and oldest pending job. Provider and customer identifiers must be masked.

## Read-only verification commands

```bash
cd apps/api
go test ./internal/channelplatform
go test ./internal/channelplatform/...
go test ./internal/runtime
go test ./internal/commerce/...
go test ./internal/ai
go test ./...
go test -race ./...
go build ./cmd/api

cd ../..
npm run build:admin
bash -n scripts/phase-q-pilot-verify.sh
bash -n scripts/whatsapp-live-smoke.sh
bash -n scripts/production-readiness-check.sh
git diff --check
```

Public, non-mutating preflight:

```bash
PILOT_API_BASE_URL=https://pilot-api.example.com \
PRODUCTION_API_BASE_URL=https://production-api.example.com \
ADMIN_BASE_URL=https://admin.example.com \
PRODUCTION_RELEASE_COMMIT=<deployed-commit> \
scripts/production-readiness-check.sh
```

Authenticated Phase Q verification remains pilot-only. Its default checks are non-destructive; every mutating probe requires its own explicit confirmation variable. Never point it at production by accident.

## Phase R validation record

The 2026-08-31 review produced the following redacted evidence:

- All requested targeted Go suites passed: channel platform, WhatsApp subpackages, runtime, commerce, and AI.
- `go test ./...`, `go test -race ./...`, and `go build ./cmd/api` passed.
- The race run emitted non-fatal macOS linker warnings about malformed `LC_DYSYMTAB` metadata; no race was reported.
- `npm run build:admin` passed.
- The Phase Q, WhatsApp live-smoke, and production-readiness scripts passed `bash -n`; `git diff --check` passed.
- Tracked secret-pattern matches were limited to documented development placeholders and test fixtures; no production credential was printed or added.
- Public pilot API, production API, and shared Admin endpoints returned healthy responses. API health included database connectivity.
- The read-only preflight correctly failed the promotion gate because the tree is not yet immutable and the production artifact migration inventory differs from the candidate. Production database migrations, integration mode, and backup/restore remain unverified.
- The isolated pilot verifier passed health, authentication, tenant seed, WhatsApp setup, durable valid-signature evidence, retained Paystack test evidence, one unique invalid-signature rejection, and provider/runtime evidence. Order, fulfilment, and AI mutations were not repeated.
- The shared production authentication API accepted the existing controlled admin fixture and returned a tenant-scoped session. The audit endpoint returned tenant-scoped records. Credentials, actors, targets, metadata, and identifiers were not exported.
- No API/Admin deployment, production variable change, Meta change, live payment, order mutation, or customer WhatsApp send occurred.

## Recommended next phase

Phase S should be a controlled production promotion and launch-observability phase. It should close the blockers in this document, produce immutable release artifacts, rehearse backup/restore, configure minimum alerts, run the approved deployment sequence, and capture a signed go/no-go record. Phase S must still treat live Paystack activation, Meta changes, and any customer message as separate explicit approvals.
