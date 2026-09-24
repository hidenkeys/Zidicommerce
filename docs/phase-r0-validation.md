# Phase R0 validation record

Validation date: 2026-09-24

This record covers the multi-channel onboarding and messaging implementation on
`codex/phase-r0-multi-channel`. It records automated and local evidence only. No
staging or production deployment was performed.

## Automated validation

| Check | Result | Notes |
| --- | --- | --- |
| `go test ./internal/channelplatform/...` | Passed | Shared platform plus Meta, TikTok, and WhatsApp adapters |
| `go test ./internal/runtime` | Passed | Provider-neutral processing, AI replies, handoff, and channel isolation |
| `go test ./internal/ai` | Passed | Run with `AI_PHASEC_ARTIFACT_DIR=/tmp/zidicommerce-ai-evaluations` as required by the repository test script |
| `go test ./internal/commerce/...` | Passed | Existing commerce-core tests passed; several domain packages currently contain no tests |
| `go test ./...` | Passed | Complete API package suite |
| `go test -race ./...` | Passed | Run with a writable Go cache and localhost access for `httptest` servers |
| `go build -o /tmp/zidicommerce-api ./cmd/api` | Passed | API binary compiled successfully |
| `npm run build:admin` | Passed | TypeScript and Vite production build, 1,860 modules |
| `npm run build:field` | Passed | TypeScript and Vite production build, 40 modules |
| `git diff --check` | Passed | Checked before the documentation commit |

The first sandboxed race attempt could not open disposable localhost listeners;
the same suite passed outside that network sandbox. The macOS linker emitted
`LC_DYSYMTAB` warnings but returned a successful build and test result, with no
race reports.

## Migration validation

All migrations from `000001` through `000031` were applied to a fresh,
disposable PostgreSQL 16 database with pgvector. The environment-gated migration
test passed:

```text
ZIDI_MIGRATION_TEST_DATABASE_URL=postgres://... \
  go test ./internal/migrations \
  -run TestRepositoryMigrationsApplyToEmptyPostgres -v
```

The disposable database and local API/admin services were stopped after
validation.

## Runtime integration evidence

Deterministic tests verify that Instagram and Facebook inbound messages use the
shared conversation runtime, produce AI outbound replies through the selected
provider identity, and respect human handoff. Cross-channel tests verify that
equal external customer or message identifiers remain isolated by organization
and channel. Meta signature rejection, idempotency, and the 24-hour reply window
are covered.

TikTok is deliberately absent from messaging runtime tests. Its implemented
surface is Login Kit OAuth, profile discovery, refresh, and revocation; messaging
capabilities remain `unsupported_by_provider`.

## Admin UI evidence

The Channels page was exercised against a local API and migrated PostgreSQL
database at desktop and 390 by 844 mobile viewports. WhatsApp, Instagram,
Facebook, and TikTok rendered as separate provider cards. OAuth detail,
capability status, permission state, health, recovery actions, and the guided
setup flow remained usable without horizontal overflow.

The UI truthfully showed Instagram and Facebook messaging as awaiting provider
permission review and TikTok messaging as unsupported. No token or secret value
was present in the browser state or rendered diagnostics.

## External access evidence

Read-only inspection found the Meta app `ZidiHQ` in Live mode with WhatsApp and
Webhooks installed. Messenger, Instagram, and Facebook Login for Business were
available but not installed; relevant messaging permissions remained at
Standard Access with no App Review approval, and Page/Instagram messaging
subscriptions were empty.

The available TikTok developer session was not authenticated, so the app's
approved products and scopes could not be verified. Publicly verified Login Kit
and profile capabilities are implemented; no customer-service direct-message
API is claimed.

Railway staging was inspected read-only. The API, database, and admin services
were running, but there were no connected provider accounts, identities, or
credentials. No Railway deployment or variable change was made.

## Decision

- Application code: ready for a clean integrated staging validation when that
  deployment is explicitly authorized.
- Instagram and Facebook live merchant messaging: **requires App Review** and
  provider product/subscription configuration.
- TikTok app-specific capability verification: **requires provider access**.
- Production readiness: intentionally outside Phase R0.
