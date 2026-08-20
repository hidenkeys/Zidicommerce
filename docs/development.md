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

Invitation email uses `EMAIL_MODE=log` locally by default. For SMTP-backed invitations set `EMAIL_MODE=smtp`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `EMAIL_FROM`, and `APP_BASE_URL`.

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

The tests do not require a live production database. They cover health behavior, config DSNs, migration runner behavior, token issuing/parsing, auth middleware, role policy, tenant scope validation, cart totals, order creation, insufficient inventory, inventory decrement, invalid transitions, payment idempotency, tenant isolation, organization onboarding, invitations, role restrictions, store-level access, and disabled member login protection.

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

## Phase Guardrails

Do not add these before the bot phase:

- WhatsApp webhooks
- Bing Chun-specific logic
- Bot Builder
- Bot Runtime
- full commerce screens
