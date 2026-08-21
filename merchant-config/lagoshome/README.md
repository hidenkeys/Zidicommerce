# Lagos Home Services (demo tenant)

This folder documents the **field-service pilot tenant**. It is configuration and seed data, not a runtime fork.

Do not copy this into generic bot/runtime code. Another org can use the same `SERVICE_BOOKING` module with different pools, fees, and matching weights.

Seed:

```bash
cd apps/api && go run ./cmd/seed-fieldservice
```

See `docs/field-service.md`.
