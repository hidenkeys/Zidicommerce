# Test evidence

| Evidence | Status | Boundary |
|---|---|---|
| [Phase C evaluation](../../apps/api/ai-provider-eval-report.md) | Historical automated live-provider evaluation reported 105/105 for Ollama and Groq | Applies to that suite and recorded configuration |
| [Journey Studio Phase R1](../phase-r1-implementation-report.md) | Historical automated and browser QA | Includes PostgreSQL and mock provider paths. Preview and controlled records are simulation evidence |
| [November staging acceptance record](../whatsapp-commerce-live-acceptance.md) | Historical health smoke and Go suite/race pass | Staging revision and date in source. No current health or provider attestation |
| Meta recipient | Manually verified in prior session | Authorized test recipient appeared selected in Meta. Signed inbound, outbound delivery and WABA override remain pending |
| Paystack and fulfilment | Not tested end to end with provider | Local deterministic tests cover mock payment safety. Collect test transaction and deployed order transition evidence |
| Production authorization, monitoring, restore and rollback | Pending | Require evidence for the approved artifact and target |

Phase 1 ran two targeted Go runtime tests on the current checkout: `TestJourneyLivePinnedIdempotentOrderAndPaymentSafety` and `TestJourneyFlowCheckoutDispatchDoesNotDependOnWording`. Both passed. `git diff --check` also passed.

## Phase 2 · 2026-11-10

| Check | Status | Evidence boundary |
|---|---|---|
| `go test -count=1 ./...` from apps/api | Pass on rerun | First run failed because the sandbox blocked loopback listeners in the Meta, TikTok and SMTP mock tests. Reran with loopback access and an empty application environment. All ordinary packages passed |
| `npm run check:admin` | Pass | Type check and Vite production build to temporary directory |
| `npm run check:field` | Pass | Type check and Vite production build to temporary directory |
| `git diff --check` and trailing-whitespace scan of tracking records | Pass | Local check |
| Race suite, dedicated PostgreSQL migration run and deployed/browser/provider checks | Not run | Required in their delivery phases. The opt-in live AI and database tests were intentionally left disabled |

## Phase 3 · 2026-11-10

| Check | Status | Evidence boundary |
|---|---|---|
| `go test -count=1 ./...` | Pass | Clean application environment, local loopback mock servers. Includes AI, runtime, channel adapters and commerce |
| `go test -race -count=1 ./...` | Pass | Local race suite. macOS linker emitted LC_DYSYMTAB warnings |
| `go build ./cmd/api` | Pass | Binary written to a temporary directory |
| `npm run check:admin`, `npm run build:admin`, `npm run check:field`, `npm run build:field` | Pass | Type checks and production asset builds |
| `git diff --check`, Go formatting and reverse review patch check | Pass | Uncommitted code review patch matches working tree |
| Meta configuration | Blocked | App-level CRM callback observed. Password reauthentication prompt opened for owner. No settings saved or messages sent |
| PostgreSQL concurrency and deployed WhatsApp/Paystack test payment | Not tested | Require disposable PostgreSQL and staging provider UAT |

## Controlled beta candidate · 2026-11-11

- Go full suite, race suite, API build, Admin and Field checks/builds, Git diff check and all 13 Playwright journeys passed. The candidate CI passed. The clean PostgreSQL 17 concurrency test ran against a real server. Staging preflight passed the staging checks; it did not attest production.
- The staging backup restored in a disposable PostgreSQL database. Checked 32 migration records, seven organizations, one store, one product, one inventory row, ten orders and two WhatsApp configurations at the snapshot. SHA-256: `6c1f513c49c3433641f926856a0c10d89c148202cee23d865343f20cac69db42`. Both disposable Railway databases and the remote temporary backup were removed.
- Staging Groq `openai/gpt-oss-20b`: direct request from the deployment and grounded Admin test-chat passed. Catalogue, price, stock, store and unknowns tested; prompt-disclosure probe revealed no internals. Handoff claim, AI pause, staff reply, release and resumption passed in test-chat. Session `89667238-981a-44ad-9f3b-56379844f51c` resolved; its handoff and messages remain as audit evidence.
- Synthetic commerce customer `689a93d0-5682-4a21-9bfa-3f5a90d618a5` remains. Orders `ZC-20261111-AD3213FC` and `ZC-20261111-AD3E880A` were cancelled and retained for audit. The latter check verified exact stock restoration and no payment or fulfilment.
- Railway alert probe deployment `89099d0f-8250-435b-b725-4a8d455d212c` failed as intended. The corresponding Railway email reached the designated operator inbox at 02:17:58 WAT. The disposable probe service was removed. This proves build alert delivery only.

## Conditional launch verification · 2026-11-11

- Confirmed deployed staging `CONTROLLED_BETA` and the single designated tester before the device check. The tester was asked to complete a fresh checkout using Paystack Test Mode and leave Check payment untouched. A fresh provider-paid order on the updated candidate has yet to be verified.
- A synthetic active returns entry returned 13 days. Updating it returned 29 days in the same chat. Archiving removed the answer. The knowledge entry was archived and its session resolved.
- A synthetic product in a separate tenant showed NGN 1,234 and four in stock. Updating the database through the catalogue and inventory APIs changed the answers in the same chat to NGN 5,678 and zero available. The synthetic product was deactivated and its session resolved.
- Staging returned store details to Support despite denying the store list. Commit `e59e94a` applied the store view permission to the details route. On the deployed API, Support received HTTP 403 for the list and both store details. The assigned store remained accessible to Manager and the unassigned store returned 404. Admin and Viewer retained authorized read access.
- Eight browser cases passed on the updated API and existing Admin image: Administrator, Manager, Support and Viewer, each at 1440×900 and 390×844. Checked authenticated Orders, role-specific Stores or Conversations UI, store visibility and create-control restrictions. Staging rejected old tokens with 401 after demotion and disablement. Changing a manager's store assignment immediately changed access with the existing token. Cross-tenant store requests returned 404 in both directions.
- The Go suite, commerce race test and API build passed for `e59e94a`. A focused runtime test injected an AI failure and verified the workflow fallback. Targeted WhatsApp consent, window and template policy tests passed locally. A live WhatsApp handoff, deployed provider fault and deployed WhatsApp policy exercise remain open.
- Synthetic staff were disabled and their assignments cleared. Synthetic stores and product were deactivated. Knowledge and conversation evidence was retained in archived or resolved state.
- The Railway Redeploy action rebuilt the API, yielding a different digest. The subsequent exact Rollback operation restored the pinned API and Admin digests. API, database, login and prior order read passed. Returning staging to the current API and Admin digests succeeded. The Admin image is carried forward from the preceding candidate because the fix changed only the API.
- After restoring the candidate images, staging reported API and DB healthy, Admin login HTTP 200, active Paystack configuration and the designated tester filter. Staging rejected an invalid WhatsApp signature with 403 and malformed JSON with 400.
- Verified no native backups or schedules on the staging Postgres volume. Railway rejected the daily schedule with `BAD_USER_INPUT`: “Manual backups and backup schedules are only available for Pro workspaces”. The previously restored logical backup is still the recovery evidence.
- Cleanup preserved audit history. The synthetic role tenant was inactivated and has zero active memberships. The isolated owner account and one unattached registration attempt were disabled.
- Staging queue snapshot showed zero queued, processing or retry-pending jobs, 11 historical outbound and six historical notification jobs in permanent failure, and three processed Paystack webhook rows. Permanent failures predate this verification and were not replayed. This snapshot does not verify alert delivery or the next customer's notifications.

## WhatsApp Web and Paystack acceptance · 2026-11-11

- Used the authenticated designated WhatsApp Web tester to select Staging QA Store, QA Products, Staging Test Mug (Standard ×1) and pickup. The cart total was NGN 2,500. Submitted the order and paid through the TEST checkout using Paystack's documented test card.
- Order `ZC-20261111-74BCA586`, payment reference `zc_53a08dc0-0419-41ec-bd71-b369002a9740`. A processed signed `charge.success` event produced exactly one `payment_webhook_verified` transition. No manual verification was invoked. The order moved through processing, ready and completed, and pickup reached completed.
- Payment confirmation and all three fulfilment notifications failed on the original artifact. They were routed to the older sandbox channel and blocked by that channel's closed service window. The order's conversation used the designated staging business channel. Preserve the failed notification rows and the completed order for audit. The routing fix uses the order's tenant-scoped checkout conversation channel. A fresh run on the fix is required for full acceptance.
- `cancel` opted the tester out, and the following order response was blocked by policy. `START` restored consent and the store menu was delivered inside the service window. The previous order remained paid and its records were preserved.
- The completed staging order retained one line item, one paid payment, completed fulfilment and stock of five units with zero reserved. API and database health passed after deployment of the routing fix. The full Go suite passed after field-service fixtures were aligned with the existing conversation-link schema. Chrome then required the Mac to be manually unlocked before further WhatsApp checks.
