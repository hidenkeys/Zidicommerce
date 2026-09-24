# Phase R0 multi-channel implementation plan

This plan extends the channel platform without creating provider-specific
conversation, AI, commerce, or handoff systems. Each slice is independently
testable and should be committed before the next slice starts.

## Architecture decisions

1. Keep `channelplatform.Adapter` as the message translation boundary and add a
   separate provider-neutral onboarding service. Authorization and asset
   discovery have a different lifecycle from webhook/message processing.
2. Store provider support as a catalog and resolved connection capability grants
   as tenant-owned rows. A resolved capability records availability, reason,
   required scopes, and verification time rather than treating a JSON string as
   universal truth.
3. Store OAuth sessions as hashed, expiring, single-use records bound to
   organization, connection, provider, and requesting user. Store PKCE verifiers
   through encrypted secret references, never in browser storage or API output.
4. Store access and refresh tokens with the existing AES-GCM secret store. API
   views expose only credential type, status, expiry, and safe reference type.
5. Model discovered provider assets separately from selected provider accounts.
   Instagram accounts and Facebook Pages remain separate channel connections
   even when one Meta authorization can discover both.
6. Keep provider webhook payloads inside adapters. Emit the existing normalized
   `InboundEvent` and `DeliveryUpdate`, adding only shared reply/attachment/event
   fields that are required by more than one provider.
7. Enforce capabilities before outbound construction. AI/runtime receives the
   resolved capability set and cannot offer unsupported interaction types.
8. Implement TikTok OAuth/profile foundations only. Do not register a messaging
   adapter or conversation actions until an official API and current app access
   are proven.

## Delivery slices

### R0.1: capability model

- Add provider catalog definitions for WhatsApp, Instagram, Facebook Messenger,
  and TikTok.
- Add tenant-scoped resolved capability rows and safe API views.
- Resolve capabilities from provider support, required scopes, granted scopes,
  selected asset, and connection state.
- Add outbound capability guards and migration tests.

### R0.2: shared OAuth and asset selection

- Add secure state, PKCE S256, expiry, tenant/user binding, one-time callback
  claims, cancellation, retries, and audit events.
- Add injectable provider clients for authorization URL, code exchange, refresh,
  revocation, and asset discovery.
- Add encrypted access/refresh token storage and safe scope/expiry metadata.
- Add start, callback, asset-list, asset-select, reconnect, and disconnect APIs.

### R0.3: shared Meta client and Instagram

- Reuse one Meta authorization client while keeping Instagram account records
  separate from Facebook Pages.
- Discover eligible professional accounts, validate granted messaging scopes,
  select one account, and persist its safe identity.
- Add official signature verification, webhook normalization, text-first sends,
  safe unsupported-event handling, health evidence, and deterministic fakes.

### R0.4: Facebook Page Messenger

- Discover Pages the authorized user can manage and require explicit selection.
- Subscribe the selected Page through the official API client boundary.
- Normalize text, attachments, postbacks, reactions, delivery, and read events.
- Add policy-window-aware outbound replies and provider health evidence.

### R0.5: TikTok foundation

- Add Login Kit authorization, profile discovery, refresh/revoke handling, and
  scope-aware capabilities behind an injectable client.
- Represent messaging as unsupported in the API and UI.
- Keep Content Posting API as a separately gated optional marketing capability;
  it must not enter conversations.

### R0.6: admin experience

- Split the current Channels page into shared connection summary/onboarding
  components and provider-specific panels.
- Show status, selected account, granted/missing scopes, resolved capabilities,
  credential and webhook health, activity, expiry, safe errors, and next action.
- Implement guided connect, resume, select, reconnect, test, and disconnect
  states. Never render credential material.

### R0.7: integration, observability, and documentation

- Verify Instagram and Facebook events enter the existing runtime and exercise
  AI, commerce actions, and handoff without provider conditionals.
- Add safe onboarding, refresh, permission, webhook, inbound/outbound, error,
  AI, handoff, and conversion metrics where evidence is available.
- Update architecture, setup, troubleshooting, disconnect, privacy, and rollback
  documentation.

## Security invariants

- Every protected query includes organization and connection identity.
- Every OAuth session is random, hashed at rest, short-lived, single-use, and
  bound to the authenticated user who started it.
- Callback input never supplies an organization ID.
- Tokens and PKCE verifiers are encrypted server-side and excluded from JSON,
  logs, audit metadata, provider events, and URLs.
- Webhook signatures are verified before event persistence or runtime dispatch.
- Provider event identity is unique within the tenant/provider boundary.
- Cross-channel customer identities remain separate unless a future explicit,
  audited linking workflow is introduced.
- Disconnect archives/revokes only the selected connection and cannot cascade to
  another tenant or related Meta asset.

## Validation gates

Each slice must pass focused tests plus:

```text
go test ./internal/channelplatform/...
go test ./internal/runtime
go test ./internal/ai
go test ./internal/commerce/...
go test ./...
go test -race ./...
go build ./cmd/api
npm run build:admin
git diff --check
```

Provider-client tests are network-free. Live checks use only app-role, sandbox,
or designated test assets and are reported separately from automated results.

## External blockers recorded before implementation

- Meta Messenger and Instagram products are not installed on `ZidiHQ`.
- Required Meta messaging permissions are only at Standard Access and have no
  App Review request, so external merchant onboarding is not available yet.
- Page and Instagram messaging webhook callbacks/subscriptions are not configured.
- TikTok developer-account products and scopes are unverified because the
  available session requires login.
- No external provider setting will be changed and no staging deployment will be
  performed in Phase R0 without a separate, explicit validation step.

