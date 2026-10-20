# Phase R1 Implementation Report

Date: 2026-10-02. Repository: `/Users/oluwateniolasobande/Zidi/Zidicommerce`.

## 1. Historical Zidi Findings

The read-only `zidi-client/src/store/createCampaignStore.ts` and campaign pages supplied useful concepts: welcome text, ordered text/multiple-choice questions, separate options and saved responses. The older flow required a question count upfront. `Zidi/whatsappbot/bot.go` advanced a linear question counter and held active sessions in a process-local map keyed by sender. Those session and linear-routing assumptions were not copied. Neither historical repository was edited.

## 2. Current Zidi Commerce Findings

Migrations `000004_bot_builder.sql` and `000009_phase8_self_service_bot_builder.sql` already provide the graph, version, immutable snapshot and self-service foundations. Current runtime sessions, processed messages, commerce actions, merchant workflow configuration, handoff services, channel capabilities and encrypted provider secret storage are reusable. The older Admin builder exposes technical configuration fields and JSON-oriented previews; Journey Studio adds a merchant view without removing that advanced surface or rewriting AI.

## 3. Architecture Implemented

One canonical existing bot graph, with per-version journey settings and per-step presentation/safety metadata. Guided chat, Flow compilation and grounded AI routing share it. Mutable activation is separate from immutable publications. Commerce data is not duplicated into templates. Preview snapshots are pinned and database-persisted.

## 4. Database Migrations

Added `000032_phase_r1_journey_studio.sql`: bindings, provider deployments, preview sessions, hashed-token Flow sessions, exchange receipts and execution leases. Tenant/version/connection/conversation composite foreign keys protect new linked records. The polymorphic receipt session ID is protected by scoped service lookup and uniqueness, not a misleading single-table FK. All 32 migrations applied to an isolated local pgvector-enabled PostgreSQL 16 QA database. No remote database was migrated.

## 5. Backend Changes

Journey CRUD, templates/library, validation, compile, immutable publication, clone, rollback, mode switch, pause and archive APIs. Persisted actor-owned previews and explicitly acknowledged test-record sessions. Dynamic tenant-scoped commerce choices, live checkout pricing/stock, action receipts, lease serialization, provider lifecycle and secure encrypted exchange. Existing services remain the transaction/payment/handoff boundaries. Journey edits are atomic and generic legacy graph-edit endpoints reject journey versions; publication rechecks the graph under the version lock.

## 6. Admin Journey Studio Changes

New `/journeys` navigation and list. Desktop three-column builder; mobile tabs. Safe step editing/reordering, option/branch configuration, protected-step controls, local undo/redo, unsaved-change detection, draft saves, validation issues, publication confirmation, history/rollback/pause and explicit provider controls. Normal preview and test-record mode have different labels and confirmation requirements.

## 7. Guided-Chat Renderer Status

Implemented through the existing runtime/channel adapter. Database-pinned sessions, opaque selections, numeric current-list fallback, pagination, typed validation, retries, supported Back/Menu/Cancel/Restart, TTL, idempotent inbound events, transactional commerce and provider-verified payment. Long labels fall back to usable numbered text. Location/media requests remain guided-chat features.

## 8. WhatsApp Flow Compiler Status

Deterministic JSON 7.3/data API 3.0 with stable screens, supported components, terminal screen, forward routing, server-side actions/validation and explicit unsupported-construct errors/warnings. Self refresh and return navigation are reported, not hidden. Local tests do not certify Meta acceptance. Repeated-cart, native Back and payment-refresh routes are external verification gates.

## 9. Interactive Preview Status

Chat and compiled-Flow preview share the same persisted interpreter session. Read-only previews produce no commerce/customer/support/notification writes. Empty catalogue, unavailable stock, payment failure, handoff and unsupported-channel scenarios are supported. Controlled test records require explicit acknowledgement, ownership, publication and a local test channel; notifications and external calls are suppressed.

## 10. AI Routing Integration

Connection/intent bindings and a versioned default welcome menu route into published journeys, grounded AI or human support. AI presentation calls the existing grounded callback, with deterministic fallback. No AI source files, prompts, retrieval or provider implementations changed. Existing Phase C deterministic tests remain intact; live model comparisons were not rerun.

## 11. Security and Tenant-Isolation Results

Scoped CRUD/preview/provider operations; role restrictions; actor-owned previews/tests; composite FKs; encrypted keys; strict raw-body HMAC and RSA-OAEP/AES-GCM; expiry/version/screen/turn validation; replay receipts; rate/payload limits; markup refusal; sanitized errors and audit classifications. Arbitrary encrypted request action text is classified as `invalid_action`, never copied into audits. Admin message APIs redact Flow tokens without mutating durable delivery data. Both lifecycle and public exchange restrict the pilot to exact sandbox test identities. Paystack live keys are rejected for Flow pilot order/payment execution.

## 12. Automated Test Results

Final results after all fixes:

| Verification | Result |
| --- | --- |
| `go test ./internal/bot` | PASS |
| `go test ./internal/runtime` | PASS |
| `go test ./internal/channelplatform/...` | PASS |
| `go test ./internal/commerce/...` | PASS |
| `go test ./internal/ai` (also rerun with `-count=1`) | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS, no races reported |
| `go build -o /tmp/zidi-r1-api ./cmd/api` | PASS; output kept outside repository |
| `npm run build:admin` | PASS |
| `git diff --check` | PASS |
| `npm run test:journeys` against actual API/PostgreSQL | 13/13 PASS |

New tests cover canonical JSON formatting versus mapping tampering, templates/compiler routing, CRUD/tenant/permissions, immutable cloning, preview freshness/no writes, restart/replay/stale/expiry, pinned versions, opaque IDs, stock changes during checkout and after review, duplicate orders, payment-claim refusal, Flow Back/expiry/redaction and duplicate Flow commerce submissions, leases, menus, controlled tests, crypto, signed encrypted HTTP, safe audit classification, provider lifecycle/revision/hash guards/failure isolation and Paystack test credential classification.

Full-chain PostgreSQL browser testing found and fixed two defects missed by SQLite fixtures: JSONB formatting differences blocked publication, and an unqualified ON CONFLICT expiry column was ambiguous in PostgreSQL. The provider lifecycle test also found a draft retry lookup using the new attempted ID instead of the existing deployment; that was fixed.

Normal AI tests include the unchanged 105-case deterministic Phase C evaluation. Final AI verification directed generated artifacts to `/tmp/zidi-r1-ai`; prior live reports were not overwritten. Normal tests intentionally skip opt-in live Ollama/Groq harnesses.

macOS race compilation emitted existing linker LC_DYSYMTAB warnings, but did not report a race or fail the suite. Initial sandbox attempts at existing Meta/TikTok mock-server tests could not bind localhost; rerunning with local-port access passed.

## 13. Browser/UI Test Results

**13/13 passed; zero browser JavaScript errors.** The actual API and PostgreSQL-backed admin were tested with a seeded local merchant, not mocked frontend responses. Desktop 1440x1000 and mobile 390x844 checks cover template creation, editing/undo/redo, channel binding, save, drag/keyboard reorder with stable dimensions, options/branching, long store/product labels, invalid quantity, shared Chat/Flow preview, no preview orders, validation, immutable publication/cloning/history, non-overlapping desktop columns, mobile tabs and viewer UI/API restrictions. The final suite additionally exercised invalid-draft validation and one explicitly confirmed controlled test order: one new order, total 450000 minor units (NGN 4500), payment not marked paid.

Results and screenshots: `/tmp/zidi-r1-browser/results.json`, `desktop-chat.png`, `desktop-flow.png`, `mobile-chat.png`, `mobile-flow.png`, `viewer-readonly.png`, `controlled-test.png`. Desktop and mobile Flow screenshots were visually inspected; controls and long labels remain readable without horizontal overflow or overlapping columns. Credentials remain in a private local manifest, not this report. Local QA is available at `http://127.0.0.1:3017/journeys`, using the isolated API on port 8087.

## 14. Live Meta Validation Results

**Not performed; not claimed passed.** R1 has no designated sandbox WABA/test phone and explicitly authorized recipient configured for this run, no R1 public HTTPS data endpoint, and no verified Paystack test-mode setup for a live round trip. `WHATSAPP_FLOWS_ENABLED=false` throughout local QA. No provider Flow was created/uploaded/published/deprecated on Meta. No production phone/WABA was altered and no WhatsApp message was sent. Official Meta Flow JSON and official WhatsApp sample sources were consulted; fake Graph lifecycle tests are contract/error-path tests, not live provider evidence.

## 15. Known Limitations and External Blockers

Meta/device validation remains mandatory, particularly native back/return branches, repeat-cart routing and payment-screen refresh. Location/media collection and cross-journey menus cannot compile into this Flow subset; guided chat is the explicit supported presentation for those constructs. One active connection binding per journey. In-app Flow preview approximates component presentation, not Meta's renderer. Controlled local payments are simulated, not real Paystack test transactions. Store notification recipient metadata must be configured. Expired preview/token/receipt retention cleanup remains operational housekeeping. Uncertain remote Flow creation needs reconciliation rather than automatic duplicate creation.

There was no production deployment and no clean integrated staging deployment. This report is not authorization to enable real channels.

## 16. Files Changed

New files:

- `migrations/000032_phase_r1_journey_studio.sql`
- `apps/api/internal/bot/journey.go`
- `apps/api/internal/bot/journey_templates.go`
- `apps/api/internal/bot/journey_flow.go`
- `apps/api/internal/bot/journey_handler.go`
- `apps/api/internal/bot/journey_test.go`
- `apps/api/internal/runtime/journey.go`
- `apps/api/internal/runtime/journey_lock.go`
- `apps/api/internal/runtime/journey_flow.go`
- `apps/api/internal/runtime/journey_controlled.go`
- `apps/api/internal/runtime/journey_handler.go`
- `apps/api/internal/runtime/journey_test.go`
- `apps/api/internal/channelplatform/whatsapp/flows.go`
- `apps/api/internal/channelplatform/whatsapp/flow_crypto.go`
- `apps/api/internal/channelplatform/whatsapp/flow_handler.go`
- `apps/api/internal/channelplatform/whatsapp/flows_test.go`
- `apps/api/internal/channelplatform/whatsapp/flow_crypto_test.go`
- `apps/api/internal/channelplatform/whatsapp/flow_handler_test.go`
- `apps/api/internal/commerce/core/journey_payment_test.go`
- `apps/api/cmd/journey-qa-seed/main.go`
- `apps/admin/src/pages/Journeys.tsx`
- `apps/admin/src/pages/JourneyFlowPreview.tsx`
- `apps/admin/src/pages/journeys.css`
- `scripts/journey-studio-e2e.mjs`
- `docs/journey-studio.md`
- `docs/phase-r1-implementation-report.md`

Modified integration files:

- `apps/api/internal/bot/service.go`, `handler.go`
- `apps/api/internal/runtime/service.go`, `handler.go`, `channel_boundary.go`, `adapter_sender.go`
- `apps/api/internal/channelplatform/whatsapp/service.go`, `handler.go`, `adapter.go`
- `apps/api/internal/commerce/core/service.go`
- `apps/api/internal/config/config.go`
- `apps/api/cmd/api/main.go`
- `apps/api/go.mod`, `apps/api/go.sum` (rate-limiter dependencies)
- `apps/admin/src/App.tsx`, `components/Shell.tsx`
- `package.json` (`test:journeys` script)

Preserved pre-existing unrelated work: modified `docs/operator-guide.md`, `docs/whatsapp-adapter.md`, and untracked `docs/production-readiness.md`, `scripts/production-readiness-check.sh`. Historical repositories and remote deployments were untouched.

## 17. Final Decision

**REQUIRES META ACCESS.** Local application verification supports merchant UAT of the builder/guided/read-only-preview experience. Flow provider acceptance, real device delivery and a genuine Paystack test payment round trip remain external gates; do not promote to production or integrated staging on the strength of mock tests.
