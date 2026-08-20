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
```

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

The Phase 1 tests do not require a live production database. They cover health behavior, config DSNs, migration runner behavior, token issuing/parsing, auth middleware, role policy, and tenant scope validation.

## Phase Guardrails

Do not add these in Phase 1:

- WhatsApp webhooks
- Paystack checkout
- delivery integrations
- Bing Chun-specific logic
- Bot Builder
- Bot Runtime
- full commerce screens

