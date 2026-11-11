# Phase 2 repository and architecture audit

**2026-11-10 · FAIL for release promotion · proceed to stabilization when approved**

Bing Chun is the confirmed pilot. Designated testers will place test orders, pay using Paystack test mode and rehearse fulfilment without dispatching goods. The audit uses branch `codex/whatsapp-customer-experience` at `e0019e6` and the existing local worktree. The four pre-existing edits and untracked readiness files identified in Phase 1 were preserved. The main website, production, Railway, remote databases and provider accounts were outside the inspection.

## Architecture and implementation

| Boundary | Implementation and evidence | Beta gate |
|---|---|---|
| Admin and field frontends | Separate React/Vite workspaces share packages. Admin covers org, team, catalogue, inventory, stores, inbox, knowledge, journeys, orders and payments. Field portal is separately built and outside the first beta | Verify deployed Admin URLs, desktop/mobile, invitation, and permissions |
| API and auth | Go/Fiber mounts health, public auth and provider callbacks, then JWT-authenticated application routes. Claims carry tenant and role. Services add authorization and store assignments where needed | Enforce payment permissions and test demotion/revocation, role and store boundaries |
| PostgreSQL and jobs | GORM service queries scope records by organization; channel schema includes tenant integrity constraints. Postgres workers claim due jobs with `FOR UPDATE SKIP LOCKED`; handlers must use the job tenant and recheck resource ownership. API and worker share the deployment | Test two tenants across CRUD, relation reads, worker payloads and webhook routing. Row-level security was not found in the SQL migrations. Application predicates and constraints carry this guarantee |
| Commerce | Commerce core owns catalogue, inventory, carts, order transitions, payments and fulfilment. Checkout takes amount and currency from the stored order and attaches tenant/order/customer metadata. Signed Paystack events undergo independent provider verification | Harden mode, authorization, idempotency, result matching and retry behavior before provider UAT |
| AI | Session organization drives retrieval. Security classification, state and active tenant knowledge feed grounding. Deterministic answers cover recognized facts and unknowns. Generated answers are validated and retried. Runtime owns routing and deterministic commerce actions | Exercise provider outage, malformed answers, stale state, unauthorized order lookup and fresh catalogue/knowledge through the deployed runtime |
| Knowledge | Structured entries, legacy FAQs, reviewed document promotion and optional hybrid retrieval are present. Embeddings and vector search default off | Verify active/draft/archive and cross-tenant behavior on PostgreSQL. Keep optional vector infrastructure out of the beta unless separately attested |
| Journey Studio | Published snapshots, bindings, pinned sessions, execution leases and action receipts feed guided chat. Preview simulates writes; controlled record tests use an internal payment mock. Native Flow has separate encrypted exchange and designated sandbox ID gates | Verify real device guided and AI modes. Keep native Flows disabled pending separate Meta acceptance |
| Channels and support | Dedicated WhatsApp webhook/adapter normalizes signed events, records receipts and status, applies channel policy and feeds the runtime. Runtime has staff handoff, claims, replies and support tickets. Instagram/Messenger handlers and TikTok identity OAuth exist separately | Enforce WhatsApp-only in the running release. Verify consent, service window, duplicates, delivery and handoff on designated test assets |
| Deployment | API Docker image contains root SQL migrations and seed profiles and runs as a non-root user. Admin and field each have separate Dockerfiles and nginx configuration. API checks database health and runs migrations during startup | Pin reviewed artifact digests and prevent parallel migration runners. Attest health, backups, rollback and logs in the target environment |

Key paths are [API wiring](../../apps/api/cmd/api/main.go), [HTTP routing](../../apps/api/internal/httpapi/app.go), [tenant scope](../../apps/api/internal/tenant/scope.go), [job claims](../../apps/api/internal/jobs/service.go), [AI orchestration](../../apps/api/internal/ai/orchestrator.go), [journey runtime](../../apps/api/internal/runtime/journey.go), [payment webhooks](../../apps/api/internal/commerce/core/webhooks.go), and [WhatsApp webhook](../../apps/api/internal/channelplatform/whatsapp/handler.go).

## Prioritized remediation backlog

| ID | Priority and classification | Source finding and required verification |
|---|---|---|
| A01 | P0 · Application defect | `RequireSandboxPaymentProvider` is called on the Flow path. Shared `InitializePayment` resolves a tenant Paystack credential or environment fallback and calls the provider without enforcing test mode. Payment configuration can introduce credentials independently of the env default. Add a beta policy at credential admission and the common provider boundary. Prove rejection across guided, admin, existing tenant credentials, callbacks and reconciliation without real charges. |
| A02 | P0 · Application defect | `POST /payments/initialize` and `POST /payments/verify` have no explicit payment permission middleware. Initialize uses the order view and store check. Verify takes the JWT tenant and reference and can mutate payment/order with no payment or store check. The sibling reconciliation route has explicit middleware. Separate authorized human mutations from runtime operations and test denied roles, foreign stores and foreign tenants. |
| A03 | P0 · Application defect | Checkout reads the idempotency row, generates a random reference, calls the provider and inserts the row afterward. Concurrent requests or a crash after provider success can create multiple remote initializations even when the unique index protects local rows. Reserve durable identity before the remote call and test parallel requests and uncertain results. |
| A04 | P0 · Application defect | `verifyPaymentMatches` accepts missing reference, zero amount and empty currency. A successful provider response with omitted fields therefore bypasses field comparisons. Require exact positive amount, reference and currency for Paystack. Test absent and mismatched fields without weakening the internal simulation contract. |
| A05 | P0 · Application defect | The webhook records a failed event on provider verify error or a not-yet-paid response and returns normally. Existing event replay returns stored status before verification. A transient failure can strand the payment until a separate verification path runs. Add safe retry/reconciliation and prove it causes one transition. |
| A06 | P0 · Application defect candidate | Payment webhooks select by provider/reference globally. The schema guarantees uniqueness per tenant/provider/reference, while checkout generates UUID-based references and persists the provider response reference. Prove globally unambiguous routing and credential binding, including a deliberately duplicated reference, before accepting a cross-tenant safety claim. |
| A07 | P0 · Product gap | The binary wires Instagram and Messenger adapters and the public Meta webhook regardless of a dedicated WhatsApp-only release switch. OAuth setup depends on credentials; inbound processing depends on signature and connection state. Require an explicit deny-by-default beta allowlist at inbound, outbound and onboarding boundaries, including persisted connections. |
| A08 | P0 · Application defect candidate | JWT middleware uses the role claim without a membership refresh. Some service checks may constrain specific paths. Verify disabled or demoted staff lose protected access while their original token is still valid and add revocation if required. |
| A09 | P0 · Infrastructure defect candidate | Startup migrations use per-file transactions and filename-only bookkeeping. The runner checks before beginning each transaction and has no migration lock or checksum. Require one migration executor and reviewed SQL hashes. Test clean install, upgrade and concurrent startup using disposable PostgreSQL. |
| A10 | P0 · Configuration defect | Prior staging evidence leaves app secret, verify token and isolated test WABA routing unresolved. Reattest the temporary access token and prove signed inbound/outbound with designated testers. Avoid the production CRM callback. |
| A11 | P1 · Application defect candidate | Request logging includes raw returned error text. GORM SQL logs are parameterized, which does not sanitize every error source. Trace upstream errors, startup DB failures and worker failures with redacted fixtures before releasing logs to operators. |
| A12 | P0 · Product gap | Actual-device order, signed payment, paid-to-complete fulfilment, cross-tenant probes, concurrency, and signed UAT lack evidence for this candidate. Add acceptance evidence from UI through backend and providers. |
| A13 | P0 · Infrastructure and documentation gap | Older Phase R artifact/migration inventory predates later local changes. Current deployed commit, DB state, restore rehearsal, alert delivery and rollback targets require fresh attestation in their respective phases. |
| A14 | P0 · Provider limitation | Prior Meta password reauthentication blocked app-secret reveal. Account-owner action is needed if it persists during staging setup. |
| A15 | P0 · Expected safety behavior | Native Flow defaults off and the pilot requires designated sandbox assets. Verify disabled behavior in staging and retain the separate provider acceptance gate. |

No removal is recommended solely because an adapter or legacy route is outside beta. The normal API chooses the dedicated WhatsApp webhook when wired and otherwise registers the legacy runtime webhook. Check the legacy constructor and deployment paths for bypasses before retiring it. Field service, startup fixtures and adoption tooling should stay outside the beta path. The existing config rejects WhatsApp signature bypass for `APP_ENV=production`. Staging must also explicitly attest bypass is off.

## Migration inventory

Root files below are in application order. `000001` requires pgcrypto. `000017` attempts pgvector and contains an extension-availability path. Tenant-integrity constraints in `000022` and later integration and journey changes warrant an upgrade check against a representative sanitized database. The dedicated PostgreSQL test requires an empty disposable database. This audit did not apply SQL or query any database.

| Version | Migration |
|---|---|
| 000001 | [000001_zidicommerce_foundation.sql](../../migrations/000001_zidicommerce_foundation.sql) |
| 000002 | [000002_commerce_core.sql](../../migrations/000002_commerce_core.sql) |
| 000003 | [000003_organization_team_onboarding.sql](../../migrations/000003_organization_team_onboarding.sql) |
| 000004 | [000004_bot_builder.sql](../../migrations/000004_bot_builder.sql) |
| 000005 | [000005_bot_runtime.sql](../../migrations/000005_bot_runtime.sql) |
| 000006 | [000006_merchant_deployment_runtime.sql](../../migrations/000006_merchant_deployment_runtime.sql) |
| 000007 | [000007_production_reliability.sql](../../migrations/000007_production_reliability.sql) |
| 000008 | [000008_phase7_audit_hardening.sql](../../migrations/000008_phase7_audit_hardening.sql) |
| 000009 | [000009_phase8_self_service_bot_builder.sql](../../migrations/000009_phase8_self_service_bot_builder.sql) |
| 000010 | [000010_phase8_payment_secret_store.sql](../../migrations/000010_phase8_payment_secret_store.sql) |
| 000011 | [000011_phase9_merchant_pilot.sql](../../migrations/000011_phase9_merchant_pilot.sql) |
| 000012 | [000012_customer_default_address.sql](../../migrations/000012_customer_default_address.sql) |
| 000013 | [000013_invitation_email_status.sql](../../migrations/000013_invitation_email_status.sql) |
| 000014 | [000014_field_service.sql](../../migrations/000014_field_service.sql) |
| 000015 | [000015_phase_d_merchant_knowledge.sql](../../migrations/000015_phase_d_merchant_knowledge.sql) |
| 000016 | [000016_phase_d3_document_ingestion.sql](../../migrations/000016_phase_d3_document_ingestion.sql) |
| 000017 | [000017_phase_d4_knowledge_embeddings.sql](../../migrations/000017_phase_d4_knowledge_embeddings.sql) |
| 000018 | [000018_phase_e_conversation_ops.sql](../../migrations/000018_phase_e_conversation_ops.sql) |
| 000019 | [000019_phase_f_commerce_action_engine.sql](../../migrations/000019_phase_f_commerce_action_engine.sql) |
| 000020 | [000020_phase_g_merchant_operations.sql](../../migrations/000020_phase_g_merchant_operations.sql) |
| 000021 | [000021_phase_j_channel_platform.sql](../../migrations/000021_phase_j_channel_platform.sql) |
| 000022 | [000022_phase_j_channel_tenant_integrity.sql](../../migrations/000022_phase_j_channel_tenant_integrity.sql) |
| 000023 | [000023_phase_k_whatsapp_adapter.sql](../../migrations/000023_phase_k_whatsapp_adapter.sql) |
| 000024 | [000024_phase_l_whatsapp_onboarding.sql](../../migrations/000024_phase_l_whatsapp_onboarding.sql) |
| 000025 | [000025_phase_j1_meta_embedded_signup.sql](../../migrations/000025_phase_j1_meta_embedded_signup.sql) |
| 000026 | [000026_whatsapp_provider_status_reconciliation.sql](../../migrations/000026_whatsapp_provider_status_reconciliation.sql) |
| 000027 | [000027_phase_n_whatsapp_policy.sql](../../migrations/000027_phase_n_whatsapp_policy.sql) |
| 000028 | [000028_phase_r0_channel_capabilities.sql](../../migrations/000028_phase_r0_channel_capabilities.sql) |
| 000029 | [000029_phase_r0_shared_oauth.sql](../../migrations/000029_phase_r0_shared_oauth.sql) |
| 000030 | [000030_phase_r0_meta_messaging.sql](../../migrations/000030_phase_r0_meta_messaging.sql) |
| 000031 | [000031_phase_r0_tiktok_identity.sql](../../migrations/000031_phase_r0_tiktok_identity.sql) |
| 000032 | [000032_phase_r1_journey_studio.sql](../../migrations/000032_phase_r1_journey_studio.sql) |

## Environment inventory

Names only. All runtime values were intentionally omitted. The lists reflect the API config loader and frontend source, with deployment attestation deferred. Secret-bearing settings must be entered through protected environment or encrypted tenant credential storage.

```text
AI_EMBEDDINGS_ENABLED
AI_EMBEDDING_DIMENSIONS
AI_EMBEDDING_MODEL
AI_EMBEDDING_PROVIDER
AI_MAX_TOOL_CALLS
AI_MODEL
AI_PROVIDER
AI_VECTOR_SEARCH_ENABLED
AI_VECTOR_SEARCH_THRESHOLD
APP_BASE_URL
APP_ENV
APP_NAME
CHANNEL_SECRET_ENCRYPTION_KEY
CORS_ALLOWED_ORIGINS
DATABASE_HOST
DATABASE_NAME
DATABASE_PASSWORD
DATABASE_PORT
DATABASE_SSLMODE
DATABASE_TIMEZONE
DATABASE_URL
DATABASE_USER
EMAIL_FROM
EMAIL_FROM_NAME
EMAIL_MODE
FACEBOOK_OAUTH_BASE_URL
FIELD_SERVICE_DEMO_PASSWORD
FIELD_SERVICE_DEMO_SEED
FIELD_SERVICE_PILOT_PASSWORD
FIELD_SERVICE_PILOT_SEED_PROFILE
FIELD_SERVICE_PORTAL_URL
GROQ_API_KEY
GROQ_BASE_URL
GROQ_MODEL
INSTAGRAM_API_OAUTH_BASE_URL
INSTAGRAM_GRAPH_BASE_URL
INSTAGRAM_OAUTH_BASE_URL
JWT_ISSUER
JWT_SECRET
JWT_TTL_MINUTES
LOG_LEVEL
META_ADVANCED_ACCESS
META_APP_ID
META_APP_SECRET
META_EMBEDDED_SIGNUP_CONFIGURATION_ID
META_GRAPH_API_VERSION
META_WEBHOOK_VERIFY_TOKEN
MIGRATIONS_DIR
OLLAMA_BASE_URL
OLLAMA_CHAT_MODEL
PAYMENT_PROVIDER
PAYMENT_SECRET_ENCRYPTION_KEY
PAYSTACK_SECRET_KEY
SERVER_PORT
SMTP_FROM_EMAIL
SMTP_FROM_NAME
SMTP_HOST
SMTP_PASSWORD
SMTP_PORT
SMTP_TLS
SMTP_USER
SMTP_USERNAME
TIKTOK_API_BASE_URL
TIKTOK_AUTHORIZE_URL
TIKTOK_CLIENT_KEY
TIKTOK_CLIENT_SECRET
TIKTOK_VERIFIED_ACCESS
VITE_ADMIN_BASE_URL
VITE_API_BASE_URL
VITE_FIELD_PORTAL_URL
WHATSAPP_ADOPT_BOT_ID
WHATSAPP_ADOPT_PHONE_NUMBER_ID
WHATSAPP_ADOPT_TARGET_ORG_SLUG
WHATSAPP_ADOPT_VERIFY_TOKEN
WHATSAPP_FLOWS_ENABLED
WHATSAPP_FLOWS_TEST_PHONE_ID
WHATSAPP_FLOWS_TEST_WABA_ID
WHATSAPP_GRAPH_BASE_URL
WHATSAPP_SIGNATURE_BYPASS
WHATSAPP_WEBHOOK_PUBLIC_BASE_URL
```

Target policy to attest in Phase 4 covers production-grade JWT and database transport, explicit CORS and public callback origin, encrypted channel and payment stores, test-only Paystack, a selected AI provider with fallback, scoped SMTP, signature enforcement and disabled seeds/adoption/Flows. Do not infer server settings from the frontend build.

## Test inventory

- Go has tests for authentication and roles, org and tenant scope, commerce, AI, bot knowledge, channel capabilities and providers, runtime, jobs, migrations, database logging, SMTP and field service. CI runs formatting and the repository `npm run check` entry point. It does not provision a dedicated PostgreSQL database or run deployed provider acceptance.
- AI has deterministic regression, Phase C adversarial and merchant-knowledge suites, plus opt-in live and remote harnesses. The earlier 105/105 provider report is historical evidence.
- Journey Studio browser QA is separately invoked with `npm run test:journeys` and a dedicated fixture manifest. Previous browser runs and local provider mocks do not establish the current device, provider or deployed result.
- Postgres migration and browser acceptance, live providers, concurrency against deployed services, and production operations were outside this audit. See [test evidence](test-evidence.md) for this phase's command results.

## Decision

Phase 2 establishes enough evidence to start Phase 3 after approval. Release promotion fails on the confirmed payment controls and missing deployed acceptance. Stabilize A01–A07 first, then close the remaining confirmed P0 defects or demonstrate the suspected paths are safe. Preserve the existing architecture. Present a reviewable candidate diff and fresh tests before requesting approval to commit, push, merge or tag. Production remains separately gated.
