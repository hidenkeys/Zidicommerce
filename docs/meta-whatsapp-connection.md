# Meta WhatsApp connection

Phase J.1 uses Meta WhatsApp Embedded Signup for merchant-facing authorization while preserving Zidi's existing channel and WhatsApp adapter architecture.

## Responsibility boundary

Meta owns:

- Meta login and consent;
- business portfolio, WhatsApp Business Account (WABA), and phone selection;
- authorization and authorization-code issuance;
- Meta Business verification, App Review, permissions, account quality, and phone registration requirements.

Zidi owns:

- the tenant-scoped channel connection and ownership mode;
- the short-lived signup attempt and server-side code exchange;
- validation that the returned phone belongs to the authorized WABA;
- encrypted access-token custody and safe credential status;
- subscription of the Zidi Meta app to the WABA;
- webhook verification/signatures, tenant routing, message normalization, health, events, and metrics;
- the existing conversation, AI, bot, handoff, and commerce runtime.

```text
Meta Embedded Signup
-> tenant-bound completion
-> ChannelConnection + validated Meta assets
-> existing WhatsApp adapter
-> channel-neutral conversation runtime
-> AI / bot / human handoff
-> commerce engine
```

## Merchant-managed onboarding

1. An organization administrator creates or opens a WhatsApp connection with `merchant_managed` ownership.
2. Admin requests a short-lived signup attempt from Zidi. The API returns only the browser-safe Meta app ID, Embedded Signup configuration ID, Graph version, and opaque attempt token.
3. **Connect with Meta** launches the official Facebook JavaScript SDK flow. Meta handles login and asset selection.
4. The browser receives a short-lived authorization code and selected WABA/phone IDs from Meta. The code remains in the callback closure and is sent directly to the Zidi completion endpoint; it is not stored in React state or browser storage.
5. The API derives the organization from the authenticated user, validates the tenant-bound attempt, exchanges the code server-side, verifies the WABA/phone relationship, and subscribes the app to WABA events.
6. The API encrypts the access token with the channel secret store. App-secret and shared verify-token credentials are environment references. Normal APIs never return values or references.
7. Zidi updates the existing provider account and phone identity, then shows merchant-safe business/phone status.
8. The merchant sends the controlled test message, replies from the approved recipient, completes validation, and runs health.

Completion is idempotent. Repeating a completed attempt returns the existing safe connection result without exchanging the code again. Expired attempts require a new launch. A phone or authorized WABA already attached to another connection is rejected without revealing the other tenant.

The browser event may include additional Meta session context, but Zidi does not trust or persist an unverified Business Portfolio identifier from the browser. The server currently validates the WABA and its phone relationship through Graph API; Business Portfolio ownership and selection remain Meta-managed. This avoids inventing or trusting an unsupported association check.

## Zidi-managed onboarding

**Let Zidi help** changes the connection ownership to `zidi_managed` and records an assisted request. Its independent states are:

```text
not_requested -> requested -> in_progress
                              -> awaiting_merchant_action
                              -> blocked
                              -> connected -> completed
```

Only a Zidi platform operator can advance operational states. Merchant action is still required when Meta requires login, consent, business verification, phone ownership, or another account action. The assisted state cannot become connected/completed until server-side Meta authorization is actually recorded.

The assisted flow is coordination, not a bypass. It ultimately uses the same Meta-owned authorization and the same Zidi completion boundary.

## Connection lifecycle

Ownership, transport status, authorization, setup progress, and health are separate:

```text
ownership:     merchant_managed | zidi_managed
connection:    setup_required -> connecting -> connected -> healthy
authorization: not_started -> pending -> authorized
setup:         setup_started -> test_message_ready -> connected -> healthy
health:        healthy | degraded | failed
```

Authorization failures become `failed`; revoked/expired credentials use the existing credential and setup states and can require reauthorization. Disconnecting stops the connection from active processing but does not delete conversation/audit history or revoke the token at Meta. Reconnect opens the existing record and may require Meta authorization again.

## API surface

Protected, `channels.manage` routes:

```text
POST /v1/channel-platform/connections/:id/whatsapp/embedded-signup/initiate
POST /v1/channel-platform/connections/:id/whatsapp/embedded-signup/complete
POST /v1/channel-platform/connections/:id/whatsapp/assisted-setup
```

The completion payload has no organization ID. The authenticated actor and route connection determine the tenant. The completion response contains connection/authorization status, merchant-safe business/phone labels, ownership, and next action. It contains no token or private secret reference.

Legacy/manual configuration endpoints remain available for existing connections and explicit operator recovery. They are not the normal merchant-managed onboarding path.

## Credential handling

- `META_APP_SECRET` and `META_WEBHOOK_VERIFY_TOKEN` exist only in the API environment.
- the authorization code is transient and is never persisted or logged;
- the returned access token is immediately encrypted with AES-256-GCM through `channel_provider_secrets` and referenced as `dbenc://...` internally;
- AES-GCM additional authenticated data binds the secret to organization, connection, and credential type;
- API views expose only status, expiry, validation time, and safe reference type;
- audit records contain booleans/status/failure codes, never token material or raw Meta responses;
- provider errors are reduced to safe internal codes and merchant guidance.

`CHANNEL_SECRET_ENCRYPTION_KEY` must be stable across restarts. Losing it makes encrypted access tokens unreadable and requires Meta reauthorization.

## Webhooks

Embedded Signup uses the shared callback for the Zidi Meta app:

```text
GET  /v1/runtime/webhooks/whatsapp
POST /v1/runtime/webhooks/whatsapp
```

GET verifies `META_WEBHOOK_VERIFY_TOKEN` in constant time. Legacy/manual connection URLs with `connection_id` remain compatible.

After authorization, Zidi calls `/{WABA-ID}/subscribed_apps`. POST handling is unchanged: it reads `phone_number_id`, resolves exactly one configured connection/organization, verifies `X-Hub-Signature-256` with the Zidi Meta app secret, records the idempotent provider event, and invokes the existing channel-neutral runtime.

## Health and metrics

Embedded Signup records authorization and WABA subscription evidence, but it does not manufacture `healthy`. Until a correctly signed POST webhook is observed, health remains degraded with `signed_webhook_not_observed`. Health still depends on resolvable credentials and observed webhook/test behavior. The Channels page reports connection, authorization, messaging, webhook, last inbound/outbound/provider error, and credential state independently.

Existing provider-neutral metrics remain unchanged: inbound, outbound, failed, delivered, read, conversations, handling mode, response latency, and provider errors.

## Server configuration

```text
CHANNEL_SECRET_ENCRYPTION_KEY=<stable 32-byte key>
WHATSAPP_GRAPH_BASE_URL=https://graph.facebook.com
WHATSAPP_WEBHOOK_PUBLIC_BASE_URL=https://api.example.com
WHATSAPP_SIGNATURE_BYPASS=false

META_APP_ID=<browser-safe Meta app ID>
META_APP_SECRET=<server-only app secret>
META_EMBEDDED_SIGNUP_CONFIGURATION_ID=<Meta configuration ID>
META_GRAPH_API_VERSION=<explicit supported version such as the version configured for the app>
META_WEBHOOK_VERIFY_TOKEN=<server-only shared verify token>
```

Embedded Signup remains disabled when any required value or encrypted channel secret storage is missing. The API and existing manual connections continue to run.

Do not guess a Graph version. Configure a version supported by the deployed Meta app and update it through an intentional provider compatibility change.

## Meta prerequisites

Operators must complete these in Meta, not in repository code:

- configure a Meta app and add the WhatsApp product;
- configure Facebook Login for Business / WhatsApp Embedded Signup and obtain the configuration ID;
- add valid JavaScript SDK domains and OAuth redirect/domain settings for the Admin deployment;
- configure the public HTTPS webhook callback and shared verify token;
- request the permissions and Advanced Access required by Meta for third-party onboarding, including the applicable business and WhatsApp management/messaging permissions;
- complete Meta Business verification and App Review before onboarding external production merchants;
- satisfy phone registration and two-step/PIN requirements where Meta requires them;
- use Meta test users/businesses/numbers while the app is in development mode.

Meta's official references:

- [WhatsApp Business Platform Embedded Signup collection](https://www.postman.com/meta/whatsapp-business-platform/documentation/du6gzjv/embedded-signup)
- [WhatsApp Business Platform documentation](https://developers.facebook.com/docs/whatsapp/)

No real Meta connection is proven until these values are configured and the full hosted flow, WABA subscription, signed inbound event, outbound test, and reply are observed in the target environment.

## Failure and reconnection behavior

| Failure | Behavior |
| --- | --- |
| Meta dialog cancelled | No asset is associated; start again |
| attempt expired | New initiation required |
| code exchange rejected | Safe `authorization_exchange_failed`; no token returned/logged |
| WABA/phone mismatch | Completion rejected before persistence |
| duplicate phone/WABA | Completion rejected across all organizations |
| WABA subscription failure | Authorization not presented as connected; launch again after fixing Meta setup |
| encrypted storage unavailable | Embedded Signup disabled |
| credential expired/revoked | Existing reauthorization/attention state and health behavior apply |
| disconnected in Zidi | Runtime stops accepting the connection; Meta revocation remains a separate operator action |

## Deferred

This phase does not add Instagram, Messenger, billing, subscriptions, Paystack changes, Meta usage pricing, campaigns, advanced template management, automated credit-line handling, or a second messaging/conversation implementation.
