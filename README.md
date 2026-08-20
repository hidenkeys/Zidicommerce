# ZidiCommerce

ZidiCommerce is the commerce platform being separated from the existing Zidi marketing, rewards, surveys, and CRM product.

Phase 1 establishes the foundation only:

- Go/Fiber/PostgreSQL API
- explicit SQL migrations
- organization tenant boundary
- authentication and authorization foundations
- commerce domain boundaries
- React admin application shell
- shared package foundation
- documentation and basic tests

Phase 2 adds functional commerce core APIs:

- organization management
- stores, hours, and fulfilment modes
- catalogue categories, products, variants, prices, and images
- store-level inventory
- customers and carts
- transaction-safe order creation
- controlled order state transitions
- payment provider abstraction
- fulfilment and channel foundations
- operational admin screens

It intentionally does not include WhatsApp, Paystack checkout, Bot Builder, Bot Runtime, Bing Chun-specific logic, or production deployment.

## Repository Structure

```text
apps/
  api/      ZidiCommerce API
  admin/    ZidiCommerce Admin shell
packages/
  shared/   Shared TypeScript types and constants
migrations/ SQL migrations run by the API
configs/    Safe example configuration
docs/       Architecture and development documentation
```

## Quick Start

```bash
cp .env.example .env
cd apps/api
go test ./...
go run ./cmd/api
```

In another terminal:

```bash
cd apps/admin
npm install
npm run dev
```

The API health endpoint is available at:

```text
GET http://localhost:8080/health
```
