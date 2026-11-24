# Phase 6 local acceptance

From the repository root, with Node/npm dependencies, Playwright Chromium, Go,
OpenSSL, curl, lsof, and Homebrew PostgreSQL **17** installed:

```sh
scripts/local-acceptance/start.sh
npm run acceptance:local
scripts/local-acceptance/stop.sh
```

The lead runs these commands on a machine that can bind loopback ports. The
implementation sandbox only runs static checks. To use an already installed
Playwright outside the workspace, set `PLAYWRIGHT_MODULE_PATH` to its module path.
No staging secrets, existing accounts, or fixture manifests are needed.

The launcher builds both frontends with `VITE_API_BASE_URL=http://127.0.0.1:4325`;
the clients normalize an origin to the API's `/v1` prefix. Admin previews on
127.0.0.1:4326, Field on 127.0.0.1:4327, and the API on 127.0.0.1:4325. PostgreSQL
17 uses 55450 and a private Unix socket directly under `.local-acceptance/` (a nested socket path exceeds macOS limits).
The launcher refuses occupied ports and existing PID state. API execution uses a
clean environment and an absolute migration directory, so repository `.env`
files and inherited provider secrets cannot configure it.

All data lives under gitignored `.local-acceptance/`. `api.env` has mode 600,
random JWT/encryption keys, development mode, test payments, log email, beta
and WhatsApp signature bypass disabled. No demo seed or external channel is used.
Embeddings/vector search are disabled. There is no disabled chat-provider mode;
Ollama targets unused loopback port 4328 and the suite does not call AI chat.
Keep that port unused while running acceptance.

`LOCAL_ACCEPTANCE_EMAIL_LOG=true` enables a development-only email capture in
private `api.log`. Normal logging mode continues to redact email bodies; staging
and production cannot enable this capture. The runner reads the invitation link
in memory and accepts it using the public API. Passwords are random and retained
only in runner memory; passwords and tokens are never printed by the runner.
Logs contain local invitation credentials and must stay local. Failure screenshots
and a concise `results.json` go under `.local-acceptance/evidence/`; the runner
clears credential inputs before screenshots and does not record traces/HARs.

Every check prints PASS/FAIL, attempts a screenshot on failure, and continues.
Failed prerequisites fail dependent checks explicitly. The process exits nonzero
if any check fails. Each invocation creates a fresh `Acceptance Foods <timestamp>`
organization and owner, so repeated runs do not reuse fixtures.

## Mutation coverage and API contracts

All routes below use `/v1`:

| Check | Real mutations and persisted assertions |
| --- | --- |
| Fresh owner | `POST /auth/register`, `POST /onboarding/organization`, `GET /auth/me`; real Admin login |
| Business knowledge | `POST /document-sources` ingests markdown as `pasted_text`; `GET /document-sources/:id/chunks`, `PATCH /document-chunks/:id`, `POST /document-chunks/:id/approve`, `GET /knowledge-entries/:id`, `DELETE /document-chunks/:id`, `DELETE /document-sources/:id`, `GET /document-sources/:id`; review_required → approved → archived |
| Catalogue forms | UI category/product create, product edit, variant create and price save; `GET/POST /catalogue/categories`, `GET/POST /catalogue/products`, `GET/PATCH /catalogue/products/:id`, `POST /catalogue/products/:id/variants`, `PATCH /catalogue/variants/:id`; persisted values and reloaded form |
| Stores and inventory forms | UI store creation with opening hours, store detail edit, opening stock and adjustment; `GET/POST /stores`, `GET/PATCH /stores/:id`, `GET/POST /inventory`, `PATCH /inventory/:id`; persisted hours/name/quantity and reload |
| Limited staff | `POST /organizations/current/invitations`, logged link, `POST /invitations/:token/accept`, `POST /auth/login`; sign in as store_staff; UI permission state plus 403 for `GET /payment-configurations`, `GET /organizations/current/members`, `POST /channels`, `POST /journey-versions/:id/publish` |
| Field setup | Real owner Field login; `POST /field/pools`, `PUT /field/settings`, `POST /field/providers`, `POST /bots/service-booking`, `GET /bots/:id/versions`, `POST /bot-versions/:id/publish`, `POST /channels` (web only), `POST /runtime/test/start`, `POST /runtime/test/message`; persisted request and matches |
| Field dispatch and completion | Provider Field login; UI decline/accept/status actions; `GET /field/inbox`, `GET /field/requests/:id`, `GET /field/requests/:id/matches`, `POST /field/dispatch/:id/decline`, `POST /field/dispatch/:id/accept`, `POST /field/requests/:id/status`, `GET /field/home`; dispatching → assigned → on_the_way → arrived → in_progress → completed |
| Orders | `POST /customers`, `POST /orders`, test-only `POST /payments/initialize` and `/payments/verify`, `POST /orders/:id/transition`, `GET /orders/:id`; paid → processing → ready → completed and separate cancellation |

Category editing has no API endpoint or UI form. Store opening hours can be set
on creation and survive a detail edit; the existing store edit form cannot change
hours. Document binary upload/extraction is unavailable in this API; markdown
text ingestion exercises its supported deterministic chunk path. These gaps are
not reported as passing edit/upload checks. Field requests have no direct create
endpoint; the supported runtime simulator creates a real booking without fees.
No external WhatsApp, live payment, AI chat, or email delivery is tested.

Stop only terminates recorded PIDs whose start times still match, then stops the
cluster under `.local-acceptance/pg`. It leaves evidence/data for inspection.
After stopping, remove `.local-acceptance/` to reset the disposable cluster.
