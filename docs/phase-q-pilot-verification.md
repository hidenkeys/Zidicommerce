# Phase Q Pilot Gap Verification

> **Retired 2026-09-01:** this report is retained as historical pilot evidence. The pilot service and its dedicated test database were replaced by the isolated general staging stack. Use [Staging deployment](staging-deployment.md) and `scripts/staging-smoke.sh` for current validation. Do not run the pilot verifier against staging or production.

Date: 2026-08-31

Scope: isolated Railway WhatsApp pilot only. No production API, shared Admin deployment, Meta configuration, live-mode payment credential, or source merchant database was changed.

## Failure classification

| Reported gap | Classification | Root cause | Resolution |
|---|---|---|---|
| No completed payment-to-fulfilment path | External action in the original run; report was later stale | The original run stopped before a provider checkout, but later pilot acceptance produced genuine Paystack test-mode signed-webhook evidence. | Reused one exact provider-verified paid pilot order and completed the normal order state machine. |
| Unpaid orders rejected for processing/ready/completed | Expected safety behavior | The authoritative order state machine correctly blocks fulfilment before verified payment. | Retained the rule and changed the verifier classification to `expected_safety_block`. |
| Direct fulfilment endpoint could advance independently | Application defect | `UpdateFulfilment` previously maintained a second status path that did not consult the order/payment state machine. | Status updates now map through `TransitionOrder`; an unpaid direct-ready regression test proves fail-closed behavior. |
| AI tests referenced missing `support_handoffs` | Test fixture defect | Three SQLite fixtures omitted a runtime table that production migrations already create. | Added `runtime.SupportHandoff` to those fixture migrations without removing handoff behavior. |
| Healthy WhatsApp connection showed `requires_attention` | Application status defect plus legacy evidence shape | A historical test-send error and missing narrow `last_inbound_test_at` marker outweighed real signed inbound/outbound evidence. | Historical errors stop blocking completed readiness; a real inbound plus valid signature is accepted as transport evidence. |
| Signature status became `rejected` after a negative test | Reporting defect | The latest rejected timestamp replaced durable valid-signature readiness. | Valid and rejected evidence are separate. Rejections stay counted and return HTTP 403 without erasing verified readiness. |
| Repeated negative probe did not increase the count | Verification harness brittleness | Identical test payloads intentionally deduplicated to one provider event. | Each verifier security probe now has a unique non-sensitive event key. |

## Final isolated-pilot result

| Check | Classification | Evidence |
|---|---|---|
| API health | `pass` | Health endpoint returned HTTP 200 after deployment. |
| Pilot authentication | `pass` | Authenticated owner session remained valid. |
| Tenant commerce seed | `pass` | Active tenant-scoped stores and products remained available. |
| Grounded enquiries | `pass` | Product, store, delivery, and unknown-product checks returned grounded responses. |
| Order-to-payment initialization | `manually_verified` | Controlled pilot order flow retained provider checkout evidence. |
| Paystack payment | `sandbox_verified` | Test-mode checkout was committed only through signed webhook/provider verification. |
| Unpaid fulfilment | `expected_safety_block` | Direct ready transition returned HTTP 400 and changed no authoritative state. |
| Post-payment fulfilment | `sandbox_verified` | Verified paid order advanced through processing, ready, and completed; order and fulfilment ended consistently with timeline evidence. |
| Customer status delivery | `sandbox_verified` | Fresh outbound events and Meta delivered callbacks followed the controlled operational transitions. |
| Human handoff | `manually_verified` | Tenant-scoped history remains readable; claim/release/resume was verified during the pilot. |
| WhatsApp setup and health | `pass` | Completed setup and connection health both resolve to healthy. |
| Invalid signature | `pass` | Unique invalid request returned HTTP 403, increased rejected security evidence, and preserved verified readiness. |
| Provider/runtime evidence | `pass` | Sent, delivered, and rejected-security event classes remain separately observable. |

## Verification commands

The following local gates passed:

- `go test ./internal/ai`
- `go test ./internal/channelplatform`
- `go test ./internal/channelplatform/...`
- `go test ./internal/runtime`
- `go test ./internal/commerce/...`
- `go test ./...`
- `go test -race ./...`
- `go build ./cmd/api`
- `npm run build:admin`
- `bash -n scripts/whatsapp-live-smoke.sh`
- `bash -n scripts/phase-q-pilot-verify.sh`
- `git diff --check`

The race suite passed with non-fatal macOS linker warnings about malformed `LC_DYSYMTAB` metadata. No race was reported.

## Deployment and limits

The API changes were deployed successfully to the isolated pilot and its health check passed. No migration was required. The Admin change was built locally but was not deployed because the Railway Admin service is shared rather than pilot-specific.

This is Paystack test-mode evidence, not authorization for live charges. Production promotion, live-mode payment validation, and any shared Admin deployment remain separate approval gates.

## Production boundary

This report proves isolated-pilot behavior only. It does not attest the production database migration table, production seed/adoption flags, production provider modes, shared Admin artifact, Meta callback, backup/restore readiness, or monitoring ownership. Those gates require a separate readiness review before any production deployment.
