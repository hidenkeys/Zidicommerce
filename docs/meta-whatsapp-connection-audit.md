# Meta WhatsApp connection audit

Status: pre-implementation audit for Phase J.1. This document describes the repository state before the Embedded Signup changes in this phase.

## Executive summary

Zidi already has a sound provider-neutral channel boundary. A tenant-scoped `ChannelConnection` owns provider accounts, identities, secret references, health checks, events, and daily metrics. The WhatsApp adapter validates signed webhooks, resolves an organization from the configured phone number, normalizes inbound messages into the runtime, and sends outbound messages through Meta Graph API. None of that should be replaced.

The merchant onboarding experience is the gap. It currently asks an administrator to obtain and paste Meta asset identifiers plus an access token, app secret, and webhook verify token. That duplicates the authorization and asset-selection work Meta Embedded Signup is intended to own. It also treats webhook verification as connection-specific, although a Meta app has a shared webhook product configuration and subscribes individual WABAs to that app.

The recommended migration is additive: preserve the channel and WhatsApp runtime, add a tenant-scoped Embedded Signup initiation/completion boundary, store the returned token with the existing encrypted secret store, associate validated Meta assets with the existing connection, and replace the merchant-facing credential form with `Connect with Meta`. Keep manual references available only as an explicit operator/assisted recovery path.

## Implementation inventory

| Area | State before Phase J.1 | Assessment |
| --- | --- | --- |
| Channel abstraction | Implemented | Reusable unchanged |
| Ownership model | `merchant_managed` and `zidi_managed` are implemented independently of status | Reusable unchanged |
| Lifecycle | Connection, setup, credential, webhook, and health states are implemented | Reuse; map reauthorization/provider errors to the existing credential/setup/attention states |
| Tenant isolation | Protected operations derive the organization from the authenticated actor and scope database queries | Reusable; completion must follow the same rule |
| Secret storage | AES-GCM encrypted database secrets and scoped `dbenc://` / `env://` references | Reusable; access tokens must never enter normal API views |
| WhatsApp inbound | Signed webhook, phone identity resolution, idempotent provider events, runtime normalization | Reusable unchanged |
| WhatsApp outbound | Runtime channel sender -> WhatsApp adapter -> Graph API | Reusable unchanged |
| Health | Connection-specific credential, webhook, signature, inbound/outbound, test, and provider-failure evidence | Reusable; do not synthesize health during signup |
| Metrics | Provider-neutral daily metrics and provider events | Reusable unchanged |
| Merchant onboarding | Manual identifiers and three pasted credentials | Must change for merchant-managed setup |
| Meta Embedded Signup | Missing | Add initiation and completion APIs plus the official browser flow |
| Assisted setup | Ownership exists, but no explicit assisted-request lifecycle | Add a small state model without bypassing Meta |

## Current architecture

The persistent hierarchy is:

```text
Organization
  -> ChannelConnection
     -> ProviderAccount
     -> ChannelIdentity
     -> CredentialReference -> encrypted or environment-backed secret
     -> WhatsAppConfiguration
     -> HealthCheck / ProviderEvent / MetricDaily
```

`ChannelConnection` is provider-neutral and records ownership, environment, lifecycle status, capabilities, and tenant identity. WhatsApp-specific asset identifiers and onboarding evidence live in `channel_whatsapp_configs`. This is the correct ownership boundary.

The current channel statuses cover not connected, setup required, connecting, connected/healthy, degraded, disconnected, requires attention, disabled, and archived. Credential status already represents `requires_reauthorization`; WhatsApp setup state represents credential expiry and provider/webhook failures. Adding another competing lifecycle is unnecessary.

## Current Meta connection flow

The current Channels page creates a WhatsApp connection and presents four setup stages:

1. Paste phone number, WABA, Meta business, display phone, and Graph API version.
2. Paste or reference an access token, app secret, and verify token.
3. copy a connection-specific webhook URL and verify it in Meta.
4. send a test message, receive a reply, complete validation, and run health checks.

The backend encrypts raw credential values before storing references, and API responses expose status/reference type rather than secret material. This is safer than storing plaintext, but the normal merchant should not have to extract and paste these credentials when Embedded Signup can return authorization through Meta.

## Reusable components

- `internal/channelplatform`: tenant-scoped connection CRUD, ownership, lifecycle validation, provider accounts, identities, credential references, audit events, health, provider events, and metrics.
- `internal/channelplatform/secrets.go`: authenticated encryption with organization, connection, and credential type in the additional authenticated data.
- `internal/channelplatform/whatsapp`: existing Cloud API adapter, webhook signature verification, payload normalization, delivery status handling, outbound messaging, health, test-message workflow, and safe operational diagnostics.
- `internal/runtime`: the channel-neutral inbound/outbound boundary and conversation processing.
- Existing `channels.view` and `channels.manage` authorization policy and organization audit log patterns.
- Existing global uniqueness of WhatsApp phone number IDs and provider event idempotency.

## Functionality duplicated from Meta

The merchant UI currently reproduces work that Meta should own:

- Meta login and authorization.
- selection or creation of the business portfolio, WABA, and phone number.
- acquisition of a user access token.
- manual transfer of asset identifiers from Meta screens to Zidi.

Zidi should still validate the returned assets, subscribe its Meta app to the WABA, encrypt the resulting token, record the association, monitor health, and operate messaging.

## Required changes

1. Add server configuration for the Meta app ID, app secret, Embedded Signup configuration ID, Graph version, and shared webhook verify token. Only the app ID/configuration ID/version are browser-safe.
2. Add a short-lived, tenant-scoped signup attempt so completion is idempotent and safe to retry.
3. Add a Meta client boundary for code exchange, token/app validation, asset validation, and WABA app subscription.
4. Add protected initiation and completion endpoints that derive the organization from the authenticated Zidi user, never request an organization ID from the browser, and return only merchant-safe data.
5. Store the access token through the encrypted secret store. Store the app secret and shared webhook token as environment references where available.
6. Add a small assisted-setup lifecycle for `zidi_managed` connections.
7. Replace the merchant credential-entry workflow with the official Meta JavaScript SDK flow. Retain manual setup only as an explicitly labelled assisted/operator path.
8. Support one shared Meta app webhook verification endpoint while preserving the existing phone-number-to-tenant routing for POST events.

## Components that must remain unchanged

- Channel-neutral inbound and outbound contracts.
- Runtime, conversation, AI, bot, handoff, and commerce engines.
- Provider event idempotency and phone-number tenant resolution.
- Existing signed webhook verification and outbound Graph messaging.
- Provider-neutral health and metrics storage.
- Existing legacy/manual connection compatibility.

## Security review

### Existing strengths

- Protected service queries include `organization_id` and connection ID.
- Permission middleware and service-level authorization both protect channel operations.
- Raw credential values are encrypted with AES-GCM and are never serialized.
- Credential views expose configured/resolvable status, not secret references or values.
- Webhook POST requests are signature-checked before normalization.
- Inbound tenant resolution comes from a unique configured phone identity, not request-supplied organization data.
- Provider event idempotency prevents duplicate runtime processing.

### Existing risks and gaps

- Manual credential entry increases accidental disclosure risk in browsers, password managers, screenshots, and support workflows even though the server encrypts the values.
- The UI asks merchants for an app secret, which belongs to the Zidi Meta app in the Embedded Signup model.
- Connection-specific verification URLs do not model the normal shared webhook-product configuration of one Zidi Meta app cleanly.
- There is no server-side authorization-code exchange or verification that a returned WABA contains the returned phone number.
- There is no short-lived signup attempt binding a Meta completion to the authenticated connection.
- Provider errors must be sanitized so a Graph response or request URL cannot expose token material.

## Tenant isolation

The current protected channel flow is tenant-scoped through the authenticated `CurrentUser`. Services use both organization and connection IDs, and cross-tenant connection reads return not found/forbidden according to the existing HTTP error policy. Webhook traffic has no user session; it resolves the configured connection and organization from the globally unique phone number ID before recording an event or invoking the runtime.

Embedded Signup completion must preserve this model. The route connection ID and a server-created signup attempt identify the target; the organization is taken only from the authenticated actor. A phone number already attached to a different organization must be rejected without revealing which organization owns it.

## Webhook architecture

POST webhook processing is already correct in shape:

```text
Meta webhook
  -> decode phone_number_id
  -> tenant-scoped connection lookup
  -> HMAC signature verification
  -> idempotent provider event
  -> WhatsApp normalization
  -> channel-neutral runtime
```

GET verification currently expects a connection-specific query parameter and verify token. Embedded Signup should use the shared verification token configured for the Zidi Meta app. This does not weaken POST isolation because message events continue to resolve by phone number and validate the Meta app signature.

## Outbound messaging architecture

The runtime selects the registered `whatsapp` channel sender. The adapter resolves the connection's access-token reference, posts through Graph API using the configured phone number ID, and records outbound/provider status evidence. Embedded Signup changes how those existing references and identifiers are populated; it does not create another sender.

## Health architecture

Health is evidence-based and connection-specific. It checks asset identifiers, resolvable credentials, webhook/signature observations, test-message readiness, inbound/outbound timestamps, rate limiting, and provider failures. Embedded Signup completion may mark authorization and WABA subscription results, but it must not claim `healthy` until the existing checks have evidence.

## Metrics architecture

`channel_metrics_daily` is provider-neutral and supports inbound, outbound, delivered, failed, read, conversation, webhook-failure, provider-error, API-error, and latency counters. `channel_provider_events` stores normalized provider activity. Embedded Signup should add audit/provider setup events only; no Meta-specific dashboard schema is needed.

## Recommended migration path

1. Deploy the additive migration and backend with Embedded Signup disabled when required Meta values are absent.
2. Configure the Zidi Meta app and shared webhook externally.
3. Enable `Connect with Meta` for merchant-managed connections.
4. Keep existing manual connections running with their current secret references and webhook URLs.
5. Reauthorize an existing merchant connection through Embedded Signup when the merchant chooses to reconnect; do not invalidate working tokens automatically.
6. Use the assisted state for Zidi-managed setup, then complete the same Meta-owned authorization flow when merchant action is required.

## Meta prerequisites outside this repository

Based on Meta's official WhatsApp Business Platform documentation and official Postman collection, operators must complete the following outside Zidi:

- create/configure the Zidi Meta app and WhatsApp product;
- create a Facebook Login for Business / Embedded Signup configuration and record its configuration ID;
- configure an HTTPS webhook callback and verify token for the Meta app;
- subscribe the app to each onboarded WABA after authorization;
- complete Meta Business verification and App Review requirements for release to third-party businesses;
- obtain the required Advanced Access, including `business_management` and `whatsapp_business_management`; messaging also requires the applicable WhatsApp messaging permission;
- satisfy Meta phone-number registration and two-factor/PIN requirements when the selected onboarding variation requires registration;
- configure allowed domains, valid OAuth redirect URIs, and app mode appropriate to test or production users.

These are Meta-controlled prerequisites. Repository code cannot bypass them. Billing/credit-line sharing, pricing, templates, campaigns, Instagram, and Messenger are outside this phase.

## Target architecture

```text
Authenticated Zidi organization
  -> ChannelConnection (ownership + lifecycle)
  -> Meta Embedded Signup (Meta-owned login and asset selection)
  -> tenant-bound completion + encrypted credential
  -> validated Business Portfolio / WABA / phone identity
  -> existing WhatsApp webhook and messaging adapter
  -> existing channel engine
  -> existing conversation runtime
  -> AI / bot / human handoff
  -> commerce engine
```

Meta owns authentication, business/WABA/phone selection, consent, and authorization. Zidi owns the connection record, safe credential custody, WABA subscription, operational status, channel routing, conversations, and commerce behavior.
