# TikTok channel foundation

Zidi's TikTok integration is an account-connection foundation, not a customer
messaging channel. The public TikTok developer products verified for Phase R0 do
not expose a customer-service Direct Message inbox or send API. Zidi therefore
does not register a TikTok runtime sender, webhook handler, conversation inbox,
or messaging capabilities.

## Implemented support

The integration uses TikTok Login Kit to:

- authorize with OAuth 2.0 and S256 PKCE;
- exchange the authorization code on the API server;
- request only `user.info.basic`;
- record the scopes TikTok actually granted;
- retrieve `open_id`, safe profile identity, username, and avatar URL;
- store access and refresh tokens in the encrypted channel credential store;
- track access-token and refresh-token expiry separately;
- refresh credentials, reconnect, revoke access, and disconnect;
- create a tenant-scoped `tiktok_account` channel identity; and
- report profile access and OAuth readiness through the shared capability model.

Content Posting API support is not implemented. It is a separate marketing
capability and must not be routed into customer conversations if it is added
later.

## Configuration

Create a TikTok developer application with Login Kit, configure the exact Zidi
OAuth callback URL, and set these API environment variables:

```text
TIKTOK_CLIENT_KEY=
TIKTOK_CLIENT_SECRET=
TIKTOK_VERIFIED_ACCESS=false
TIKTOK_AUTHORIZE_URL=https://www.tiktok.com/v2/auth/authorize/
TIKTOK_API_BASE_URL=https://open.tiktokapis.com
```

Keep `TIKTOK_VERIFIED_ACCESS=false` until the Zidi developer application,
sandbox or production mode, redirect URI, enabled products, and approved scopes
have been inspected in an authenticated TikTok for Developers session. Endpoint
variables have official defaults and exist for deterministic provider-client
tests; do not replace them with unofficial services.

## OAuth and credential security

TikTok uses the shared channel OAuth lifecycle. The state is random, hashed at
rest, single use, ten minutes long, and bound to the organization, connection,
and initiating user. The PKCE verifier and provider tokens are encrypted with
AES-GCM and never returned to the browser. Authorization codes are exchanged
server-side with the client secret.

Selecting the discovered account creates separate provider-account and channel-
identity records for the current organization. The provider `open_id` is not
used to merge a TikTok user with WhatsApp, Instagram, or Facebook identities.

## Recovery and disconnect

- A failed or expired OAuth session can be retried with a new session; state is
  never reused.
- Token refresh rotates both access and refresh credentials and persists the new
  expiry values.
- Refresh failure marks the credential and connection as requiring attention.
- Reconnect starts a new provider authorization flow.
- Disconnect calls TikTok's revoke endpoint, removes encrypted token material,
  disconnects the provider account and identity, and disables resolved
  capabilities.
- API errors are reduced to safe status/code classifications. Tokens and raw
  provider payloads must not be logged or exposed in diagnostics.

## Confirmed limitations

`inbound_text`, `outbound_text`, media messaging, interactive messages,
reactions, delivery/read events, commerce conversations, and human handoff are
all reported as `unavailable` for TikTok. The Channels UI must say:
"Messaging unavailable through the current TikTok API."

Adding TikTok messaging later requires all of the following evidence before a
runtime adapter can be enabled:

1. Official TikTok documentation for a customer-service messaging API.
2. The required product and scopes approved for the Zidi developer application.
3. A documented webhook signature and replay-protection mechanism.
4. Sandbox evidence for inbound normalization and policy-compliant replies.
5. The same tenant-isolation, idempotency, capability, health, and recovery tests
   used by the existing channel adapters.

The TikTok developer portal was not authenticated during the Phase R0 audit, so
the current application's configuration and approvals remain unverified. Do not
present TikTok messaging, publishing, or unrestricted merchant onboarding as
available until provider evidence is recorded.
