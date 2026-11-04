# Staging deployment — 2026-11-04

Application commit: `81dde00456aa3e085aa693b9980883c92e2af6d3` on `codex/whatsapp-customer-experience`.

Deployment used a clean `git archive` of that commit, excluding unrelated working-tree changes and local secrets. Existing Railway environment variables and Dockerfile settings were retained.

## Target

- Railway project: `zidicommerce-staging` (`3c6283f1-4371-45aa-8763-627e226b8ef0`).
- Environment: `staging` (`ea7de095-ecf9-40bf-bde6-6a321ea847b4`).
- API deployment: `cae87df8-808e-46b0-9e4d-0bbb299606e4`, SUCCESS.
- Admin deployment: `32b89361-3a9a-45c6-8215-13e8732b57ac`, SUCCESS.
- Existing PostgreSQL service restarted with its existing PostgreSQL 18 image and persistent volume; dashboard reports Online.

## Smoke checks

- API `/health`: HTTP 200, `{"checks":{"database":"ok"},"status":"ok"}`.
- Admin `/login`: HTTP 200.
- Main website `https://zidihq.com/`: HTTP 200. No main website or other project services were changed.

Admin: https://zidicommerce-staging-admin-staging.up.railway.app/login

API health: https://zidicommerce-staging-api-staging.up.railway.app/health

## Scope and remaining verification

This releases the existing Journey Studio foundation, Paystack order-reference metadata and experience-mode guidance. It does not certify live WhatsApp Flow acceptance, autonomous ordering or a real Paystack payment round trip. No customer messages or payments were initiated. Login credentials were not changed. No Redis instance was added.

For rollback, retain the deployment IDs above and choose the intended prior application deployment explicitly. Application rollback does not reverse database migrations; check schema compatibility before reverting.
