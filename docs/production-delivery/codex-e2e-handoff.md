# Codex handoff: full staging end-to-end test

Copy everything below the line into Codex. Codex needs network access to the staging URLs and Meta/Paystack test endpoints.

---

You are taking over end-to-end verification of ZidiCommerce on staging. Work only in this repository:

`/Users/oluwateniolasobande/Zidi/zidi-worktrees/commerce-production-v2` (git clone of `hidenkeys/Zidicommerce`, branch `codex/commerce-production-v2`, pushed to GitHub).

Do not touch `/Users/oluwateniolasobande/Zidi/Zidicommerce` (original working copy with preserved uncommitted work) or `/Users/oluwateniolasobande/Zidi/zidi-ui-redesign-20261119` (original redesign delivery). Never deploy to production, never use live Paystack keys, never message anyone except the designated tester number, and never print or commit passwords, tokens or API keys.

## What has changed on this branch (newest last)

| Commit | Change |
|---|---|
| `69bcd17` | UI redesign of Commerce Admin and Field (base) |
| `0df1a2b` | Merged local Embedded Signup sandbox restriction (`META_SIGNUP_SANDBOX_WABA_ID` / `_PHONE_ID`) |
| `3a1bf85` | Production-readiness runbook and local merge report |
| `6beef43` / `a425954` | `scripts/staging-ui-check.mjs`: headless staging sign-in and evidence capture (handles the product tour) |
| `dc3f9c9` / `c0dc9c2` | Brand v2: concept 03 "orchestrated layers" logo, palette v2 tokens (`packages/shared/src/zidi-tokens.css`), icons |
| `e8c4527` | Shared WhatsApp preview renderer `apps/admin/src/components/whatsapp/*` |
| `d3c7c0d` | Channels rebuilt as a hub + per-connection workspace + connect wizard `apps/admin/src/components/channels/*` |
| `2c3e3d9` | Flow Studio rebuilt with mode-specific editors and live preview `apps/admin/src/components/flow-studio/*`, `lib/experience-modes.ts` |
| `e482bfd` | First-run product tour, readiness card, next-step empty states `apps/admin/src/components/onboarding/*` |
| `2c79448` | Local acceptance stack and suite (`scripts/local-acceptance/*`, `e2e/commerce-acceptance.mjs`), Field runtime action authorization policies (`apps/api/internal/authz/permissions.go`), DSN quoting fix (`apps/api/internal/config/config.go`), sidebar logo fix |

Read first: `docs/production-delivery/test-access.md`, `docs/production-delivery/local-acceptance.md`, `docs/production-delivery/local-merge-report.md`, `docs/brand/final/README.md`, `docs/whatsapp-commerce-live-acceptance.md`, `docs/meta-whatsapp-connection.md`, `docs/journey-studio.md`.

Staging state now: API deploy `f3f388d1`, Admin deploy `1dbcee36` (both from `2c79448`). Last signed-in UI check: 34/34 captures clean. Local acceptance: 14/14 PASS.

Four WhatsApp experience modes (backend values): `automated_commerce` (Automated Commerce), `guided_questions` (Guided Questions), `whatsapp_flow` (WhatsApp Flow), `ai` (AI Conversation). `guided_chat` is legacy.

## Access

Staging Admin, owner login, password file, organizations, WhatsApp test number and tester, Paystack mode: see `docs/production-delivery/test-access.md`. Read the password from the password file inside your scripts only (follow `scripts/staging-ui-check.mjs`), never echo it.

## The test number rule

Use the **same WhatsApp number for every store and organization**: the ZidiHQ business number **+234 808 510 5382** (phone number ID `683059678235387`). Send customer messages from the designated tester only (number ending 2209, from `WHATSAPP_TESTER_NUMBERS`).

One number routes to one connection at a time (phone-level webhook override `…/v1/runtime/webhooks/whatsapp?connection_id=…`). Test organizations sequentially:
1. Record the number's current connection, organization and active experience.
2. For each organization and store under test, bind the number to that organization's connection and experience through the Channels and Flow Studio UI, using the existing activation APIs.
3. Run that organization's tests.
4. At the end, restore the original binding (Bing Chun) and confirm with a final inbound test.

If moving the number between organizations needs Meta-side changes (callback override per connection), stop and report the exact change needed instead of guessing.

Staging currently shows this number as **Needs attention: complete provider onboarding and asset verification**. Diagnose first: health refresh in Channels, the capability records, and the `/v1/runtime/webhooks/whatsapp` logs in Railway. Fix only through supported operator flows.

## Full e2e scope (both organizations: Bing Chun Nigeria (Staging) and a second non-Bing-Chun organization)

For every store in each organization:

1. Auth and access: sign-in, password reset email and invitation email delivered via Resend to the authorised address only, invite acceptance, role-limited staff denial on payments/team/channels/Flow Studio publish (UI and API 403).
2. Catalogue, inventory, stores: create/edit/save round-trips; stock adjust; store hours.
3. Business knowledge: ingest, review chunks, approve, archive; confirm AI Conversation answers from approved knowledge only.
4. Channels: hub status accurate; per-number Overview/Experience/Test/Health/Settings; health refresh; "Send test message" to the tester only, with confirmation; wizard shows Embedded Signup as unavailable with reason (no `META_EMBEDDED_SIGNUP_CONFIGURATION_ID`) and the manual path works.
5. Flow Studio, for each of the four modes: create a new experience from the mode picker, edit, validate, publish, activate on the test number, preview in the shared WhatsApp renderer (Chat, and Flow for WhatsApp Flow), then real WhatsApp messages from the tester. Record the exact messages sent and replies received. Build one WhatsApp Flow from scratch and complete it on the device.
6. Commerce journey per mode: browse catalogue → cart → checkout → Paystack **test** payment (published test card) → webhook/verify → order paid → processing → ready → completed; plus one cancellation. Confirm fulfilment and pickup records.
7. Handoff: request a human, see it in Conversations, reply as staff, resume automation.
8. Field: provider dispatch decline/accept and every job status (the local suite covers this; repeat on staging only if Field is deployed there; it currently is not).
9. Onboarding: fresh organization sees the 8-step tour, readiness card and next-step empty states.
10. Responsive check of every touched page at 390/768/1440 with `scripts/staging-ui-check.mjs` (extend it for interactive steps; keep secrets redacted).

## Rules

- Prefer the real UI for each step; use the API only to assert state.
- Fix genuine bugs minimally on this branch with tests; never weaken assertions. Commit in logical commits ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, push the branch, and redeploy staging with `railway up --service <zidicommerce-staging-api|zidicommerce-staging-admin> --environment staging` from the repository root only after builds and tests pass (`npm run build:admin`, `npm run build:field`, `npm --workspace apps/admin run test:components`, `cd apps/api && go test ./...`).
- Do not change Railway variables or Meta settings without stopping to ask, except where `test-access.md` already says so.

## Deliverables

1. `docs/production-delivery/staging-e2e-report.md`: per organization, store and mode, each step PASS/FAIL with evidence path, the exact WhatsApp messages and replies, order references, Paystack test references (no secrets), bugs found and fixed (commit hashes), and remaining blockers.
2. `docs/production-delivery/user-testing-guide.md`: Admin URL, test number, tester instructions, for each of the four modes the exact messages to send and expected replies, and step-by-step "build and test a WhatsApp Flow in Flow Studio".
3. A short final summary: what passed, what failed, what is deployed (deploy IDs), and what needs the owner (for example Meta Embedded Signup configuration ID, App Review).
