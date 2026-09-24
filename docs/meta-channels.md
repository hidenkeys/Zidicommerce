# Instagram and Facebook Messenger channels

Instagram Direct and Facebook Page Messenger share Zidi's Meta transport code,
but they are separate channel connections. Each connection has its own selected
asset, scoped customer identities, capabilities, credentials, health, events,
metrics, and disconnect lifecycle.

## Required Meta configuration

The Meta app must have the applicable products installed before onboarding can
complete:

- Instagram API with Instagram Login for an Instagram professional account.
- Facebook Login for Business and Messenger for Facebook Pages.
- Webhooks for the `instagram` and `page` objects.

Set the following server-side values:

```text
META_APP_ID=
META_APP_SECRET=
META_GRAPH_API_VERSION=v24.0
META_WEBHOOK_VERIFY_TOKEN=
META_ADVANCED_ACCESS=false
```

`META_ADVANCED_ACCESS` must remain `false` until App Review confirms the
permissions used for third-party merchant assets. Standard Access supports only
app-role and test assets. The current audited ZidiHQ app has Standard Access and
has not installed the Instagram, Messenger, or Facebook Login for Business
products.

Provider host variables have official defaults and exist mainly for deterministic
tests. Do not point them at proxies or undocumented APIs.

## Authorization flows

Instagram requests only:

- `instagram_business_basic`
- `instagram_business_manage_messages`

Zidi exchanges the code server-side, obtains a long-lived token, reads the safe
professional-account profile, and subscribes the explicitly selected account to
supported messaging webhook fields.

Facebook requests only:

- `pages_show_list`
- `pages_manage_metadata`
- `pages_read_engagement`
- `pages_messaging`

Zidi records the permissions Meta actually reports, lists Pages where the user
has a messaging task, and requires explicit Page selection. At selection time it
obtains and encrypts the Page token and subscribes that Page. The temporary user
token is retained only as an encrypted refresh/revocation credential.

OAuth state is random, hashed at rest, single use, ten minutes long, and bound to
the organization, connection, and initiating user. Tokens and PKCE verifiers use
the AES-GCM channel secret store; API responses expose only credential status and
expiry.

## Webhooks

Configure both Meta webhook objects with:

```text
GET/POST https://<api-origin>/v1/runtime/webhooks/meta
```

Use `META_WEBHOOK_VERIFY_TOKEN` as the dashboard verification token. Subscribe
only to fields used by Zidi:

- Instagram: `messages`, `messaging_postbacks`, `messaging_seen`,
  `message_reactions`.
- Page: `messages`, `messaging_postbacks`, `message_deliveries`, `message_reads`,
  `message_reactions`.

Every POST must include a valid `X-Hub-Signature-256` HMAC made with the Meta app
secret. Invalid signatures are rejected before event persistence or runtime
dispatch. A later invalid probe increments rejection evidence without erasing a
previously active webhook state.

Provider payloads stop at the adapter. Text, supported attachments, quick
replies/postbacks, reactions, delivery events, and read watermarks become Zidi's
normalized channel events. Provider event IDs and idempotency keys prevent
duplicate conversations or messages.

## Messaging policy

Zidi sends Meta social replies only after that scoped customer identity has sent
an inbound event on the same channel connection and while the 24-hour reply
window remains open. It does not send unsolicited messages. WhatsApp templates
are rejected by the Meta social adapter; quick replies and supported attachments
use their own provider translations.

The normalized event enters the existing runtime, so AI, knowledge retrieval,
commerce, orders, payments, and human handoff do not contain Instagram- or
Facebook-specific branches.

## Recovery and disconnect

- Missing or denied scopes stay visible in the connection capability states.
- Refresh failures mark credentials as requiring reauthorization and move the
  connection to `requires_attention`.
- Reconnect starts a new OAuth session; failed or expired state is never reused.
- Disconnect revokes provider authorization, removes encrypted token material,
  marks account identities disconnected, and disables resolved capabilities.
- Provider errors and webhook evidence are safe classifications only; raw
  payloads, access tokens, and provider error messages are not logged.

Until Advanced Access is approved, the implementation is suitable for automated
tests and controlled app-role/test assets only. It must not be presented as
available for arbitrary merchant accounts.
