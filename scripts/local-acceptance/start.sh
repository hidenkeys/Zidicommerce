#!/usr/bin/env bash
set -euo pipefail
umask 077
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STATE="$ROOT/.local-acceptance"
PG="$STATE/pg"
INITDB=/opt/homebrew/bin/initdb
PG_CTL=/opt/homebrew/bin/pg_ctl
for command in "$INITDB" "$PG_CTL" /opt/homebrew/bin/createdb /opt/homebrew/bin/psql; do
  [[ -x "$command" ]] || { echo "Missing Homebrew PostgreSQL 17 binary: $command" >&2; exit 1; }
done
for command in "$INITDB" "$PG_CTL"; do
  [[ "$("$command" --version)" == *" 17."* ]] || { echo "$command must be PostgreSQL 17." >&2; exit 1; }
done
for command in node npm go curl openssl lsof; do
  command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done
mkdir -p "$STATE/evidence"
chmod 700 "$STATE" "$STATE/evidence"
[[ ! -e "$STATE/pids" ]] || { echo 'Existing stack state: run scripts/local-acceptance/stop.sh first.' >&2; exit 1; }
for port in 55450 4325 4326 4327; do
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Port $port is already occupied; refusing to start." >&2; exit 1
  fi
done
# Compile before starting services. The API executes from STATE, so godotenv
# cannot discover a developer/staging .env in the repository.
(cd "$ROOT/apps/api" && go build -o "$STATE/api" ./cmd/api)
VITE_API_BASE_URL=http://127.0.0.1:4325 npm --prefix "$ROOT" run build:admin >"$STATE/build-admin.log" 2>&1
VITE_API_BASE_URL=http://127.0.0.1:4325 npm --prefix "$ROOT" run build:field >"$STATE/build-field.log" 2>&1
[[ ! -e "$STATE/.env" ]] || { echo 'Remove .local-acceptance/.env; only generated api.env is supported.' >&2; exit 1; }
cat > "$STATE/api.env" <<ENV
APP_ENV=development
ORGANIZATION_HARD_DELETE_ENABLED=true
SERVER_HOST=127.0.0.1
SERVER_PORT=4325
LOG_LEVEL=info
DATABASE_HOST=127.0.0.1
DATABASE_PORT=55450
DATABASE_NAME=commerce_acceptance DATABASE_URL=postgres://acceptance@127.0.0.1:55450/commerce_acceptance?sslmode=disable
DATABASE_USER=acceptance
DATABASE_SSLMODE=disable
JWT_SECRET=$(openssl rand -hex 32)
PAYMENT_SECRET_ENCRYPTION_KEY=$(openssl rand -hex 32)
CHANNEL_SECRET_ENCRYPTION_KEY=$(openssl rand -hex 32)
PAYMENT_PROVIDER=test
EMAIL_MODE=log
LOCAL_ACCEPTANCE_EMAIL_LOG=true
APP_BASE_URL=http://127.0.0.1:4326
FIELD_SERVICE_PORTAL_URL=http://127.0.0.1:4327
CORS_ALLOWED_ORIGINS=http://127.0.0.1:4326,http://127.0.0.1:4327
WHATSAPP_SIGNATURE_BYPASS=false
WHATSAPP_FLOWS_ENABLED=false
CONTROLLED_BETA=false
FIELD_SERVICE_DEMO_SEED=false
AI_PROVIDER=ollama
OLLAMA_BASE_URL=http://127.0.0.1:4328
AI_EMBEDDINGS_ENABLED=false
AI_VECTOR_SEARCH_ENABLED=false
AI_EMBEDDING_PROVIDER=disabled
ENV
printf 'MIGRATIONS_DIR=%q\n' "$ROOT/migrations" >> "$STATE/api.env"
chmod 600 "$STATE/api.env"
: > "$STATE/pids"
cleanup() {
  local status="$?"
  trap - EXIT INT TERM
  "$ROOT/scripts/local-acceptance/stop.sh" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [[ ! -f "$PG/PG_VERSION" ]]; then
  LC_ALL=C "$INITDB" -D "$PG" -U acceptance --locale=C --encoding=UTF8 --auth-local=trust --auth-host=trust >"$STATE/initdb.log" 2>&1
fi
[[ "$(cat "$PG/PG_VERSION")" == 17 ]] || { echo 'Existing cluster is not PostgreSQL 17.' >&2; false; }
# Keep the socket directly in STATE: a nested directory exceeds macOS sockaddr_un
# limits at this worktree path.
LC_ALL=C "$PG_CTL" -D "$PG" -l "$STATE/postgres.log" -o "-h 127.0.0.1 -p 55450 -k $STATE" -w start >/dev/null
if ! /opt/homebrew/bin/psql -h "$STATE" -p 55450 -U acceptance -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='commerce_acceptance'" | /usr/bin/grep -q 1; then
  /opt/homebrew/bin/createdb -h "$STATE" -p 55450 -U acceptance commerce_acceptance
fi
record_pid() {
  local pid="$1" started
  started="$(ps -p "$pid" -o lstart=)"
  [[ -n "$started" ]]
  printf '%s\t%s\n' "$pid" "$started" >> "$STATE/pids"
}
# env -i guarantees inherited provider/SMTP/staging credentials cannot leak in.
: > "$STATE/api.log"
chmod 600 "$STATE/api.log"
(
  cd "$STATE"
  exec env -i PATH="$PATH" HOME="$HOME" /bin/bash -c 'set -a; source "$1"; set +a; exec "$2"' _ "$STATE/api.env" "$STATE/api"
) >"$STATE/api.log" 2>&1 &
record_pid "$!"
wait_url() {
  local url="$1" count
  for count in {1..120}; do
    if curl --fail --silent --max-time 2 "$url" >/dev/null; then return 0; fi
    sleep 1
  done
  echo "Timed out waiting for $url. Inspect .local-acceptance logs." >&2
  return 1
}
wait_url http://127.0.0.1:4325/health
for app in admin field; do
  port=4326
  [[ "$app" != field ]] || port=4327
  (
    cd "$ROOT/apps/$app"
    exec node "$ROOT/node_modules/vite/bin/vite.js" preview --host 127.0.0.1 --port "$port" --strictPort --configLoader runner
  ) >"$STATE/$app.log" 2>&1 &
  record_pid "$!"
  wait_url "http://127.0.0.1:$port"
done
trap - EXIT INT TERM
printf 'Local acceptance ready: API 4325, Admin 4326, Field 4327, Postgres 55450.\nRun npm run acceptance:local; stop with scripts/local-acceptance/stop.sh.\n'
