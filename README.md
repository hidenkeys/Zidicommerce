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

Phase 3 adds merchant onboarding and organization access:

- public user registration
- merchant organization onboarding
- organization memberships
- secure, expiring invitations
- email sender abstraction
- store-level staff assignments
- team management screens
- business settings and onboarding progress
- audit log foundation

Phase 4 adds the configurable Bot Builder foundation:

- organization-scoped bots and bot versions
- reusable module, action, and question catalogues
- variables, questions, actions, conditions, integrations, and steps
- validation before publish
- immutable published snapshots
- Bot Builder admin screen
- audit events for bot creation, validation, and publishing

Phase 5 adds the deterministic Bot Runtime:

- published-snapshot-only execution
- persistent conversation sessions and messages
- session variables and safe template resolution
- question validation
- conditions, actions, modules, handoff, and end steps
- commerce-backed action registry
- test runtime simulator
- channel-neutral message models
- WhatsApp webhook adapter with signature verification
- idempotent inbound message processing
- runtime events and structured logging

Phase 6-8 add the first real merchant deployment and self-service bot layer:

- WhatsApp Cloud API outbound adapter
- persisted outbound delivery records
- customer resolution from WhatsApp conversations
- nested runtime module call/return support
- expanded commerce action registry
- Paystack webhook verification and idempotent payment processing
- notification audit records
- generic merchant JSON import tooling
- admin access to merchant import and bot version views
- self-service Bot Builder modules for order, track order, FAQ, complaints, and handoff
- merchant payment configuration with secure secret storage and environment fallback
- immutable published runtime snapshots
- customer/order/payment isolation checks

Phase 9 prepares the production merchant pilot path:

- repeatable/idempotent merchant configuration import
- Bing Chun pilot represented as tenant data, not runtime code
- production readiness checklist in Admin
- conversation visibility for active customer sessions
- persisted complaint tickets
- claimable support handoffs with internal notes
- exact pilot E2E test for store, catalogue, inventory, payment, tracking, and customer isolation

It intentionally does not include LLM orchestration or business-specific bot logic.

See [docs/merchant-deployment.md](docs/merchant-deployment.md) for onboarding, WhatsApp, payment, bot, and local testing guidance.

Operator training (what every Admin module does, and how to run a merchant):

- [docs/operator-guide.md](docs/operator-guide.md)
- [docs/how-to.md](docs/how-to.md)
- [docs/modules/README.md](docs/modules/README.md)

## Repository Structure

```text
apps/
  api/      ZidiCommerce API
  admin/    Commerce merchant admin
  field/    Field-service owner + provider portals
packages/
  shared/   Shared TypeScript types and constants
migrations/ SQL migrations run by the API
configs/    Safe example configuration
docs/       Architecture and development documentation
merchant-config/
  bingchun/ Commerce pilot tenant data
  lagoshome/ Field-service demo tenant notes
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
npm install
npm run dev:admin
```

Field-service (handyman) demo tenant:

```bash
cd apps/api && go run ./cmd/seed-fieldservice
npm run dev:field
```

See `docs/field-service.md`.

The API health endpoint is available at:

```text
GET http://localhost:8080/health
```
