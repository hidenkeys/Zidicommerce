# Phase 3 release notes

**Bing Chun · designated testers · Paystack test payments · simulated fulfilment**

Reviewed the Phase 2 audit, shared commerce checkout, payment proof and webhook processing, customer payment checks, admin payment routes, channel onboarding, runtime, API startup and existing tests. Production and the main Zidi website were outside the work.

## Candidate changes

| Area | Change |
|---|---|
| Payment mode | `CONTROLLED_BETA=true` requires Paystack and rejects a non-test environment key. Tenant Paystack credential writes, provider resolution and webhook signature selection enforce test mode. The generic order transition cannot set an order to paid in beta. Previously created checkout URLs without test-mode attestation are refused on reuse in beta. |
| Payment access | Admin initialization and verification require `payments.manage`. Store-scoped verification checks order access. Runtime checks the conversation customer owns the order before verification and before returning payment data. |
| Checkout initialization | Reserve the payment and reference while holding the order row lock before making the provider call. A documented validation rejection releases the unused reservation so corrected details can be retried with the same key. A successful replay reuses the saved checkout. Ambiguous failures retain the reservation for reconciliation. |
| Verification | Paystack proof must contain the exact reference, amount and currency. Webhook routing refuses ambiguous references, verifies the signature before processing, and binds the transaction to the matched tenant and payment. Timeout and not-yet-paid results return a retryable error. Successfully processed events retain their replay protection. |
| WhatsApp only | Beta startup excludes other-provider OAuth routes, the Instagram/Messenger webhook and their senders. Connection creation and runtime inbound, outbound and queued sends enforce the restriction. Beta config rejects native Flows, signature bypass, demo seed, pilot seed and one-time number adoption. |

## Operator behavior and staging history

- **Expected safety behavior.** An uncertain initialization leaves a pending reservation with no checkout URL. Retries will not open a second checkout. An operator must inspect the reference in the Paystack test dashboard and reconcile it before any fresh order is placed. There is no automatic recovery of an authorization URL lost between provider success and the local update. Existing failed webhook receipts from older releases may require manual reconciliation.
- **Sandbox acceptance.** The sandbox account has a verified staging callback and a confirmed delivery receipt for the test send. The CRM app-level callback was left unchanged. A signed customer reply reached staging, but the runtime rejected it because it could not load the published bot snapshot. The Bing Chun pilot journey is now published and bound to the staging sandbox in guided chat mode with the `order` trigger. It uses the staging QA catalogue and Paystack test credentials. A fresh order message was processed and onboarding is connected. Two outbound capability records were repaired on the sandbox after manual onboarding failed to activate them. Meta confirmed the store prompt was delivered. Paystack verified NGN 7,500 for test order ZC-20261110-8F479987. The order and simulated pickup both reached completed using the normal staging APIs. A subsequent inbound message reset the onboarding label to inbound test received, so setup was completed again.
## Merged build verified in staging

On November 10, 2026, [PR #1](https://github.com/hidenkeys/Zidicommerce/pull/1) merged as `0a3ac420793c6e612c57d233ef231310057c7a9d` after the API and frontend CI checks passed. Railway deployed that commit to the staging API as `f5f0c459-6f0e-41a9-bf09-ededac06afaf`. The API and database reported healthy. The preceding staging deployment was `ca9a6402-801b-4000-bd7d-a49e951a78f2`. [GitHub CI](https://github.com/hidenkeys/Zidicommerce/actions/runs/42768259276) · [Staging deployment](https://railway.com/project/3c6283f1-4371-45aa-8763-627e226b8ef0/service/146fa126-0640-4d8f-95c1-fe60326aac61?id=f5f0c459-6f0e-41a9-bf09-ededac06afaf)

- **Paystack checkout retry passed.** The staging API returned HTTP 400 for the invalid checkout. Corrected details succeeded with the same idempotency key, and replay reused the payment reference. The unpaid probe order `ZC-20261110-14749BB5` was cancelled.
- **Bing Chun guided checkout passed.** Paystack verified NGN 5,000 in test mode for WhatsApp order `ZC-20261110-27E71F8C`. The order and simulated pickup both reached Completed. The WhatsApp sandbox stayed Connected.

## Staging verification of `08f5738`

On November 10, 2026, Railway deployed `08f5738` to the staging API. This build contains the [PR #3 merge](https://github.com/hidenkeys/Zidicommerce/pull/3) at `35b1799` plus the existing WhatsApp designated-tester filter from `46810fe`. The staging configuration check confirmed the tester allowlist and Paystack Test Mode. The designated-tester filter must be included in the approved beta artifact. [Railway deployment](https://railway.com/project/3c6283f1-4371-45aa-8763-627e226b8ef0/service/146fa126-0640-4d8f-95c1-fe60326aac61?id=47101822-8352-442d-9163-cba5647000eb)

| Worker | Cross-tenant result | Same-tenant control |
|---|---|---|
| Notification delivery | Target remained queued with zero attempts and no outbound message linked. | Notification without a channel or recipient moved to Skipped. |
| Outbound delivery | Target remained queued with zero attempts and no provider message ID. | The worker claimed the message and marked it Skipped because the synthetic channel had no sender configured. |
| Dispatch timeout | Target remained Notified with no response timestamp. | The attempt moved to Timeout and recorded a response timestamp. |

All six disposable jobs and their test data were removed. Background job tenant isolation is closed for these three paths.

## Gates before the controlled beta opens

1. Confirm the dedicated Bing Chun tenant, eligible stores, named fulfilment lead and designated testers. Verify access is limited to those testers, and attest the signed WhatsApp routing and approved credentials in the target environment. Keep Paystack in test mode, pickup simulated, native Flows disabled, and signature bypass and unsafe seeds disabled.
2. Background job tenant isolation passed for notification delivery, outbound delivery and dispatch timeout. Staff revocation, tested cross-tenant API routes, ambiguous payment references, concurrent checkout, signed replay and a provider-delivered payment transition have staging evidence below. First-attempt Paystack delivery, concurrent processing of a previously unseen event, concurrent replay of a processed event, and a sampled staging log scan passed.
3. Pin the approved API and frontend artifacts, including the WhatsApp tester filter in the API build. Verify the target database migration state and single migration runner, rehearse restore and rollback, and confirm alerts reach a named operator.
4. Obtain signed Bing Chun UAT and explicit launch approval for the target environment. If the intended launch uses real payments or fulfils real orders, approve and verify that expanded scope separately.

## Staging safety checks on November 10, 2026

| Check | Result | Evidence and scope |
|---|---|---|
| Staff access revocation | **Passed after remediation.** | Disabling a disposable tenant's merchant admin membership blocked fresh login with HTTP 401. Its already issued token still returned HTTP 200 from authentication, current organization, and orders endpoints. The staging retest below confirmed that existing tokens were rejected after disablement and demotion. |
| Cross-tenant API isolation | **Passed for tested routes.** | The separate tenant received HTTP 404 for the Bing Chun order, pickup, payment verification and WhatsApp configuration. Worker isolation and ambiguous payment-reference routing have separate evidence in these notes. |
| Signed Paystack webhook replay | **Passed for an already paid order.** | A bad signature returned HTTP 403. Two submissions of the same test-signed event for `ZC-20261110-27E71F8C` returned HTTP 200 and reused processed receipt `c409d161-9b9d-4692-bba0-b1fd36d1d9c8`. The staging API verified the Paystack transaction and the order stayed Completed. The event was constructed for this probe. |
| Provider-delivered Paystack payment | **Passed after redelivery in Test Mode.** | Bing Chun WhatsApp order `ZC-20261110-07C94121` was Awaiting payment for NGN 5,000. Paystack's original delivery went to the CRM Flutterwave endpoint and received HTTP 400. After the Test Webhook URL was corrected, Paystack resent the original `charge.success` event to staging. Staging processed the event at 10:24:02 UTC (11:24:02 Lagos time), and the order audit recorded `payment_webhook_verified` from Awaiting payment to Paid. The tester left Check payment untouched. This verifies redelivery and payment confirmation. |
| First-attempt Paystack delivery | **Passed in Test Mode.** | Paystack recorded one delivery attempt for the NGN 5,000 payment on fresh Bing Chun order `ZC-20261110-88697F27`. The staging endpoint returned HTTP 200 for `charge.success`. The order audit shows one transition from Awaiting payment to Paid at 21:20:26 UTC (22:20:26 Lagos time) on November 10, 2026, recorded as `payment_webhook_verified`. The tester left Check payment untouched. |
| Concurrent Paystack replay | **Passed for a processed event.** | Six simultaneous signed copies of the event for `ZC-20261110-07C94121` returned HTTP 200 and reused the same processed receipt. The order audit retained one transition to Paid. This probe covered concurrent replay after processing. The separate first-processing probe below covered a previously unseen event. |
| Concurrent first processing of a Paystack webhook | **Passed in staging.** | A disposable order started Awaiting payment with a Pending payment and zero receipts for the probe event. Six simultaneous test-signed copies of the same previously unseen `charge.success` event referenced a successful NGN 100 Paystack Test Mode charge. Staging verified the transaction against Paystack. All six returned HTTP 200 with receipt `825503c7-c81a-4030-913e-dbf0dd98ad11`. PostgreSQL recorded one processed receipt, one Awaiting payment to Paid transition, one payment confirmation event, and one notification record. The concurrent requests were generated by the probe. The genuine provider delivery for this disposable charge had arrived separately before the order existed. Staging fixtures were removed. |
| Staging log and webhook error scan | **No exposed secrets detected in the sample.** | The scan covered 28 API runtime entries and eight webhook ingress entries from the probes. It checked for credential and signature patterns and selected configured secrets. Malformed payload and invalid signature probes returned generic HTTP 400 and 403 errors. |
| Concurrent checkout | **Passed in staging.** | Six parallel requests used distinct keys for `ZC-20261110-6AA169F0`. One returned HTTP 200 and five returned HTTP 409. A later retry reused the checkout. PostgreSQL-backed API results showed one payment row and one successful reference. The unpaid test order was cancelled. |

Staff revocation remediation passed in staging on November 10, 2026. Deployment `024ab41d-9b2a-4a94-8456-83ac0c6abc7e` runs commit `d45c51e`. Two disposable merchant admin memberships exercised demotion to Viewer and disablement through the staff API. Both existing tokens returned HTTP 401 on the identity, organization and order endpoints. A fresh Viewer token worked with reduced permissions, and a disabled user could not sign in. The full API suite passed.

On staging commit `bfe1dff`, the notification worker completed a job owned by one disposable tenant and changed another tenant's queued notification to Skipped. The notification had no channel or recipient. A same-tenant control also completed and changed its own notification to Skipped. Both fixtures were removed. This was a launch blocker until the staging retest below. Code inspection also found that outbound delivery and field-service dispatch timeout handlers look up payload resources by ID without checking the job tenant. The initial probe covered notification delivery. The later retests covered all three job types.

The ambiguous Paystack reference probe passed. Two disposable tenants held pending payment rows with the same reference. The staging webhook returned HTTP 409 before signature verification, both payments stayed pending and no webhook receipt was created. Both rows were removed.

Worker tenant isolation passed on staging API deployment `ab815d77-91a7-4bdb-bfe5-c9b4d6b842ac` from `aaa8abd`. Three jobs targeting another tenant left the notification queued, the outbound record queued with zero attempts and no provider message ID, and the dispatch attempt notified with no response timestamp. Three same-tenant controls exercised the corresponding paths. The outbound control used a channel without a configured sender to avoid sending an external message. All probe data was removed.
