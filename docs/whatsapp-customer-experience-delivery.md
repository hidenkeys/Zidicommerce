# WhatsApp customer experience delivery

## Branches and checkpoints

Active implementation: `codex/whatsapp-customer-experience`.

Historical local phase branches are renamed under `archive/phase-*`; commits and remote branches remain intact. No remote branch is deleted. The Railway recovery snapshot remains unchanged. Unrelated operator/readiness working-tree changes are preserved outside these commits.

Baseline: `70bc9a2` checkpoints the previously uncommitted Journey Studio implementation. Its historical verification report is not evidence of a current deployment or Meta acceptance.

## Delivery sequence

1. Verify and checkpoint existing journeys, commerce and provider boundaries.
2. Harden default commerce checkout: authoritative totals, Paystack metadata, duplicate submission protection, signed verification and recovery paths.
3. Validate published visual Flow on an isolated WhatsApp test identity, including cart changes, Back, checkout return and fulfilment notifications.
4. Complete guided-chat editing and validate the same purchase scenarios.
5. Integrate Vercel-backed autonomous conversations after choosing SDK/Gateway/hosting responsibilities; keep commerce authorization and confirmation in Go.
6. Add measured messaging and AI usage, distinguish estimates from actual charges, then controlled rollout.

## Redis decision

The current runtime already persists sessions, execution leases, action receipts and jobs in PostgreSQL. Do not replace these with ephemeral Redis records. Redis is not introduced in this initial checkpoint: doing so would add deployment and failure modes without completing the customer purchase journey.

The first suitable Redis workload is shared rate limiting across API replicas, followed by optional short-lived caching or job wake-up notifications. Implement it as an explicit follow-up with tenant-prefixed keys, TTLs, bounded timeouts, outage behavior and multi-instance tests. PostgreSQL remains authoritative; job payloads and payment idempotency must survive Redis loss. Never cache authoritative checkout totals or payment success.

## Deployment gate

Deploy only to an identified ZidiCommerce test environment after its API/admin builds and migration state are verified. The currently connected Railway accounts do not list ZidiCommerce. Do not redeploy unrelated projects or alter the public Zidi website. Live Meta acceptance and genuine Paystack test payment evidence remain outstanding; local previews and simulated payments do not satisfy these gates.
