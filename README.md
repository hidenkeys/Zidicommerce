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

It intentionally does not include LLM orchestration, business-specific bot logic, outbound WhatsApp sending, or production deployment.

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
