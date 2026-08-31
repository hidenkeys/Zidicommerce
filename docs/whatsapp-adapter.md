# Meta/WhatsApp Production Adapter and Onboarding

Phase K connects WhatsApp Cloud API to the Phase J channel boundary. Phase L adds durable production onboarding and controlled operational validation. Phase J.1 replaces normal merchant credential entry with Meta Embedded Signup while retaining explicit legacy/operator recovery. Provider-specific HTTP, signature, payload, delivery-status, and setup behavior lives in `internal/channelplatform/whatsapp`. The conversation runtime, AI, human handoff, and commerce services remain provider-neutral.

## Supported flow

```text
Meta webhook
-> phone_number_id lookup
-> tenant-scoped channel connection
-> HMAC signature verification
-> normalized channel event
-> existing conversation runtime
-> provider-neutral outbound command
-> WhatsApp Cloud API request
-> delivery-status webhook
-> existing outbound delivery row
```

Inbound normalization supports text, interactive button replies, interactive list replies, and image, document, audio, or video metadata. Unsupported message types are recorded as ignored provider events. Media references are metadata only; Phase K does not download or persist Meta media files.

Outbound translation supports text, reply buttons, lists, and basic image, document, audio, or video links. The existing outbound job controls retries and idempotency. Meta request failures never roll back already-committed conversation state.

## Messaging policy boundary

Phase N inserts a network-free WhatsApp policy guard between the provider-neutral outbound command and credential resolution/Graph HTTP. Every runtime reply, staff reply, setup test, and future workflow command that reaches the production adapter is evaluated at this boundary.

- A signed inbound message opens or extends that contact's customer-service window for 24 hours.
- The conversation inbox reads the same tenant-scoped policy state. It shows consent and service-window status and disables free-form composition when the contact opted out or an approved template is required.
- Free-form text, interactive, and media replies require an open window.
- Outside the window, the command must reference a local template record whose status is `approved` and whose required variables are present.
- `opted_out` contacts are blocked. There is no operational bypass in Phase N.
- Missing contact state, closed windows, unapproved templates, invalid variables, and active rate-limit windows are blocked before credentials are resolved and before Meta is called.
- Policy blocks are durable policy decisions and provider events. They are not provider failures and do not degrade channel health.

The contact record stores a connection-specific SHA-256 correlation hash and a masked phone display. Raw provider identifiers remain transient at this layer. Existing delivery/outbound infrastructure still retains the provider recipient where required to deliver the message.

Inbound consent handling is intentionally conservative. `STOP`, `UNSUBSCRIBE`, or `CANCEL` as the whole message opts out; `START`, `YES`, or `SUBSCRIBE` as the whole message opts in. Matching is case-insensitive and accepts surrounding punctuation, but phrases such as “cancel my order” and “yes please” do not change consent.

## Template lifecycle

`whatsapp_message_templates` is a tenant- and connection-scoped local mirror. Supported states are `draft`, `pending`, `approved`, `rejected`, `paused`, `disabled`, and `archived`; supported categories are `utility`, `authentication`, `marketing`, and `service`.

For each template, copy the exact Meta name, language, category, body, variable order, and current status. Marking a local row `approved` does not submit it to Meta and does not prove approval independently. Only use `approved` after verifying that same name and language in WhatsApp Manager. Phase N does not perform Meta template synchronization or Meta-side create/edit/delete operations.

Variables are named locally for merchant readability and sent to Meta as body parameters in the stored schema order. Preview supports both positional placeholders such as `{{1}}` and local named placeholders such as `{{customer_name}}`. Archive a record to remove it from policy-eligible sends without deleting its audit history.

Protected template/contact operations are under the connection's `/whatsapp` route:

```text
GET/POST  /templates
GET/PATCH/DELETE /templates/:template_id
POST      /templates/:template_id/preview
GET       /contacts
PATCH     /contacts/:contact_id
GET       /advanced-metrics
GET       /policy-blocks
```

`channels.view` is read-only. `channels.manage` is required for template and manual-consent changes.

## Data and tenant boundary

`channel_whatsapp_configs` stores non-secret provider identity and operational state:

- organization and channel connection
- phone number ID and display number
- WhatsApp and Meta business account IDs
- Graph API version and webhook status
- last inbound, outbound, verification, provider-failure, and rate-limit timestamps
- onboarding state, signed-webhook observation, test-send time, and matching inbound-test time
- connection method, authorization status/expiry, and assisted-setup state

The approved test recipient is stored only as a SHA-256 correlation hash and a last-four-digit display value. The full number is accepted only for the controlled send request and is not returned by the configuration API.

All configuration, credential, provider-event, health, metric, and delivery queries include the organization and channel connection. A unique non-empty `phone_number_id` maps an inbound webhook to one tenant. Unknown phone IDs fail without entering the conversation runtime.

## Credentials

The adapter requires three credentials:

- `access_token`: authorizes outbound Graph API requests
- `app_secret`: verifies `X-Hub-Signature-256`
- `verify_token`: verifies Meta's GET subscription challenge

Credentials are represented by `channel_credential_references`; the reference itself is never returned by the API. Supported runtime reference schemes are:

- `dbenc://`: AES-256-GCM encrypted value in `channel_provider_secrets`
- `env://ENV_NAME`: strict environment-variable reference
- `legacy://channels/{connection_id}/{credential}`: read-only migration compatibility

`vault://`, `secret://`, and `kms://` remain reserved references and fail clearly until a corresponding resolver is installed. Set `CHANNEL_SECRET_ENCRYPTION_KEY` to a base64, hex, or raw 32-byte key to enable encrypted credential entry in Admin. The key must be stable across API restarts and rotated through an explicit re-encryption procedure.

Legacy `channels.secret_config` is no longer read by the production sender directly. An administrator can explicitly migrate legacy values. Migration encrypts them when encrypted storage is enabled, otherwise it creates read-only legacy references. It does not delete the original values.

## Webhook security

Meta uses the same public endpoint for setup and events:

```text
GET  /v1/runtime/webhooks/whatsapp
POST /v1/runtime/webhooks/whatsapp
```

GET requires `hub.mode=subscribe`, a non-empty challenge, and a constant-time match against a configured verify-token reference. POST requires a recognized phone number ID and a valid HMAC SHA-256 signature over the exact request body. Missing secrets, missing signatures, malformed payloads, unknown phone IDs, and invalid signatures fail closed.

Embedded Signup uses the shared callback and `META_WEBHOOK_VERIFY_TOKEN` configured for the Zidi Meta app. Manual/legacy connections retain a connection-specific callback URL containing `connection_id`. A failed attempt cannot downgrade a webhook that was already verified. Set `WHATSAPP_WEBHOOK_PUBLIC_BASE_URL` to the public API origin so the generated URL is deployable.

`WHATSAPP_SIGNATURE_BYPASS` defaults to `false`. It exists only for controlled local development and configuration loading rejects it when `APP_ENV=production`.

## Idempotency, delivery, and failures

Inbound messages are idempotent by tenant, provider event ID, and normalized idempotency key before runtime processing. Delivery statuses update an existing outbound row only when organization, channel, and Meta message ID all match. Supported statuses are `sent`, `delivered`, `read`, and `failed`.

Outbound provider responses retain only safe metadata such as status code, provider message ID, provider error code/type/subcode, and trace ID. Access tokens, app secrets, verification tokens, authorization headers, and unrestricted raw webhook bodies are not stored in provider events or returned to Admin.

HTTP 429 responses record the rate-limit window when Meta sends `Retry-After`. Other transport and provider errors record a degraded health result and provider-error metric. Lexical error summaries shown to merchants never include credential material or raw Meta payloads.

## Configuration

```text
CHANNEL_SECRET_ENCRYPTION_KEY=<stable 32-byte key, base64 or hex recommended>
WHATSAPP_GRAPH_BASE_URL=https://graph.facebook.com
WHATSAPP_WEBHOOK_PUBLIC_BASE_URL=https://api.example.com
WHATSAPP_SIGNATURE_BYPASS=false
META_APP_ID=...
META_APP_SECRET=...
META_EMBEDDED_SIGNUP_CONFIGURATION_ID=...
META_GRAPH_API_VERSION=...
META_WEBHOOK_VERIFY_TOKEN=...
```

For `env://` references, define the referenced variables in the API process, for example:

```text
WHATSAPP_ACCESS_TOKEN=...
WHATSAPP_APP_SECRET=...
WHATSAPP_VERIFY_TOKEN=...
```

Do not put real values in `.env.example`, source control, logs, audit metadata, provider events, screenshots, or support tickets.

## Setup lifecycle

WhatsApp authorization and onboarding are separate from the provider-neutral transport status:

```text
not_connected -> setup_started -> credentials_added -> webhook_verified
-> phone_identity_verified -> test_message_ready -> test_message_sent
-> inbound_test_received -> connected -> healthy
```

Operational exceptions are `degraded`, `requires_attention`, `disconnected`, `credential_expired`, and `webhook_failed`. Rotating an access token invalidates the outbound/inbound test milestones. Rotating an app secret invalidates signed-webhook and inbound validation. Rotating a verify token requires Meta GET verification again.

Embedded authorization uses `not_started -> pending -> authorized`, with `failed`, `revoked`, and `requires_reauthorization` exceptions. Assisted setup uses its own coordination states and cannot become connected before Meta authorization.

## Operator checklist

1. Configure the Zidi Meta app, Embedded Signup configuration, shared webhook, explicit Graph version, and API secrets outside the repository.
2. Create a WhatsApp connection in Organization -> Channels.
3. Use **Connect with Meta** for merchant-managed authorization, or **Let Zidi help** to enter the assisted setup lifecycle.
4. Confirm that server-side completion validates the WABA and phone and reports authorization without exposing their private identifiers or token.
5. Add an approved test recipient in Meta when using a test number.
6. Send the controlled outbound test from Admin. Confirm Meta accepted it and a delivery event appears.
7. Reply from the same recipient. Confirm Admin shows a verified POST signature and matching inbound test.
8. Complete validation and run the final health check. Confirm provider events and metrics are tenant-scoped.
9. Test duplicate completion, cross-tenant attempts, invalid signature, and unknown phone number ID before production rollout.

## Meta requirements

- A Meta developer app with WhatsApp, Facebook Login for Business / Embedded Signup configuration, valid domains, and public HTTPS webhook.
- The required Meta Business verification, App Review, permissions, and Advanced Access for third-party production merchants.
- An explicit Graph API version supported by the configured app.
- `META_APP_SECRET` and `META_WEBHOOK_VERIFY_TOKEN` stored only in the API environment.
- A WhatsApp Business Account and phone selected or created through Meta's hosted flow.
- An approved test recipient while using Meta's test-number workflow.

Zidi-managed onboarding remains an operator-coordinated process. Zidi does not claim that Meta app, business, WABA, or number provisioning is automated.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| GET verification returns 403 | Use the exact callback URL from Admin and rotate/re-enter the same verify token before retrying in Meta. |
| Signed POST returns 403 | Confirm the configured app secret belongs to the app sending the webhook. Never enable signature bypass in production. |
| Meta send returns 401/403 | Rotate the access token and confirm its app, WABA, permissions, and expiry. |
| Meta send returns 429 | Wait for the recorded rate-limit window and retry once. Do not loop aggressively. |
| Recipient is invalid | Include the international country code and add the number as an approved Meta test recipient where required. |
| Missing phone number ID | Copy the Phone number ID from WhatsApp API Setup, not the WABA ID or display number. |
| Webhook verifies but no inbound event arrives | Confirm the app is subscribed to the WABA and the `messages` webhook field is enabled. |
| Message accepted but not delivered | Review delivery events for recipient eligibility, conversation window, template, or phone-quality restrictions. |
| Free-form send is blocked | Confirm that a signed inbound message from that contact occurred within the last 24 hours. Otherwise select an approved template. |
| Template send is blocked | Confirm local status is `approved`, name/language match Meta, and every required variable has a value. |
| Contact is opted out | Do not override the block. The customer must send a whole-message opt-in keyword or an authorized operator must record valid consent. |
| Policy block appears but provider health is normal | Expected. The guard stopped the request before Meta; review Recent policy blocks in Admin. |
| Channel requires attention | Review the setup state, health issues, credential status, and safe operational event guidance in Admin. |

## Optional live smoke

The script is intentionally excluded from normal tests and requires an explicit send confirmation:

```bash
ZIDI_API_BASE_URL=https://api.example.com/v1 \
ZIDI_API_TOKEN='<admin JWT>' \
WHATSAPP_CONNECTION_ID='<connection UUID>' \
WHATSAPP_TEST_RECIPIENT='<approved E.164 recipient>' \
WHATSAPP_TEMPLATE_ID='<local approved template UUID>' \
WHATSAPP_TEMPLATE_VARIABLES_JSON='{"customer_name":"Test customer"}' \
WHATSAPP_LIVE_SEND_CONFIRM=YES \
bash scripts/whatsapp-live-smoke.sh
```

For a confirmed open service window, omit the template variables and set `WHATSAPP_EXPECT_OPEN_WINDOW=YES`. The script refuses to infer that a window is open.

It prints only health, setup state, a masked recipient, signature status, and safe recent event summaries. It never prints credentials.

## Controlled production pilot

Run a pilot against a separate API deployment and one Meta test number before changing an existing production channel. The pilot deployment may use the same approved tenant database when the purpose is to exercise the real Bing Chun configuration, but it must have its own stable JWT secret, channel encryption key, HTTPS domain, and production-safe configuration. Never copy values through shell history, command arguments, screenshots, or committed files.

Use these gates in order:

1. Confirm the public `/health` endpoint returns 200 and the callback URL is HTTPS.
2. Confirm an invalid GET verify token and an invalid POST signature both return 403.
3. Create the tenant-scoped connection and save provider identifiers plus write-only credentials in Admin.
4. Stop and obtain explicit operator approval before changing the Meta callback URL or webhook subscription.
5. In Meta, set the exact callback URL shown by Admin, enter the same verify token, verify, subscribe the WABA, and enable the `messages` field.
6. Confirm Admin records GET verification and a signed POST observation without displaying any credential.
7. Stop and obtain explicit operator approval before sending a real message.
8. Send only to the approved test recipient. Record Meta acceptance and the provider message ID through safe status fields.
9. Reply from that recipient. Confirm normalization, conversation/session creation, provider events, tenant metrics, and the intended runtime response.
10. Record only status callbacks actually observed. Missing `delivered` or `read` callbacks must remain unclaimed.
11. Complete validation and health only after outbound and matching inbound evidence exists.

The pilot record should state the deployment, organization, connection ownership/environment, masked recipient, webhook result, outbound result, inbound result, observed delivery states, health result, blockers, and test/build results. It must not contain tokens, secrets, full phone numbers, private Meta IDs, raw payloads, or unrestricted logs.

### Manual Meta steps for legacy/operator recovery

Chrome automation is optional. When browser control is unavailable, an operator can safely complete the external steps in Meta App Dashboard:

1. Open the app that owns the selected WhatsApp test number.
2. Open WhatsApp -> API Setup and copy the phone number ID, display number, and WABA ID into the corresponding Admin fields.
3. Add the test recipient in Meta and complete Meta's recipient verification if required.
4. Enter the access token, app secret, and a newly generated verify token through Admin's write-only controls.
5. Open WhatsApp -> Configuration, paste the callback URL shown by Admin, enter the same verify token, and choose Verify and save only after the approval gate.
6. Subscribe the app to the WABA and enable the `messages` webhook field.

Do not paste credentials into chat, tickets, shared documents, or terminal commands. Admin never returns a saved credential value.

### Rollback, revocation, and offboarding

For a failed or completed test-number pilot:

1. Pause or archive the Zidi channel connection so no new outbound work is accepted.
2. Remove or replace the Meta webhook subscription if the pilot endpoint must stop receiving traffic.
3. Revoke the Meta access token at its source, then rotate the Zidi access-token reference so stale evidence is visible.
4. Rotate the app secret and verify token when either may have been exposed. Re-run webhook and signed-inbound validation before reconnecting.
5. Remove the approved test recipient from Meta when it is no longer needed.
6. Preserve redacted provider events and audit records for diagnosis; do not export raw credentials or payloads.
7. Remove the isolated deployment only after traffic is disabled and required evidence has been retained.

Archiving a connection is not token revocation. Revocation must also happen in Meta or the credential's upstream secret system.

## Ownership and maintenance

Merchant-managed means the merchant owns and maintains its Meta business assets and provides Zidi with scoped credentials. Zidi-managed means Zidi coordinates setup and maintenance; it does not transfer legal ownership of the merchant's Meta assets. Both models use the same tenant, credential, webhook, runtime, audit, and health boundaries.

## Advanced metrics

Admin combines provider-neutral daily metrics with WhatsApp policy/contact aggregates: inbound and outbound messages, free-form and template sends, blocked sends, closed-window blocks, open-window contacts, consent totals, delivery/read counts, AI/human handling, handoff rate, response time, provider delivery latency, provider errors, rate limits, invalid recipients, and invalid credentials. Every query is scoped by organization and connection.

## Deferred

Phase N does not add Meta template sync or mutation, bulk campaigns/broadcasts, unrestricted AI outbound initiation, operational opt-out exceptions, Instagram, media download/storage, billing, subscriptions, or a second conversation runtime.

## Phase O controlled pilot record

Phase N was deployed to the isolated WhatsApp pilot only. The production API was not deployed or reconfigured. The release artifact excluded the deferred Meta Embedded Signup work and applied the Phase N policy migration successfully.

Validation completed against the isolated pilot:

- `/health` returned 200 with database connectivity healthy.
- Signature bypass remained disabled.
- The configured GET verification token was accepted without being displayed; an invalid token returned 403.
- A valid HMAC-signed webhook payload was accepted; an invalid signature returned 403.
- Open-window free-form messaging reached a controlled provider stub, while closed-window free-form and opted-out sends were blocked before provider dispatch.
- Opt-in restored eligibility and policy events plus advanced WhatsApp metrics were recorded.
- A local template mirror remained `pending`; no Meta approval or live template delivery is claimed. Approved-template enforcement was tested transactionally with a temporary controlled record that was rolled back.
- No real customer message was sent during this validation.

Remaining external checks are a signed inbound event delivered by Meta, a live send using a genuinely Meta-approved template, and authenticated visual confirmation of the pilot Admin policy pages. These must remain incomplete until valid pilot credentials and explicit live-send approval are available.

### Phase O rollback

If the pilot regresses, redeploy the immediately preceding Phase M deployment for the isolated WhatsApp pilot, confirm `/health` returns 200, and repeat invalid-token and invalid-signature checks. If the pilot endpoint is being stopped, first replace or remove its Meta callback and subscription so Meta does not continue sending traffic to it. Do not redirect rollback traffic to the production API automatically.

## Phase P external acceptance record

Phase P completed against the isolated WhatsApp pilot deployment. The production API was not deployed or reconfigured.

- The supplied pilot owner account authenticated successfully, and the Channels and Conversations pages were verified on desktop and mobile.
- Meta already had the isolated pilot callback configured and the `messages` field subscribed. No Meta setting was changed during this phase.
- Meta reported approved templates. One approved text template was mirrored into the tenant-scoped local catalogue without exposing provider identifiers or credentials.
- The first signed inbound message exposed an invalid pilot bot configuration: the active version had no steps and no immutable published snapshot. It also exposed a replay defect where a failed runtime message was treated as completed on retry.
- Failed runtime messages can now be atomically reclaimed and retried. A regression test covers recovery after the missing snapshot is restored.
- A single terminal acceptance step was added only to the pilot bot and published through the normal immutable-snapshot path.
- A fresh Meta-signed inbound message was normalized and produced a tenant-scoped contact, open service window, channel session, inbound message, and runtime processing record.
- The acceptance workflow automatically sent one reply during inbound validation. Meta accepted it and a `delivered` callback was recorded. No separate controlled live message was sent, and no `read` callback was observed.
- Provider events and connection metrics reflected inbound processing, outbound acceptance, and delivery without storing or displaying unrestricted payloads.
- The isolated pilot `/health` endpoint returned 200 after deployment.

This proves the adapter transport and callback path, not a merchant-ready conversational workflow. Before business pilot use, configure and publish the merchant's real bot steps and repeat the relevant commerce acceptance scenarios. A missing read callback is not a transport blocker, but it must not be reported as observed.

### Phase P rollback

If this pilot repair regresses, restore the preceding isolated-pilot deployment and confirm `/health`, verification-token rejection, and signature rejection. Before stopping the endpoint, replace or remove its Meta callback and subscription. Production must not be used as an automatic fallback.

## Phase Q Bing Chun merchant pilot

Phase Q keeps the original Bing Chun database read-only and copies only merchant-operated data into the isolated WhatsApp pilot tenant. The imported dataset contains 7 stores, 6 categories, 28 products, 28 variants, 28 HTTPS product images, and 196 store inventory rows. Existing pilot placeholders are retained only as inactive records so repeated imports remain idempotent. Customer records, order history, credentials, and provider secrets are not migrated.

The pilot publishes one active immutable Bing Chun workflow snapshot with 43 steps. It covers product discovery, grounded price and stock answers, quantity and variant clarification, order intent, fulfilment guidance, conservative unknown handling, support handoff, and verified test-mode payment initialization.

The WhatsApp connection remains healthy and uses the isolated pilot callback. The Admin and bot share-link paths accept the channel-platform statuses `connected`, `healthy`, `degraded`, and `requires_attention` in addition to the legacy `active` value, matching the statuses already accepted by the runtime boundary. No additional real WhatsApp scenario is sent without explicit operator approval.

Controlled live acceptance covered product discovery, grounded price and stock, unavailable products, tracking, pickup, delivery guidance, returns, support handoff and release, session reset, consent opt-out and opt-in, and explicit pre-payment cancellation. The pilot also exposed and corrected three runtime defects: tracking intent used a value that did not match the published branch; repeated checkout reused an old order because idempotency was scoped to the long-lived conversation instead of the cart/order; and a completed checkout blocked `cancel order` before the commerce cancellation handler. Regression tests now cover cart-scoped order idempotency, active-cart cleanup, and cancellation of an awaiting-payment order after the workflow reaches its terminal step.

Paystack test-mode acceptance is complete for the isolated pilot. The merchant-scoped secret is stored through the write-only encrypted configuration path, masked configuration responses were verified, and the provider readiness test passed. Two genuine Paystack test checkouts were initialized and confirmed through the signed webhook path; matching payments and orders moved to `paid` idempotently. The first transaction exposed a channel-status compatibility defect in payment notifications: healthy channel-platform connections were not selected by the legacy `active`-only query. The query now accepts the runtime's usable WhatsApp states, regression coverage creates a healthy channel, and a second paid transaction produced a completed notification job and a delivered WhatsApp confirmation. This is test-mode evidence only and does not authorize live charges or production promotion.

Rollback is pilot-only: restore the preceding isolated-pilot deployment, select the prior published bot snapshot, and confirm `/health`, invalid-token rejection, and invalid-signature rejection. If the pilot endpoint is retired, replace or remove its Meta callback before taking it down. Never redirect pilot traffic to production automatically, and never mutate the original Bing Chun source as part of rollback.
