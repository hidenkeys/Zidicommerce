# Default WhatsApp commerce acceptance

## Current evidence

The default order journey covers store/category/product/variant selection, quantity, stock checking, repeated additions, fulfilment details, customer contact, live cart review, explicit confirmation, order creation, Paystack initialization, verification and notifications. Merchant operations advance fulfilment; payment does not mean goods have already been delivered.

Commit `742b2a5` replaces checkout detection by English response text with an explicit internal checkout marker. Deterministic Flow tests exercise pending-payment refusal, independently verified payment, final-response replay without duplicate journey notifications, and pickup fulfilment through processing, ready and completed. The provider in those tests is a mock, not a real Paystack payment.

## Live prerequisites

### Staging release verification

On 2026-11-04, application commit `29fbd8e` (code change `742b2a5`) was deployed to the staging API as deployment `c0c48258-7716-4d9a-87e3-849627569873`, status SUCCESS. Full `go test ./...` and `go test -race ./...` passed. The API health endpoint returned HTTP 200 with database `ok`; staging admin login and the unchanged main website both returned HTTP 200. Admin code was unchanged and its previous deployment was retained. This release does not establish a live WhatsApp or real Paystack test-payment result.

- Staging project only: `3c6283f1-4371-45aa-8763-627e226b8ef0`, environment `ea7de095-ecf9-40bf-bde6-6a321ea847b4`.
- An explicitly designated sandbox WhatsApp connection and authorized recipient.
- Existing Meta access token, app secret and verified webhook configured securely on that connection. Do not put credentials in this document.
- `WHATSAPP_FLOWS_ENABLED=true`, `WHATSAPP_FLOWS_TEST_WABA_ID` and `WHATSAPP_FLOWS_TEST_PHONE_ID` must match that sandbox connection. Do not substitute a production identity.
- Public HTTPS Flow data endpoint and registered encryption public key.
- Test-mode Paystack configuration. Live keys must remain rejected for the Flow pilot.
- Tenant store notification recipient configured for the test. Store notifications also require an authorized test recipient.

At inspection on 2026-11-04, staging had a test-mode Paystack key but no configured Flow test identity. No live recipient had been designated. Do not describe local or mock evidence as Meta acceptance.

### Meta setup progress (2026-11-04)

Browser inspection established the connected Meta test number `+1 555-157-1522`, phone ID `803787309476689`, test WABA `1315307873272485`, business ID `729319212854392`, app ID `784275937440591`. The Nigerian number initially shown in API Setup was unverified and was not selected for staging.

With explicit operator approval, Meta authorization was saved for the nine existing accounts (no future-account opt-in), and a temporary access token was stored through the staging admin's encrypted credential form. The staging connection is Zidi-managed/Sandbox with the verified test references and Graph `v25.0`. No credential value is included in this record. The UI reports Access Token Available; App Secret and Verify Token remain Missing.

App-secret disclosure is blocked by Meta's password reauthentication prompt, which the account owner must complete. The designated recipient is awaiting Meta's five-digit verification. Neither a successful test message nor a live checkout/fulfilment round trip has been established. The existing app-level CRM callback was read but not changed. Flows remain disabled until credentials, routing isolation and recipient verification are complete.

## Actual-device acceptance steps

1. Create a new ordering journey, bind the sandbox connection, choose WhatsApp Flow and validate the draft.
2. Prepare/upload with Meta and inspect all provider validation errors. Resolve them before publishing. Publish the immutable journey and its matching provider Flow; activate only that sandbox binding.
3. Send the entry message from the authorized test recipient. Check categories, prices, variants, quantity, another-item routing and native Back on the actual phone.
4. Confirm the reviewed cart. Capture the order/payment references without exposing Flow tokens. Verify repeated submissions create only one order/payment.
5. Open the generated Paystack test checkout. Confirm the displayed amount and test mode. Complete using Paystack's current official test payment details; never use a real card or live key.
6. Confirm signed webhook/server verification agrees with order reference, amount and currency. A checkout return, screenshot or customer assertion alone must not mark the order paid.
7. Reopen/check payment status. Confirm final customer response and store/customer notification delivery. Check duplicate webhook and repeated status requests do not repeat fulfilment.
8. In merchant operations, advance processing, ready, and completed for pickup. For delivery, also verify the configured delivery transitions. Confirm the customer's order-tracking response reflects the database.
9. Test abandoned checkout, unpaid status check, changed price/stock and expired Flow token. Record real results and provider errors.

Record Meta Flow ID, journey version, device/client, timestamps, Paystack test reference, payment/order/fulfilment statuses and notification delivery evidence. Do not include customer personal details or credentials. Keep the main Zidi website and other Railway projects outside all mutations.

## Paystack merchant onboarding

Paystack's public authentication documentation currently describes API-key authentication. No general merchant OAuth/embedded-signup contract was established during this review. Do not build an imitation Connect button that implies supported delegated authorization.

For merchants retaining their own Paystack businesses, keep the existing encrypted merchant credential storage and ask Paystack whether an approved platform/partner onboarding arrangement is available for Zidi's countries and business model. Restricted keys, where available and sufficient, still require key onboarding and may not cover webhook signature validation.

Subaccounts/split payments are a documented alternative for routing platform transactions to merchant bank accounts. This is not authorization to access an existing merchant Paystack account; it changes settlement and operational responsibilities and needs a separate product/Paystack approval decision. No subaccounts or payment-onboarding changes are implemented by this phase.

Official references:

- https://paystack.com/docs/api/authentication/
- https://paystack.com/docs/developer-tools/api-keys/
- https://paystack.com/docs/payments/split-payments/
- https://support.paystack.com/en/articles/2132226
