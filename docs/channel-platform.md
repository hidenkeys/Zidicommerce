# Channel Platform and WhatsApp Adapter

Phase J makes external channels first-class, tenant-scoped product objects. Phase K implements the WhatsApp Cloud API adapter. Phase L adds production onboarding and controlled operational validation. Phase M applies those controls to one isolated, explicitly approved WhatsApp pilot. Phase N adds WhatsApp consent, service-window, template, policy, and operational controls without changing the conversation, AI, handoff, or commerce architecture.

## Boundary

The channel flow is:

```text
Provider payload
-> provider adapter
-> normalized inbound event
-> organization and channel connection
-> conversation runtime
-> AI, bot, human handoff, and commerce actions
-> provider-neutral outbound command
-> WhatsApp policy guard when the provider is WhatsApp
-> provider adapter
```

The conversation and commerce engines do not parse provider payloads or require provider account fields. `channelplatform.InboundEvent` and `channelplatform.OutboundCommand` are the stable boundary. The noop adapter remains available for deterministic tests; production WhatsApp HTTP and payload behavior is isolated in `internal/channelplatform/whatsapp`.

## Data model

The existing `channels` row is the `ChannelConnection`. Reusing that table preserves the channel IDs referenced by conversation sessions, messages, outbound deliveries, and commerce notifications.

A connection records:

- provider and merchant-facing display name
- lifecycle status
- `merchant_managed` or `zidi_managed` ownership
- sandbox or production environment
- declared provider capabilities
- creator, connection time, health-check time, and timestamps

Related tenant-scoped records are:

- `channel_provider_accounts`: external business/account identity once an adapter provisions it
- `channel_identities`: phone number, Instagram business account, page, or web widget
- `channel_credential_references`: status and secure-store reference only
- `channel_health_checks`: generic health result, latency, public error code/message, and metadata
- `channel_provider_events`: normalized, idempotent provider activity and processing result
- `channel_metrics_daily`: generic daily message, delivery, conversation, response-time, and error totals
- `whatsapp_contact_states`: hashed/masked consent and 24-hour window state
- `GET /channel-platform/connections/:id/whatsapp/conversations/:conversation_id/policy-context`: tenant-scoped inbox preflight context with masked contact state; it never returns the external customer identifier.
- `whatsapp_message_templates`: local, reviewable mirrors of Meta templates
- `whatsapp_policy_decisions`: idempotent allowed/blocked decisions without recipient identifiers
- `whatsapp_operational_metrics_daily`: WhatsApp-specific send, policy, rate-limit, and provider classifications

Every lookup includes `organization_id`. Composite foreign keys also prevent a related record from pairing one organization's ID with another organization's channel connection. Provider-event uniqueness is tenant-aware, and metrics never aggregate across organizations.

## Ownership

### Merchant-managed

The merchant owns and maintains the provider business assets. A later adapter will authorize assets owned by that merchant and report actions the merchant must take.

### Zidi-managed

Zidi coordinates setup and maintenance. The connection still records ownership explicitly; this does not imply that Zidi owns the merchant's legal provider assets.

Changing ownership is audited. Phase J.1 uses Meta Embedded Signup to exchange a short-lived authorization code, validate selected assets, encrypt the returned token, and subscribe the existing Zidi Meta app to the WABA. Meta still owns account/phone creation, consent, business verification, and phone requirements.

## Lifecycle

New records begin as `not_connected` or `setup_required`. Supported foundation states are:

```text
not_connected -> setup_required -> connecting -> connected -> healthy
                                      |              |          |
                                      v              v          v
                                 disconnected     degraded   requires_attention
```

Any non-archived state can follow its explicit transition rules into `archived`. Archived connections cannot be reactivated. Legacy `draft`, `active`, `inactive`, and `disabled` values remain accepted so existing runtime foreign keys and records are not rewritten in Phase J.

Health checks only promote connections that have reached a connected state. Missing required WhatsApp credentials move an explicitly checked connection to `requires_attention`; they never make a setup-only connection appear connected.

WhatsApp also has a provider-specific onboarding state. It records setup milestones independently of the shared connection status so the runtime boundary remains provider-neutral. A transport can accept signed inbound events while the operator is still completing controlled outbound/inbound validation.

## Credentials

The channel API stores credential references. Phase K resolves `dbenc://`, `env://`, and read-only `legacy://` references; reserved secret-backend schemes remain unavailable until their resolvers are installed. API responses expose:

- credential type
- status
- whether a reference exists
- reference backend type
- expiration and validation times

The reference and secret value are never returned. When `CHANNEL_SECRET_ENCRYPTION_KEY` is configured, Admin can submit a write-only value that is encrypted with AES-256-GCM before storage. Without encrypted storage, Admin accepts only a secure reference such as `env://WHATSAPP_ACCESS_TOKEN`.

The older `channels.secret_config` column remains read-only for explicit migration. The production sender does not read it directly. Migration never deletes legacy values automatically.

## Health and metrics

Health uses `healthy`, `degraded`, or `failed` checks. A failed check on a connected connection changes its lifecycle to `requires_attention`; setup-only records stay setup-only.

Daily metrics are provider-neutral. A zero is displayed only when a metric row exists. With no rows, the UI identifies that no provider reporting exists. Provider-specific diagnostics belong in metadata and must not become core columns unless multiple adapters share the meaning.

## Provider events

Provider events store metadata, not unrestricted raw payloads. They are idempotent by:

- organization, provider, and provider event ID
- organization and normalized idempotency key

Repeated delivery cannot produce a second event record. Processing errors remain visible to permitted organization users without exposing credentials.

## Admin API

Protected routes are under `/v1/channel-platform/connections`:

- `GET /` and `GET /:id`
- `POST /`, `PATCH /:id`, and `DELETE /:id` for safe archival
- `GET /:id/health`
- `GET /:id/metrics`
- `GET /:id/events`
- `GET /:id/credentials`
- `POST /:id/credentials` for secret references, never raw tokens
- `GET /:id/whatsapp` and `PATCH /:id/whatsapp`
- `POST /:id/whatsapp/credentials/rotate`
- `POST /:id/whatsapp/test-message`
- `POST /:id/whatsapp/complete`
- `POST /:id/whatsapp/migrate-legacy`
- `POST /:id/whatsapp/health-check`
- `POST /:id/whatsapp/embedded-signup/initiate`
- `POST /:id/whatsapp/embedded-signup/complete`
- `POST /:id/whatsapp/assisted-setup`
- `GET /:id/whatsapp/contacts` and `PATCH /:id/whatsapp/contacts/:contact_id`
- template list/get/create/update/archive and preview under `/:id/whatsapp/templates`
- `GET /:id/whatsapp/advanced-metrics`
- `GET /:id/whatsapp/policy-blocks`

`channels.view` permits read-only status access. `channels.manage` permits connection, ownership, credential-reference, and archive changes. Backend permission checks remain authoritative.

## Phase K production behavior

The WhatsApp adapter now verifies subscription tokens and POST signatures, normalizes text, interactive replies, and media metadata, sends provider-neutral outbound messages through the Graph API, maps delivery statuses to existing outbound rows, and records tenant-scoped events, metrics, health, failures, and rate-limit state.

See [Meta/WhatsApp Production Adapter](whatsapp-adapter.md) for deployment, credential, security, failure, testing, and operator guidance.

## Pilot boundary

A production pilot uses an isolated API deployment, one tenant-scoped connection, one Meta test identity, and one approved masked recipient. Public health and fail-closed webhook checks happen before Meta configuration. Changing Meta webhook settings and sending a real test message are separate explicit approval gates. A pilot is not healthy until webhook verification, signed inbound evidence, controlled outbound acceptance, matching inbound correlation, and the final connection health check are all backed by stored tenant-scoped evidence.

If a hosting provider is degraded, leave the isolated deployment queued or stopped and keep the existing production service unchanged. Do not redirect Meta to an unverified endpoint or mark an onboarding milestone complete manually.

## Phase P acceptance status

The isolated pilot has external evidence for authenticated Admin access, Meta-signed inbound processing, tenant-scoped contact/session creation, runtime execution from an immutable published snapshot, Graph API outbound acceptance, and a delivered callback. Provider events and channel metrics recorded the path, and post-deployment health returned 200.

The accepted outbound was the automatic reply from the pilot's terminal acceptance workflow. No additional controlled send was performed. A read callback was not observed. The pilot still requires the real merchant workflow to be configured and published before it represents business acceptance; production remained unchanged.

## Deferred after Phase N

Instagram, account/phone provisioning outside Meta's hosted flow, Meta template synchronization/mutation, bulk campaigns, operational opt-out exceptions, media download/storage, and channel billing remain out of scope.
