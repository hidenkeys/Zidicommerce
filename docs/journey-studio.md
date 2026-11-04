# Journey Studio (Phase R1)

## Architecture

Journey Studio is a merchant-facing editor of the existing bot graph, not a second workflow engine. A journey uses `bots`, `bot_versions`, `bot_questions`, `bot_steps`, `bot_actions`, `bot_conditions` and published snapshots. Version metadata stores journey-level settings; step metadata stores presentation and safety information. There is no second copy of the step graph in version metadata.

The same canonical configuration feeds the guided-chat interpreter, WhatsApp Flow compiler and existing grounded AI/router boundary. Products, prices, stock, stores, carts, orders, customer orders and fulfilment modes are always resolved through tenant-scoped commerce services. Configuration contains source names and filters, never a catalogue or a cached price list.

`journey_bindings` holds the active published version, connection, presentation, trigger and emergency pause. It is separate from immutable snapshots. Changing presentation or rolling back affects new sessions, not sessions already pinned to a version. A bot supports one active connection binding in this foundation; use another journey for another independently configured connection.

## Merchant Workflow

Open **Engagement > Journey Studio**. Templates include ordering, tracking, complaints, support, catalogue browsing, custom questions and a welcome menu.

The desktop builder has steps, selected-step configuration and an interactive preview. Mobile uses Steps/Edit/Preview tabs. Merchants can add optional steps, edit messages, change required answers, configure options and branches, reorder, undo/redo, save, validate and publish. Reordering changes the ordered editor list; explicit branch destinations remain authoritative and are displayed separately.

Mandatory stock/cart/review/confirmation/order/payment-verification operations cannot be removed or bypassed in a publishable commerce path. Protected condition values and commerce mappings are checked server-side. Unknown references, missing routes, unreachable steps, automatic cycles without a customer escape, unsafe variables, markup and excessive payloads prevent publication. Incomplete drafts can be saved to finish later.

Published Zidi versions and published/deprecated provider revisions are immutable. **New draft** clones a version. **Version history** lets permitted merchants view and activate an older publication. **Pause new sessions** leaves existing sessions and existing orders intact. Archiving stops new entries, without deleting commerce records.

Journey graph edits are saved atomically through the Journey Studio API. Legacy per-question/per-action editing endpoints reject Journey Studio versions, preventing partial edits from racing publication or changing a prepared provider revision. Non-journey bots retain their existing editing behavior.

## Routing and AI

Bind each published journey to a connection and intent: order, track order, complaint, catalogue, support, custom or default. A welcome-menu journey uses the default trigger and offers the connection's active published journeys plus grounded AI and human support. Its choices are opaque, revalidated against current bindings and permission boundaries, and enter the same interpreter. Wording and the default menu graph are versioned; target activation is resolved from the current binding when selected.

The welcome menu itself is guided chat. A selected journey can launch its separately published Flow. AI mode delegates to the existing grounded AI callback when available and safely falls back to the deterministic journey when unavailable. No AI retrieval, grounding, prompts or provider implementation is rewritten. AI cannot bypass the server's commerce actions.

## Guided Chat and Sessions

Conversation state, answer history, expected question, opaque options, turn identifiers, version, mode and expiry are persisted. A database execution lease serializes journey turns; inbound-event and action receipts make retries idempotent. These are not in-memory session maps.

Dynamic choices are re-read before accepting a selection. Raw store/product UUIDs and stale choices are not trusted. Numeric replies select the current displayed option only. Long labels use a numbered-text fallback; catalogues over ten items are paginated. Buttons, lists, text, location requests, media requests and Flow-launch messages use the existing channel adapter/capability boundary.

Back is allowed before a commerce write boundary. Cancel and Restart cannot erase a created order or assume its payment status. Session TTL is configurable between 5 minutes and 7 days. Interrupted sessions resume on their pinned snapshot. A crashed worker's lease expires after five minutes; receipts protect committed actions during recovery.

Order confirmation calls the existing transactional commerce service. It rechecks active products, current variant prices and available inventory before committing the order, with an idempotency key. Customer statements such as "I have paid" never set paid status. The payment provider must verify payment before notification steps proceed. A store notification needs `stores.metadata.notification_recipient`; absence safely routes to the failure/handoff path.

## Preview Versus Test Records

Normal **Chat** and **Flow** preview use a persisted, actor-owned snapshot and the same interpreter. They read the merchant's current catalogue but simulate write results inside preview state. They do not create customers, carts, orders, payments, stock movements, complaints, tickets, handoffs or notifications. Scenarios include normal, empty catalogue, unavailable stock, payment failure, handoff and unsupported-channel fallback.

Flow preview uses the compiler's screen IDs and supported components, but is an in-app simulation, not Meta's official renderer. It cannot prove provider acceptance. Switching Chat/Flow retains the same preview session and server navigation. Compile errors stop Flow interaction rather than dropping unsupported steps.

**Controlled record test** is a different, deliberately confirmed operation, restricted to a published journey on a `test` channel and an actor with `bot.publish` and `ai.use`. It creates real *test* customer/cart/order/payment records and changes that test organization's inventory. It uses `SafeTestProvider`, never Meta or a real payment provider, and suppresses notifications. The UI displays **Test records**, not **Preview only**. Run this only against a disposable QA organization. Its payment URL is a local simulation, not a live Paystack test transaction.

## WhatsApp Flow Compiler

The compiler emits Flow JSON 7.3/data API 3.0, stable screen IDs derived from step keys, a supported input screen per question and a terminal finish screen. Messages, conditions and commerce operations between screens execute on the server. It uses Form, TextBody, TextCaption, TextInput, Dropdown, RadioButtonsGroup, DatePicker and Footer.

Numeric bounds and conditional question visibility are enforced by the shared server interpreter and produce warnings. Labels are shortened within component limits while the full question stays visible. Location/media requests and cross-journey entry menus are explicitly rejected in Flow compilation; use guided chat or a text field instead. Screen and branch limits are validated. The compiler emits only forward provider routing edges, omits self-routes for screen refreshes and warns about return navigation. Add-another-item, native Back and payment-refresh branches must be exercised in the designated Meta preview before provider UAT.

Provider validation is authoritative. Local compilation and mock-provider tests must not be described as an accepted or published Meta Flow. Reference: [Meta Flow JSON](https://developers.facebook.com/docs/whatsapp/flows/reference/flowjson/), [official WhatsApp Flow examples](https://github.com/WhatsApp/WhatsApp-Flows-Tools).

## Encrypted Data Exchange

`POST /v1/journey-flows/:connection_id/data` resolves the configured tenant/connection on the server. It requires an enabled pilot, exact designated test WABA/phone, a sandbox WhatsApp connection, valid app-secret HMAC over the raw body and encrypted RSA-OAEP-SHA256/AES-128-GCM payload. Responses use the inverted IV. There is no development signature bypass for this endpoint.

An unguessable expiring Flow token is stored as a hash and binds the request to its organization, connection, conversation and pinned version. Screen and turn validation, exchange receipts and the existing durable outbox protect stale selections and duplicate submissions. Flow completion webhooks are notifications only: their submitted JSON is not trusted as payment proof or replayed as an order action. Flow tokens are redacted from admin message-list responses while durable delivery metadata remains intact.

Errors are sanitized, encrypted where applicable and audited by classification only. Tokens, credentials, ciphertext and submitted answers are not copied into Flow audit records. Provider errors do not expose access tokens or raw provider payloads to the merchant UI. Payloads and calls are bounded and rate-limited.

The R1 Meta pilot requires a Paystack **test-mode** credential before executing order/payment actions. The credential is resolved through the normal tenant-scoped payment boundary; live `sk_live_` credentials are rejected. This guard does not change normal non-Flow payments.

## Provider Lifecycle

Saving a draft does not call Meta. Explicitly authorized provider operations register the endpoint public key, prepare/create a provider draft, upload JSON, refresh validation/preview, publish or deprecate. Cloning a previously published version prepares a new provider revision. Publication checks the compiled hash again under the version lock so a stale uploaded draft cannot become the active form.

Only sandbox connections whose identity matches both designated IDs are allowed. Operations require both `channels.manage` and `bot.publish`. Private keys use the existing encrypted secret store, never the browser. Uncertain provider creation is recorded as failure and not blindly retried; an operator must reconcile an uncertain remote outcome before creating another revision.

Configure, with production assets excluded:

```dotenv
WHATSAPP_FLOWS_ENABLED=false
WHATSAPP_FLOWS_TEST_WABA_ID=
WHATSAPP_FLOWS_TEST_PHONE_ID=
```

Enable only after selecting dedicated Meta test assets, an approved recipient, the existing encrypted WhatsApp app-secret/access-token setup, a reachable HTTPS public API URL and a tenant/global Paystack test credential. The existing server public-URL and Graph settings apply. Never reuse a production WABA/phone merely because its IDs appear in a previous chat.

## API and Permissions

All protected routes are under `/v1` and use the authenticated organization, never an organization supplied in the body.

| Operation | Route | Permission |
| --- | --- | --- |
| Templates/library/list/detail | `GET /journeys...` | `bot.view` |
| Create | `POST /journeys` | `bot.manage` |
| Draft save | `PUT /journey-versions/:id` | `bot.manage` |
| Clone | `POST /journeys/:id/drafts` | `bot.manage` |
| Archive | `DELETE /journeys/:id` | `bot.manage` |
| Validate | `POST /journey-versions/:id/validate` | `bot.view`; only `bot.manage` persists draft validation status |
| Compile | `POST /journey-versions/:id/compile` | `bot.view` |
| Publish | `POST /journey-versions/:id/publish` | `bot.publish` |
| Activate/rollback/pause | `POST /journeys/:id/activate` | `bot.publish` |
| Start/answer preview | `POST /journey-versions/:id/preview`, `/journey-previews/:id/answer` | `bot.view`, actor ownership |
| Controlled test | `POST /journey-versions/:id/controlled-test`, `/journey-tests/:id/answer` | `bot.publish` + `ai.use`, actor ownership |
| Provider lifecycle | `POST /channel-platform/connections/:id/whatsapp/journey-flows/:version/:operation` | `bot.publish` + `channels.manage` |

## Local Verification

Use the repository's normal migration runner against a separate local database named `zidi_r1_qa...`. The full existing migration chain includes pgvector; a plain PostgreSQL image cannot apply that earlier migration. Use a pgvector-enabled PostgreSQL image for full-chain UI QA. No R1 embedding functionality is introduced.

Start the API with `PAYMENT_PROVIDER=test`, `AI_ENABLED=false`, `WHATSAPP_FLOWS_ENABLED=false`, email in log mode and demo/pilot seeds disabled. Give the admin dev server the API's `/v1` base URL. Then create a fresh disposable tenant before each full browser run:

```sh
# From apps/api; DATABASE_URL must be loopback and name zidi_r1_qa... .
JOURNEY_QA_ACKNOWLEDGE=local-test-records go run ./cmd/journey-qa-seed

# From repository root. Playwright and Chromium must be available.
npm run test:journeys
```

The seeder refuses remote/ordinary database names and does not load `.env`. It creates owner/viewer credentials and a long-name commerce fixture, placing the private manifest in `/tmp/zidi-r1-qa-manifest.json` with mode 0600. Do not commit or upload it. For a preinstalled Playwright runtime, set `PLAYWRIGHT_MODULE_PATH` to its `index.mjs`; otherwise install Playwright in the QA environment and its Chromium browser. No mocked frontend requests are used.

Optional overrides: `JOURNEY_QA_ADMIN_URL`, `JOURNEY_QA_API_URL`, `JOURNEY_QA_MANIFEST`, `JOURNEY_QA_ARTIFACT_DIR`. URL overrides must remain loopback. Defaults are admin port 3017, API port 8087 and artifacts `/tmp/zidi-r1-browser`. The suite checks read-only preview before explicitly creating one controlled test order, and writes results JSON and desktop/mobile screenshots.

Run backend commands from `apps/api`, keeping generated Phase C artifacts outside the working tree:

```sh
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./internal/bot
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./internal/runtime
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./internal/channelplatform/...
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./internal/commerce/...
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./internal/ai
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test ./...
AI_PHASEC_ARTIFACT_DIR=/tmp/zidi-r1-ai go test -race ./...
go build -o /tmp/zidi-r1-api ./cmd/api
```

From the repository root: `npm run build:admin` and `git diff --check`. Existing provider tests use local mock listeners, so a sandbox that prohibits listening needs a test-only escalation, not a code change. Live Ollama/Groq evaluation remains opt-in and is not part of normal deterministic tests.

## Troubleshooting and Limits

- Validation errors are expected for unfinished drafts; select each error to repair the affected step. A saved draft is not automatically safe to publish.
- Provider 429/network/validation failures leave the local journey intact. Refresh/reconcile the provider draft rather than assuming it was published.
- A missing Flow deployment safely falls back to guided chat. A disabled Flow pilot returns 503, rather than exposing an insecure endpoint.
- Missing stock or a late price/stock change is checked at the commerce boundary. Failed payment initialization does not mark an order paid.
- Flow component styling in the in-app preview is approximate. Meta validation, device delivery, native back/return routes and a genuine Paystack test link still require the controlled external test.
- No binary uploads, new channels, production deployment or integrated staging deployment are part of R1.
- Expired session/receipt tables currently need the deployment's retention housekeeping. Expiry is enforced at request time; no new cleanup worker was added.
- The provider lifecycle UI intentionally exposes only sanitized status/preview, not raw Meta diagnostic JSON or secrets.
