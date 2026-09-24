#!/usr/bin/env bash
set -euo pipefail

for command in curl jq mktemp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$command" >&2
    exit 2
  fi
done

base_url="${ZIDI_STAGING_API_BASE_URL:-https://zidicommerce-staging-api-staging.up.railway.app}"
base_url="${base_url%/}"
if [[ "$base_url" == */v1 ]]; then
  api_url="$base_url"
  root_url="${base_url%/v1}"
else
  api_url="$base_url/v1"
  root_url="$base_url"
fi

case "$root_url" in
  https://zidicommerce-staging-api-staging.up.railway.app) ;;
  *)
    if [[ "${ZIDI_ALLOW_NONDEFAULT_STAGING_URL:-}" != "YES" ]]; then
      printf 'Refusing a non-default API URL. Set ZIDI_ALLOW_NONDEFAULT_STAGING_URL=YES after confirming it is disposable staging.\n' >&2
      exit 2
    fi
    ;;
esac

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
response_file="$work_dir/response.json"
http_code=""

request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    printf '%s' "$body" > "$work_dir/request.json"
    http_code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' \
      --request "$method" --header 'Content-Type: application/json' --data-binary "@$work_dir/request.json" "$url" || true)
  else
    http_code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' \
      --request "$method" "$url" || true)
  fi
}

assert_code() {
  local check="$1"
  local expected="$2"
  if [[ "$http_code" != "$expected" ]]; then
    printf 'FAIL: %s returned HTTP %s; expected %s. Response body withheld.\n' "$check" "$http_code" "$expected" >&2
    exit 1
  fi
  printf 'PASS: %s\n' "$check"
}

request GET "$root_url/health"
assert_code "API health" "200"

token="${ZIDI_STAGING_API_TOKEN:-}"
if [[ -z "$token" ]]; then
  if [[ -z "${ZIDI_STAGING_EMAIL:-}" || -z "${ZIDI_STAGING_PASSWORD:-}" ]]; then
    printf 'Set ZIDI_STAGING_API_TOKEN, or both ZIDI_STAGING_EMAIL and ZIDI_STAGING_PASSWORD.\n' >&2
    exit 2
  fi
  login_body=$(jq -nc --arg email "$ZIDI_STAGING_EMAIL" --arg password "$ZIDI_STAGING_PASSWORD" '{email: $email, password: $password}')
  request POST "$api_url/auth/login" "$login_body"
  assert_code "staging login" "200"
  token=$(jq -r '.data.access_token // empty' "$response_file")
fi

if [[ -z "$token" ]]; then
  printf 'Authentication did not return an access token.\n' >&2
  exit 1
fi

auth_config="$work_dir/auth.curl"
{
  printf 'silent\n'
  printf 'show-error\n'
  printf 'header = "Authorization: Bearer %s"\n' "$token"
  printf 'header = "Content-Type: application/json"\n'
} > "$auth_config"
chmod 600 "$auth_config"

auth_get() {
  local path="$1"
  http_code=$(curl --config "$auth_config" --output "$response_file" --write-out '%{http_code}' \
    --request GET "$api_url$path" || true)
}

auth_get "/auth/me"
assert_code "authenticated tenant context" "200"
if [[ "$(jq -r '.data.organization_id // empty' "$response_file")" == "" ]]; then
  printf 'FAIL: authenticated user has no tenant context.\n' >&2
  exit 1
fi

for resource in stores catalogue/products customers orders knowledge-entries payments channel-platform/connections; do
  auth_get "/$resource"
  assert_code "tenant-scoped $resource" "200"
done

request GET "$api_url/runtime/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=invalid-staging-probe&hub.challenge=denied"
assert_code "invalid WhatsApp verify token is rejected" "403"

invalid_payload='{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"unconfigured-staging-probe"}}}]}]}'
request POST "$api_url/runtime/webhooks/whatsapp" "$invalid_payload"
if [[ "$http_code" != "403" && "$http_code" != "404" ]]; then
  printf 'FAIL: unconfigured invalid WhatsApp POST returned HTTP %s; expected fail-closed 403 or 404.\n' "$http_code" >&2
  exit 1
fi
printf 'PASS: unconfigured invalid WhatsApp POST fails closed\n'

printf 'Staging smoke verification passed. No response bodies, tokens, credentials, or record identifiers were printed.\n'
