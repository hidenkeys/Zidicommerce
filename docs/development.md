# ZidiCommerce Development

## Prerequisites

- Go 1.23+
- Node.js 20+
- npm or pnpm
- PostgreSQL 14+

## Environment Setup

```bash
cp .env.example .env
```

At minimum set:

```text
DATABASE_URL=postgres://postgres:postgres@localhost:5432/zidicommerce?sslmode=disable
JWT_SECRET=replace-with-a-long-random-secret
EMAIL_MODE=log
```

Invitation email uses `EMAIL_MODE=log` locally by default. For SMTP-backed invitations set `EMAIL_MODE=smtp`, `SMTP_HOST=smtp.zoho.com`, `SMTP_PORT=587`, `SMTP_TLS=starttls`, `SMTP_USER` (full Zoho address), `SMTP_PASSWORD` (Zoho app password if 2FA is on), `EMAIL_FROM`, `EMAIL_FROM_NAME`, and `APP_BASE_URL` (the Admin URL, never localhost in production). Production must use `EMAIL_MODE=smtp`; it will not silently fall back to log-only delivery.

## Database Setup

Create a local database:

```bash
createdb zidicommerce
```

The API runs pending SQL migrations from `/migrations` on startup.

If running the API from `apps/api`, use:

```text
MIGRATIONS_DIR=../../migrations
```

If running from the repository root, use:

```text
MIGRATIONS_DIR=migrations
```

## Run API

```bash
cd apps/api
go run ./cmd/api
```

Health check:

```bash
curl http://localhost:8080/health
```

## Run Admin

```bash
cd apps/admin
npm install
npm run dev
```

Admin defaults to:

```text
http://localhost:3000
```

Set API base URL with:

```text
VITE_API_BASE_URL=http://localhost:8080/v1
```

## Tests

API tests:

```bash
cd apps/api
go test ./...
```

AI regression tests for the grounded customer-service layer:

```bash
cd apps/api
go test ./internal/ai -run 'TestAIRegression'
```

See [AI Regression Suite](ai-regression.md) for coverage, live Ollama/Groq smoke tests, and how to add cases.

The tests do not require a live production database. They cover health behavior, config DSNs, migration runner behavior, token issuing/parsing, auth middleware, role policy, tenant scope validation, cart totals, order creation, insufficient inventory, inventory decrement, invalid transitions, payment idempotency, tenant isolation, organization onboarding, invitations, role restrictions, store-level access, disabled member login protection, bot builder validation, tenant isolation, publish snapshots, immutability, bot role restrictions, runtime sessions, questions, validation, conditions, actions, modules, handoff, idempotency, version pinning, and WhatsApp signature verification.

## Commerce API

Core Phase 2 and Phase 3 endpoints:

```text
POST   /v1/auth/register
GET    /v1/organizations
POST   /v1/organizations
GET    /v1/organizations/:id
PATCH  /v1/organizations/:id
GET    /v1/organizations/current

POST   /v1/onboarding/organization
PATCH  /v1/onboarding/progress

GET    /v1/organizations/current/members
POST   /v1/organizations/current/invitations
GET    /v1/organizations/current/invitations
PATCH  /v1/organizations/current/members/:id
GET    /v1/organizations/current/members/:id/stores
PUT    /v1/organizations/current/members/:id/stores
GET    /v1/organizations/current/audit-logs

POST   /v1/invitations/:token/accept

GET    /v1/stores
POST   /v1/stores
GET    /v1/stores/:id
PATCH  /v1/stores/:id
POST   /v1/stores/:id/activate
POST   /v1/stores/:id/deactivate

GET    /v1/catalogue/categories
POST   /v1/catalogue/categories
GET    /v1/catalogue/products
POST   /v1/catalogue/products
GET    /v1/catalogue/products/:id
PATCH  /v1/catalogue/products/:id
POST   /v1/catalogue/products/:id/variants
POST   /v1/catalogue/products/:id/images

GET    /v1/inventory
POST   /v1/inventory
PATCH  /v1/inventory/:id

GET    /v1/customers
POST   /v1/customers

POST   /v1/carts
GET    /v1/carts/:id
POST   /v1/carts/:id/items
PATCH  /v1/carts/:id/items/:item_id
DELETE /v1/carts/:id/items/:item_id
DELETE /v1/carts/:id/items

GET    /v1/orders
POST   /v1/orders
GET    /v1/orders/:id
POST   /v1/orders/:id/transition

GET    /v1/payments
POST   /v1/payments/initialize
POST   /v1/payments/verify

GET    /v1/fulfilment/:order_id
PATCH  /v1/fulfilment/:order_id

GET    /v1/channels
POST   /v1/channels
```

## Bot Builder API

Phase 4 endpoints are configuration-only and require an authenticated organization user. Merchant admins and platform admins can edit and publish; store managers, support agents, and viewers can inspect configuration.

```text
GET    /v1/bot-modules
GET    /v1/bot-actions
GET    /v1/bot-question-types

GET    /v1/bots
POST   /v1/bots
GET    /v1/bots/:id
PATCH  /v1/bots/:id
GET    /v1/bots/:id/versions
POST   /v1/bots/:id/versions

GET    /v1/bot-versions/:id/configuration
GET    /v1/bot-versions/:id/modules
POST   /v1/bot-versions/:id/modules
GET    /v1/bot-versions/:id/variables
POST   /v1/bot-versions/:id/variables
GET    /v1/bot-versions/:id/questions
POST   /v1/bot-versions/:id/questions
GET    /v1/bot-versions/:id/actions
POST   /v1/bot-versions/:id/actions
GET    /v1/bot-versions/:id/conditions
POST   /v1/bot-versions/:id/conditions
GET    /v1/bot-versions/:id/integrations
POST   /v1/bot-versions/:id/integrations
GET    /v1/bot-versions/:id/steps
POST   /v1/bot-versions/:id/steps
PATCH  /v1/bot-steps/:id
POST   /v1/bot-versions/:id/validate
GET    /v1/bot-versions/:id/preview
POST   /v1/bot-versions/:id/publish
```

The preview endpoint returns a configuration preview only. It does not send channel messages, initialize payments, create orders, call delivery providers, or invoke an LLM.

## Runtime API

Phase 5 runtime endpoints:

```text
POST   /v1/runtime/test/start
POST   /v1/runtime/test/message
GET    /v1/runtime/conversations
GET    /v1/runtime/conversations/:id
GET    /v1/runtime/conversations/:id/messages

GET    /v1/runtime/webhooks/whatsapp
POST   /v1/runtime/webhooks/whatsapp
```

The simulator endpoints are authenticated and run the real runtime engine. They are distinct from Bot Builder preview.

WhatsApp webhook verification uses the shared Zidi Meta app verify token for Embedded Signup connections and retains tenant-scoped credential references for legacy/manual connections. POST webhooks require a known `phone_number_id`, `X-Hub-Signature-256`, and a resolvable app secret; requests fail closed when any requirement is missing. Configure `CHANNEL_SECRET_ENCRYPTION_KEY` for encrypted database-backed secrets or use `env://ENV_NAME` references. `WHATSAPP_SIGNATURE_BYPASS` defaults to false and configuration loading rejects it in production. Embedded Signup additionally requires `META_APP_ID`, `META_APP_SECRET`, `META_EMBEDDED_SIGNUP_CONFIGURATION_ID`, `META_GRAPH_API_VERSION`, `META_WEBHOOK_VERIFY_TOKEN`, and a public HTTPS `WHATSAPP_WEBHOOK_PUBLIC_BASE_URL`. See [Meta WhatsApp Connection](meta-whatsapp-connection.md) and [Meta/WhatsApp Production Adapter](whatsapp-adapter.md).

Runtime responses are structured:

```json
{
  "conversation_id": "uuid",
  "status": "active",
  "messages": [
    {
      "type": "text",
      "text": "Welcome"
    }
  ]
}
```

See `docs/runtime.md` for session lifecycle, version pinning, channel abstraction, idempotency, and error handling.

## Phase Guardrails

Do not add these before a later AI/channel integration phase:

- Bing Chun-specific logic
- LLM orchestration
- advanced analytics
- business-specific delivery/rider workflows
