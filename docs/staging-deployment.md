# Zidi Commerce staging deployment

Date established: 2026-09-01

This is the operator record for the isolated Railway staging stack. Staging is a disposable validation environment. It is not a production replica, a customer channel, or authorization to promote a release.

## Public endpoints

- Admin: `https://zidicommerce-staging-admin-staging.up.railway.app`
- API: `https://zidicommerce-staging-api-staging.up.railway.app`
- Health: `https://zidicommerce-staging-api-staging.up.railway.app/health`

## Railway topology

- Project: `zidicommerce-staging`
- Environment: `staging`
- API service: `zidicommerce-staging-api`
- Admin service: `zidicommerce-staging-admin`
- PostgreSQL service: `zidicommerce-staging-db`

The database is dedicated to this project. The API uses a staging-only JWT secret, channel credential encryption key, and payment credential encryption key. Do not copy these values into production or commit them to the repository.

## Runtime policy

- `APP_ENV=staging`.
- The Admin points only to the staging API.
- CORS permits only the staging Admin origin.
- Groq is the default AI provider. Ollama remains supported by the application but is not deployed in this staging stack because model download, cold-start time, and memory requirements make the small Railway deployment unreliable.
- Embeddings and vector retrieval are disabled. Existing lexical and database grounding remains available.
- Paystack is test mode only. A credential whose public classification is `sk_test_*` is configured; the value must never appear in logs or documents.
- Meta and WhatsApp credentials are absent. WhatsApp is disconnected by default and no live message may be sent from staging.
- WhatsApp signature bypass is disabled.
- Email delivery uses log mode.
- Demo, field-service, pilot, and one-time channel adoption startup flags are disabled.

Provider errors must not prevent database-backed commerce behavior or deterministic AI fallbacks. Changing provider mode requires a deliberate staging configuration change and another smoke run.

## Database and seed policy

The API applied all repository migrations to a fresh PostgreSQL database during startup. The initial tenant was created through the public registration and onboarding APIs, not by importing an old database.

The staging tenant is `Zidi Commerce Staging QA`. It contains only synthetic validation data:

- one owner account;
- one active store;
- one category, product, and variant;
- synthetic inventory;
- one active merchant knowledge entry;
- one synthetic customer, cart, unpaid pickup order, and Paystack test checkout.

No production or former pilot customer data was copied. The test checkout was initialized but not charged, completed, refunded, or fulfilled.

## Login and secret handling

The temporary owner email and password are delivered directly to the authorized operator, not stored in this repository. Rotate the password after handoff. Store any replacement in the approved secret manager and never add it to this document, shell history, screenshots, or issue trackers.

The Admin login is `/login`. A successful login must resolve `/v1/auth/me` to the staging tenant before any mutation is performed.

## Safe smoke verification

The default smoke command is read-only apart from deliberately invalid public webhook probes:

```bash
ZIDI_STAGING_EMAIL='staging.owner@zidicommerce.test' \
ZIDI_STAGING_PASSWORD='from-the-secret-handoff' \
scripts/staging-smoke.sh
```

An existing access token can be supplied as `ZIDI_STAGING_API_TOKEN` instead. The script never prints credentials, tokens, response bodies, database URLs, or record identifiers. It checks API health, authentication, tenant data availability, and fail-closed WhatsApp verification behavior.

## Manual acceptance checks

1. Open the staging Admin URL and sign in with the staging owner.
2. Confirm the organization banner/name is `Zidi Commerce Staging QA`.
3. Confirm the synthetic store, product, inventory, knowledge, customer, and order are visible.
4. Confirm Channels has no WhatsApp connection and no Meta credentials.
5. Confirm the Paystack configuration is explicitly test mode and the synthetic payment remains unpaid.
6. Ask the Assistant for the price of `Staging Test Mug`; it must use the current database price of NGN 2,500.
7. Ask an unknown policy question; it must refuse or use the approved unknown response instead of guessing.

The first AI test-chat request may create a credential-free `local_test` channel connection. That is internal test transport and is not a WhatsApp connection.

## Deploy and rollback

Deploy only from the intended reviewed working tree:

```bash
railway up --service zidicommerce-staging-api --environment staging
railway up --service zidicommerce-staging-admin --environment staging
```

After each deployment, wait for Railway health checks and run `scripts/staging-smoke.sh`. Do not reuse staging commands against the production project.

Application rollback means redeploying a known-compatible prior staging artifact. Database migrations are forward-only; do not run ad hoc down SQL. Because this environment is disposable, destructive database reset is preferable to unsafe manual repair, but it still requires explicit operator approval.

## Reset procedure

1. Export any non-secret diagnostic evidence that must be retained.
2. Confirm the selected Railway project is exactly `zidicommerce-staging` and the environment is exactly `staging`.
3. Delete and recreate only `zidicommerce-staging-db`, or recreate the full staging project when isolation is uncertain.
4. Restore the API database reference and redeploy the API so every migration runs from an empty schema.
5. Register one new staging owner and create one staging organization through the normal APIs.
6. Add only synthetic smoke data, rotate temporary credentials, and rerun local plus public checks.

Never point staging at the production PostgreSQL service or import a production dump for ordinary QA.

## Known limits

- Staging has one QA tenant. Live cross-tenant probing is deferred; deterministic repository tests remain the tenant-isolation gate until a second synthetic tenant is deliberately added.
- With no configured WhatsApp identity, an invalid webhook POST is rejected before per-connection signature validation. Unit and integration tests provide the configured-identity HTTP 403 signature evidence.
- URL crawling, binary document upload, live payments, live Meta setup, and production messaging are not enabled.
- Browser automation may depend on the local Codex browser bridge. Public HTTP checks and the Admin build remain required even when that bridge is unavailable.

## Production boundary

This stack does not change or attest the production API, production database, shared production Admin, Meta callback, Paystack live mode, DNS, backups, monitoring, or incident ownership. Production promotion requires a separate readiness review and explicit approval.

The former `zidicommerce-whatsapp-pilot` service and its dedicated `Postgres-wNhG` test database were retired on 2026-09-01 after staging passed. The former pilot domain now returns HTTP 404. No Meta dashboard setting was changed during retirement; if an external Meta callback still references that retired domain, removing or redirecting it requires a separate explicitly approved Meta operation.
