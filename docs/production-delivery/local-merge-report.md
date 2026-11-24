# Local work merge report

Source: read-only LOCAL at e0019e6 plus working changes. Target: codex/commerce-production-v2 at 69bcd17, 62 commits ahead. No push, deployment, Railway access, migration, auth or RBAC change.

## Per-file outcomes

| Path | Outcome |
|---|---|
| `apps/api/cmd/api/main.go` | merged clean |
| `apps/api/internal/channelplatform/handler.go` | identical-skipped |
| `apps/api/internal/channelplatform/service.go` | identical-skipped |
| `apps/api/internal/channelplatform/whatsapp/embedded_signup.go` | merged clean |
| `apps/api/internal/channelplatform/whatsapp/embedded_signup_test.go` | merged clean |
| `apps/api/internal/channelplatform/whatsapp/onboarding.go` | identical-skipped |
| `apps/api/internal/channelplatform/whatsapp/onboarding_test.go` | identical-skipped |
| `apps/api/internal/commerce/core/handler.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `apps/api/internal/commerce/core/service.go` | Conflict resolved: kept HEAD callback URL and safe Paystack validation-rejection retry (bbced7b, f20a4e2). Local stabilization already present. |
| `apps/api/internal/commerce/core/service_test.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `apps/api/internal/commerce/core/webhooks.go` | identical-skipped |
| `apps/api/internal/config/config.go` | Conflict resolved: kept designated-tester parsing and guarded staging Flow support (08f5738, 944a394); added both local sandbox signup configuration fields. |
| `apps/api/internal/config/config_test.go` | Conflict resolved: retained HEAD guarded staging Flow regression test (944a394). |
| `apps/api/internal/runtime/actions.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `apps/api/internal/runtime/bridge.go` | identical-skipped |
| `apps/api/internal/runtime/journey_test.go` | Conflict resolved: retained HEAD job-service configuration required by newer atomic notification dispatch (bd1a4bb). |
| `apps/api/internal/runtime/outbound.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `apps/api/internal/runtime/service.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `apps/api/internal/runtime/service_test.go` | Merged clean; no net diff: local behavior already present in newer HEAD. |
| `docs/meta-whatsapp-connection.md` | merged clean |
| `docs/operator-guide.md` | merged clean |
| `docs/whatsapp-adapter.md` | merged clean |
| `apps/api/internal/commerce/core/beta_payment_test.go` | Kept HEAD: contains all local tests plus newer Paystack retry/callback coverage (f20a4e2, bbced7b). |
| `docs/production-delivery/current-status.md` | Kept HEAD: local content is superseded by later staging evidence (467cd64 through 01b991b); no local behavior/evidence lost. |
| `docs/production-delivery/decision-log.md` | identical-skipped |
| `docs/production-delivery/master-plan.md` | identical-skipped |
| `docs/production-delivery/phase-2-audit.md` | identical-skipped |
| `docs/production-delivery/phase-3-stabilization.md` | identical-skipped |
| `docs/production-delivery/release-manifest.md` | Kept HEAD: local content is superseded by later staging evidence (467cd64 through 01b991b); no local behavior/evidence lost. |
| `docs/production-delivery/risk-register.md` | Kept HEAD: local content is superseded by later staging evidence (467cd64 through 01b991b); no local behavior/evidence lost. |
| `docs/production-delivery/test-evidence.md` | Kept HEAD: local content is superseded by later staging evidence (467cd64 through 01b991b); no local behavior/evidence lost. |
| `docs/production-readiness.md` | Added local runbook; marked historical and linked current evidence to prevent stale migration/state claims. |
| `scripts/production-readiness-check.sh` | identical-skipped |

## Validation

- `npm ci`: passed; installed 131 packages from package-lock.json.
- `npm run build:admin`: passed TypeScript and Vite build; chunk-size warning only.
- `npm run build:field`: passed TypeScript and Vite build.
- `npm --workspace apps/admin run test:components`: 3 passed, 0 failed.
- Default `cd apps/api && go build ./...`: environment failure (unwritable default cache and inconsistent toolchain root); reproduced in pristine checkout.
- With the environment below, `go build ./...` and `go vet ./...`: both passed.
- Same environment, `go test -json ./...`: 467 test/subtest pass events, 4 failures, 8 skips; 17 packages passed, 4 failed, 13 had no tests.
- Pristine 69bcd17 with the same environment: 466 test/subtest pass events, the same 4 failures and 8 skips; identical package counts.
- All four failures are sandbox loopback-listener denials: TestGatewayAuthenticationAndSafeErrors, TestMetaOutboundRequiresCustomerWindowAndUsesSelectedIdentity, TestTikTokLoginKitLifecycleAndProfileDiscovery, TestSMTPSenderRequiresSTARTTLS. No merge-caused failure found.
- `git diff --check`: passed.

Exact Go command prefix (run from apps/api; append build ./..., vet ./..., or test -json ./...):

```sh
env GOROOT=/Users/oluwateniolasobande/sdk/go1.24.3 GOCACHE=/tmp/commerce-merge-go-cache AI_PHASEC_ARTIFACT_DIR=/tmp/commerce-merge-ai /Users/oluwateniolasobande/sdk/go1.24.3/bin/go
```

Baseline method: requested `git stash push -u -m commerce-local-merge-baseline-check` failed with `error: could not write index`; no stash was created. Used `git clone --no-hardlinks --no-checkout . /tmp/commerce-merge-review` and checked out the unchanged target branch at 69bcd17 there. Raw JSON logs reside in /tmp/commerce-merge-audit/go-test.json and pristine-go-test.json.

## Delivery limitation and lead actions

The session permission profile makes REPO/.git read-only. Working files are merged, but committing on REPO is blocked. Logical commits are prepared on the same branch in /tmp/commerce-merge-review, with an apply-ready format-patch series in /tmp/commerce-local-merge.patch. Apply that series to a clean 69bcd17 checkout in a session with Git metadata write access. Preserve or replace the current merged working files before applying to avoid duplicate changes. No product decision remains; the lead needs a permitted commit/apply session and an unrestricted loopback test rerun. No production readiness is asserted.
