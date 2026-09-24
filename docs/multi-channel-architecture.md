# Multi-channel architecture and operations

Phase R0 extends Zidi's existing channel platform to Instagram Direct,
Facebook Page Messenger, and the officially available TikTok account
capabilities. It does not create separate conversation, AI, commerce, order,
payment, or handoff systems for each provider.

## Request flow

```text
Provider webhook
  -> provider signature verification
  -> provider adapter and payload translation
  -> normalized inbound/delivery event
  -> tenant and channel resolution
  -> shared conversation runtime
  -> AI/RAG, commerce tools, order/payment flow, or human handoff
  -> capability-aware outbound command
  -> provider adapter and policy enforcement
  -> official provider API
```

Provider payloads stop at the adapter boundary. The runtime sees stable channel
connection, provider account, external customer, conversation, message, media,
reply-context, delivery/read, timestamp, and idempotency fields. The runtime
does not branch on Instagram or Facebook when executing AI, commerce, or
handoff behavior.

TikTok stops after OAuth and profile identity. No TikTok webhook, conversation
inbox, or sender is registered because the audited public developer platform
does not expose a customer-service messaging API.

## Provider and connection model

The provider catalog describes documented support independently from a merchant
connection. Each connection then owns resolved capability rows derived from:

1. documented provider support;
2. provider review or partner restrictions;
3. scopes actually granted by the merchant;
4. eligibility of the selected provider asset; and
5. current credential and connection readiness.

The runtime permits an outbound action only when its resolved capability is
`available`. The admin distinguishes `available`, `setup_required`,
`awaiting_permission_review`, `missing_permission`, `restricted`,
`unsupported_by_provider`, and `unverified`.

Instagram professional accounts and Facebook Pages are always separate channel
connections. A Meta authorization may reveal related assets, but it does not
merge their account, identity, credential, capability, health, or disconnect
lifecycle. TikTok identities are separate for the same reason.

## OAuth onboarding

The shared OAuth service owns start, callback, asset selection, refresh,
reconnect, cancellation, and disconnect. Its invariants are:

- state contains at least 256 bits of randomness and is stored only as a hash;
- sessions expire after ten minutes and are single use;
- each session is bound to organization, connection, provider, and initiating
  user;
- S256 PKCE is supplied where the provider flow requires it;
- authorization codes are exchanged by the API, never by the browser;
- PKCE verifiers, access tokens, and refresh tokens use the AES-GCM channel
  secret store;
- granted and denied scopes, safe identity data, and expiry are persisted;
- API and UI responses expose credential status and expiry, never token values;
- selecting an eligible asset is a separate, explicit merchant action; and
- audit events record start, callback, selection, refresh, failure,
  cancellation, and disconnect without credential material.

The browser stores only the connection ID in session storage while it leaves
Zidi for provider authorization. OAuth state, provider tokens, authorization
codes after callback, and secrets are not persisted in local storage.

## Tenant and identity isolation

Every provider-owned row includes `organization_id` and a channel connection
reference. Composite tenant foreign keys prevent a record from attaching to a
connection in another organization. Service queries resolve both organization
and connection before reading credentials, identities, events, health, or
metrics.

Provider events are idempotent within their tenant/provider boundary. Runtime
message processing is also scoped by organization and channel, so an identical
external message ID on Instagram and Facebook produces two independent records.
An identical provider customer ID remains two independent customer identities
unless a future explicit, audited linking workflow is introduced.

The Meta identity index also prevents the same Instagram account or Facebook
Page from being ambiguously attached to multiple active tenants. Onboarding must
resolve an ownership conflict instead of guessing.

## Instagram and Facebook messaging

Both Meta adapters verify `X-Hub-Signature-256` before persistence or runtime
dispatch. Valid webhook evidence remains active when a later invalid probe is
rejected. Text, supported media, postbacks, reactions, deliveries, reads, and
safe unsupported events are normalized and deduplicated.

Outbound replies use the generic runtime command and the selected provider
identity and encrypted credential. The adapter enforces a customer-initiated
24-hour response window and never treats WhatsApp templates as a Messenger or
Instagram concept. Unsolicited sends are rejected before Meta is called.

The selected asset is subscribed through the official Meta API during asset
connection. A subscription failure prevents the connection from completing.
The current Meta app still requires the applicable products and Advanced Access
before arbitrary merchant assets can use this path.

## Health and monitoring

The Channels page exposes tenant-scoped connection status, selected account,
resolved capabilities, granted/missing permissions, credential status and
expiry, webhook/adapter health, inbound/outbound counts, provider events, and
safe next actions.

Operational evidence is split by source:

- OAuth and permission lifecycle: audit events and credential/capability state.
- Webhook authenticity: last valid signature, last rejection, and rejection
  count on the Meta connection state.
- Provider activity: normalized provider events and daily inbound, outbound,
  failed, delivered, read, conversation, AI/human, response-time, and provider-
  error metrics.
- Rate limits: safe provider error classification and event metadata when Meta
  returns a rate-limit response.
- Commerce and conversion: shared conversation/order/payment audit and runtime
  records retain the originating channel; provider totals must be calculated
  from those database-grounded records.

Do not display delivery, read, response, or conversion values as exact when the
provider did not supply or Zidi did not observe the underlying evidence.

## Troubleshooting

### Authorization cannot start

Confirm the provider client ID/secret is configured server-side, the connection
provider matches the client, encrypted channel credential storage is enabled,
and the exact HTTPS redirect URI is registered with the provider. Localhost HTTP
is accepted only in development.

### Callback expired or was already used

Start a new authorization. Never retry an old state or copy an authorization
code between users, tenants, or connections.

### No eligible assets

Confirm the signed-in user manages the intended professional Instagram account
or Facebook Page and that the relevant product and permissions are enabled.
Related Meta assets are not assumed to be eligible.

### Permission review or missing permission

Compare the connection's required, granted, and missing scopes. Reauthorize to
request a newly approved scope. Keep `META_ADVANCED_ACCESS=false` until Meta App
Review evidence exists; changing the flag is not a substitute for provider
approval.

### Credential expired or revoked

Use refresh when a valid refresh path exists. A failed refresh marks the
credential `requires_reauthorization` and the connection
`requires_attention`. Reconnect starts a new OAuth session and replaces the
encrypted credential references.

### Webhook rejected

Check that Meta signs the exact raw body with the configured app secret and that
no proxy rewrites the payload. Invalid requests are rejected before event
persistence. Do not enable a signature bypass; none exists for Meta social
webhooks.

### Message is not sent

Review the resolved `outbound_text` or `outbound_media` capability, selected
identity, token health, and customer reply window. TikTok messaging is always
rejected until official support and access are implemented.

## Disconnect and rollback

The normal rollback for one provider connection is **Disconnect** in Channels.
It calls the official revoke endpoint where credentials exist, removes encrypted
secret material, marks only that connection's account and identities
disconnected, disables its resolved capabilities, and preserves conversations,
provider events, and audit history.

If application rollback is required before external merchant traffic exists:

1. Stop new onboarding for the affected provider.
2. Disconnect test connections through the API/UI so provider grants are
   revoked.
3. Restore the previous application commit; do not delete shared channel tables
   or historical conversations.
4. Leave additive R0 migrations applied unless a separately reviewed data
   migration proves no R0 records exist. The application is backward compatible
   with the added tables and identity type.
5. Remove provider callback subscriptions only from designated test assets and
   record their previous values before changing the provider portal.

Do not roll back by clearing tokens manually, deleting a Meta-related Facebook
Page/Instagram connection together, or cascading an organization record.

## Privacy and deletion responsibilities

Zidi stores the minimum provider identity, message, attachment reference,
credential metadata, and operational evidence needed for the merchant service.
Raw webhook payloads and credential values are not exposed in diagnostics.
Logs must use safe error codes and classifications rather than message bodies,
tokens, or provider error payloads.

Merchants remain responsible for customer notice, lawful basis, retention, and
responding to provider and data-subject deletion requests. A deletion workflow
must remove or anonymize tenant-scoped customer identity and conversation data
without affecting another provider identity or tenant. Provider revocation and
Zidi data deletion are separate operations: disconnect revokes future access but
preserves audit/history; an approved data-deletion request handles retained
personal data according to policy.

## Related documents

- [Provider capability matrix](phase-r0-capability-matrix.md)
- [Phase R0 implementation plan](phase-r0-implementation-plan.md)
- [Phase R0 validation record](phase-r0-validation.md)
- [Instagram and Facebook setup](meta-channels.md)
- [TikTok foundation and limitations](tiktok-channel.md)
