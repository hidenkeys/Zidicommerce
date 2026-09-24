# Phase R0 provider capability matrix

Audit date: 2026-09-24

This matrix separates provider support from Zidi implementation and from access
granted to the current developer applications. A capability is enabled at
runtime only when the provider supports it, the application has the required
product and access level, the merchant granted the required scopes, and the
selected asset is eligible.

## Evidence levels

- `available`: documented by the provider and available to the current app.
- `review_required`: documented, but third-party merchant use needs provider
  review, advanced access, business verification, or an equivalent approval.
- `restricted`: documented only for an approved partner or limited program.
- `unavailable`: the provider's documented developer products do not expose it.
- `unverified`: official support may exist, but current account access was not
  confirmed during this audit.

## Capability summary

| Capability | WhatsApp Cloud API | Instagram professional messaging | Facebook Page Messenger | TikTok developer platform |
| --- | --- | --- | --- | --- |
| OAuth onboarding | available; Embedded Signup is implemented | review_required; official Instagram Login or Facebook Login for Business | review_required; official Facebook Login for Business | available in official Login Kit; current Zidi app access unverified |
| Profile/account identity | available | review_required | review_required | available through Login Kit/Display API; current app access unverified |
| Inbound customer text | available | review_required | review_required | unavailable in the public developer product catalog |
| Outbound reply text | available, subject to WhatsApp policy | review_required, customer-initiated conversation rules apply | review_required, Messenger policy window applies | unavailable in the public developer product catalog |
| Inbound/outbound media | available | review_required for supported media types | review_required for supported attachments | unavailable as customer messaging |
| Interactive messages | available with provider-specific rules | review_required for supported quick replies/postbacks | review_required for supported quick replies/postbacks | unavailable as customer messaging |
| Templates | available with WhatsApp approval rules | not a WhatsApp-template capability | not a WhatsApp-template capability | unavailable as customer messaging |
| Reactions | provider-dependent | review_required | review_required | unavailable as customer messaging |
| Delivery/read events | available | review_required where supplied | review_required where supplied | unavailable as customer messaging |
| Comments | not in Phase R0 | review_required with separate scopes | outside the Page Messenger scope | unavailable through Login Kit |
| Content publishing | not in Phase R0 | review_required with separate publish scope | out of scope | officially available through Content Posting API; current app approval unverified |
| Commerce/AI/handoff | available through Zidi after normalization | available through Zidi after messaging authorization | available through Zidi after messaging authorization | unavailable for conversations; content publishing must remain separate |

## Current developer-account evidence

### Meta: `ZidiHQ` (`784275937440591`)

- App type: Business.
- App mode: Live.
- Business portfolio: Zidi.
- Installed products: WhatsApp and Webhooks.
- Messenger, Instagram, and Facebook Login for Business are offered but are not
  installed.
- `whatsapp_business_messaging` and `whatsapp_business_management` have Standard
  Access and active API usage; no Advanced Access review is recorded.
- `instagram_business_manage_messages`, `instagram_business_basic`,
  `instagram_manage_messages`, `instagram_basic`, `pages_messaging`,
  `pages_manage_metadata`, `pages_read_engagement`, and `pages_show_list` are
  present at Standard Access. No App Review request is recorded for them.
- The Page webhook object has no callback and all messaging fields are
  unsubscribed.
- The Instagram webhook object has no callback and all messaging fields are
  unsubscribed. The portal notes that Instagram Login webhook configuration is
  managed inside the Instagram product.
- No Meta settings were changed during this audit.

Standard Access is sufficient only for app-role/test assets. Onboarding external
merchant assets requires the applicable Advanced Access and provider review.

### TikTok

- Official public products include Login Kit, Display API, Content Posting API,
  Share Kit, Research API, Data Portability API, and Commercial Content API.
- Login Kit uses OAuth 2.0, short-lived authorization state, server-side token
  exchange, refresh tokens, and provider-approved scopes.
- Content Posting API uses explicit publishing scopes and provider audit for
  unrestricted visibility.
- The official public developer catalog inspected for this phase does not expose
  a customer-service Direct Message inbox/send API.
- The available browser session was not authenticated to TikTok for Developers,
  so Zidi application products, sandbox configuration, redirect URIs, and scope
  approvals remain `unverified`.
- Until account evidence proves otherwise, TikTok messaging is `unavailable` and
  must be represented that way in APIs and UI.

## Repository and staging evidence

- The provider-neutral adapter already normalizes inbound events and outbound
  commands, while WhatsApp owns signature verification, Graph translation,
  policy, and onboarding.
- The current capabilities column is a JSON list with broad legacy names and no
  distinction between provider support, required permission, granted scope, or
  account readiness.
- WhatsApp Embedded Signup already provides secure attempts, tenant binding,
  replay protection, encrypted token storage, and provider-client fakes, but it
  is provider-specific and does not support asset-selection as a separate step.
- The runtime consumes normalized inbound events and outbound commands, so Meta
  adapters can reuse the conversation, AI, commerce, and handoff engines.
- The admin Channels page is WhatsApp-oriented and should be split into shared
  channel summaries and provider onboarding panels.
- Railway staging has one `local_test` channel and one WhatsApp
  `setup_required` connection. There are currently zero provider-account,
  identity, and credential-reference rows.

## Official sources

- Meta Messenger Platform: https://developers.facebook.com/docs/messenger-platform/
- Meta Instagram Platform: https://developers.facebook.com/docs/instagram-platform/
- Meta Webhooks: https://developers.facebook.com/docs/graph-api/webhooks/
- Meta access levels: https://developers.facebook.com/docs/graph-api/overview/access-levels/
- TikTok Login Kit: https://developers.tiktok.com/docs/en/login-kit-overview
- TikTok scopes: https://developers.tiktok.com/docs/en/scopes-overview
- TikTok token management: https://developers.tiktok.com/docs/en/login-kit-manage-user-access-tokens
- TikTok Content Posting API: https://developers.tiktok.com/docs/en/content-posting-api-get-started

