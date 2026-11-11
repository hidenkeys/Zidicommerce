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
