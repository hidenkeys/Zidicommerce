# Organization hard delete

This staging/test cleanup permanently deletes an organization and its database data.
It is irreversible. Take and verify a backup first; it cannot be undone in the UI.
It does not revoke credentials at external providers or delete external provider data.

## Guards and authorization

Set `ORGANIZATION_HARD_DELETE_ENABLED=true` explicitly. Other values do not enable it.
`APP_ENV=production` rejects this flag at startup; the service independently refuses
production requests. Disabled/production requests return 403. Keep the flag off when
cleanup is finished. The local acceptance stack explicitly enables it.

Only an active owner membership (`is_owner`) of the target organization or an active
user with the global `platform_admin` role can delete. The service rechecks these
records inside the transaction. A merchant administrator without ownership cannot.
The authenticated DELETE route is limited to five requests per minute per IP.
The body must contain the exact current slug, otherwise the API returns 400.

## Backup first

Use the same database as the API and store the backup securely:

```sh
umask 077
pg_dump --dbname="$DATABASE_URL" --format=custom --file=before-organization-delete.dump
pg_restore --list before-organization-delete.dump >/dev/null
```

Verify restoration in an isolated database before relying on the backup. The dump
contains credentials and tenant information; restrict access and retention.

## Business settings

Open **Settings → Business → Danger zone**. The API capability endpoint hides this
section unless both the deployment guard and the current user's authority allow it.
Choose **Preview deletion**, review the per-table counts, then choose **Delete
organization…** and type its slug in the confirmation dialog. Successful deletion
clears the local session and redirects to `/login`. Shared users can still sign in
to their surviving organization; platform administrator accounts are retained.

## API usage

Use an owner or platform administrator bearer token. Dry run uses the same selection,
locking, counts, and safety checks as deletion, but performs no row deletion.

```sh
# Example staging origin; set these to your authorized staging workspace.
API=http://localhost:8080
ORG_ID=00000000-0000-0000-0000-000000000000
ORG_SLUG=test-organization

curl --fail-with-body "$API/v1/organizations/$ORG_ID/hard-delete-capability" \
  -H "Authorization: Bearer $TOKEN"

curl --fail-with-body -X DELETE "$API/v1/organizations/$ORG_ID?dry_run=true" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data "{\"confirm_slug\":\"$ORG_SLUG\"}"

# Irreversible: run only after reviewing the preview and verifying the backup.
curl --fail-with-body -X DELETE "$API/v1/organizations/$ORG_ID" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data "{\"confirm_slug\":\"$ORG_SLUG\"}"
```

The response `data` contains `counts` (table → rows selected for deletion),
`total_rows`, and `dry_run`. Counts are recomputed for every request and may change
between preview and deletion. Retained user pointer updates are not deleted rows.
A missing organization returns 404 to an authorized caller. Existing exclusive-user
sessions stop working because authentication rechecks the user/membership records.

## Deletion scope and order

`scripts/schema/organization-delete-plan.py` parses the migration DDL and emits the
reviewable child-before-parent order. At this revision it covers **92 tables**,
including **90 tables with `organization_id`**, the organization itself, and
indirect password recovery records. Regenerate the baseline after schema changes:

```sh
python3 scripts/schema/organization-delete-plan.py
python3 scripts/schema/organization-delete-plan.py --go > apps/api/internal/commerce/core/hard_delete_schema_generated.go
gofmt -w apps/api/internal/commerce/core/hard_delete_schema_generated.go
```

The service validates and refines this baseline from the actual applied PostgreSQL
catalog (`pg_constraint`, including composite FKs, and primary keys). Every table
with `organization_id` is selected directly. Foreign keys select indirect descendants
through joins, repeatedly until no further rows are found. Primary-key snapshots
support composite keys and survive SET NULL updates. Dependencies are traversed
child first, excluding SET NULL edges, and the organization is deleted last.
Unrecognized non-SET-NULL cycles or missing primary keys cause a refusal.

This includes membership/invitation rows, encrypted payment/channel secrets,
connections/webhooks, conversations/runtime sessions, jobs/leases, audit logs,
journeys/bots, document sources/chunks and their stored embedding columns.
Global users are deleted only if linked to the target organization and they have
no membership in any other organization and are not platform administrators.
Retained users' legacy `organization_id` pointers are cleared.

All discovery, snapshots, counts and deletes run in one transaction. Deterministic
SHARE ROW EXCLUSIVE locks on public tables block concurrent writes and schema changes;
there is a five-second lock acquisition timeout. This is a staging maintenance
operation and can briefly block other tenants' writes. A boundary check refuses
rather than cascading into or updating surviving records (409), including malformed
cross-tenant references. No constraints or triggers are disabled. Any failure rolls
back every change. Only after commit does a structured server log record actor ID,
organization ID, slug, and total deleted rows; no user names, emails or credentials.

## Verification

Go guard/authorization/schema-catalog unit tests run in the normal suite. The full
PostgreSQL test requires an explicitly supplied **dedicated empty database**:

```sh
cd apps/api
ZIDI_HARD_DELETE_TEST_DATABASE_URL='host=/path/to/socket dbname=dedicated_test user=tester sslmode=disable' \
  go test ./internal/commerce/core -run TestOrganizationHardDelete -v
```

It applies real migrations and exercises indirect children, composite keys, dry-run
invariance, shared users, unrelated organization data, and an information_schema
check across every tenant-column table. Never point it at a live database.
The final commerce acceptance check previews then deletes the organization created
by that run, checks login/session denial, and checks every tenant column via the
fixed local acceptance PostgreSQL instance. The acceptance suite requires the local
stack and Playwright; it is not run against staging.

## Operator CLI: one organization or a staging batch

The operator command uses the **same `HardDeleteOrganization` service method** as
HTTP, with the same environment guards, exact current slug, row selection, counts,
transaction, and boundary checks. Direct database credentials authorize this trusted
operator process; no owner token or impersonated user is needed. The HTTP service
never receives this authority. Post-commit logs identify `actor_kind=database_operator`
with a nil user ID, rather than attributing the operation to an owner.

**Back up and verify restoration first** using the backup procedure above. Ensure
`APP_ENV=staging` and `ORGANIZATION_HARD_DELETE_ENABLED=true` are configured on the
intended database. The command loads `.env` and `internal/config` just like the API,
including `DATABASE_URL` and the API's usual configuration validation (for example,
`JWT_SECRET` is still required). It does not run migrations or contact providers.

The API Dockerfile builds both binaries; the deployed container includes
`/app/org-hard-delete`. Run it from an operator shell in that container:

```sh
# Safe default: print every organization's ID/slug and per-table deletion counts.
/app/org-hard-delete --all
# Equivalent explicit preview:
/app/org-hard-delete --all --dry-run=true

# Only if the preview selects exactly 11 organizations and backup is verified:
/app/org-hard-delete --all --execute --confirm "DELETE 11 ORGANIZATIONS"

# Select by exact slug or ID; repeat --org for a batch.
/app/org-hard-delete --org test-company --org 00000000-0000-0000-0000-000000000000
/app/org-hard-delete --org test-company --execute --confirm "DELETE 1 ORGANIZATIONS"
```

For a source/Railpack deployment that does not use `Dockerfile.api`, explicitly build
both binaries in the API module and retain both in the runtime image:

```sh
cd apps/api
go build -o zidicommerce-api ./cmd/api
go build -o org-hard-delete ./cmd/org-hard-delete
# Source checkout fallback (requires the Go toolchain and dependencies):
go run ./cmd/org-hard-delete --all --dry-run=true
```

`--org` and `--all` are mutually exclusive. Duplicate references to the same ID are
counted once; missing or ambiguous references stop before any deletion. The literal
confirmation is checked against the exact resolved count. `--dry-run=false` alone
cannot delete; `--execute` and the exact confirmation are both mandatory.
`--execute --dry-run=true` is rejected. No organizations selected is an error.

`--all` snapshots the target list at startup. Each organization is deleted in its
own transaction, and the batch stops on the first error with nonzero exit status.
**Earlier successful deletions remain committed**; inspect the output and re-preview
before retrying, then use the remaining count in the new confirmation. Concurrently
created organizations are not added to an already-confirmed batch. Counts can change
between preview and execution, and each service call recomputes them.

When a user's last organization is removed, the same existing user cleanup deletes
that affected account if it has no membership in any other organization and is not
a platform administrator. Users shared across the batch survive until their final
membership is deleted; platform administrators remain. Unrelated pre-existing orphan
accounts are outside the organization selection. A surviving cross-tenant reference
still causes a safe refusal; the CLI never disables constraints to force deletion.
