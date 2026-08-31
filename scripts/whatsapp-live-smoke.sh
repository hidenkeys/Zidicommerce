#!/usr/bin/env bash
set -euo pipefail

required=(ZIDI_API_BASE_URL ZIDI_API_TOKEN WHATSAPP_CONNECTION_ID WHATSAPP_TEST_RECIPIENT)
for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "$name" >&2
    exit 2
  fi
done

if [[ "${WHATSAPP_LIVE_SEND_CONFIRM:-}" != "YES" ]]; then
  printf 'Set WHATSAPP_LIVE_SEND_CONFIRM=YES only after approving a real test message.\n' >&2
  exit 2
fi

base_url="${ZIDI_API_BASE_URL%/}"
connection_url="${base_url}/channel-platform/connections/${WHATSAPP_CONNECTION_ID}/whatsapp"

auth_get() {
  local url="$1"
  printf 'silent\nshow-error\nfail-with-body\nheader = "Authorization: Bearer %s"\nurl = "%s"\n' "$ZIDI_API_TOKEN" "$url" | curl --config -
}

auth_post() {
  local url="$1"
  local body="$2"
  local config
  config=$(printf 'silent\nshow-error\nfail-with-body\nrequest = "POST"\nheader = "Authorization: Bearer %s"\nheader = "Content-Type: application/json"\nurl = "%s"\n' "$ZIDI_API_TOKEN" "$url")
  printf '%s' "$body" | curl --config /dev/fd/3 --data-binary @- 3<<<"$config"
}

health_status=$(curl -sS -o /dev/null -w '%{http_code}' "${base_url%/v1}/health")
if [[ "$health_status" != "200" ]]; then
  printf 'API health check failed with HTTP %s.\n' "$health_status" >&2
  exit 1
fi

health=$(auth_post "${connection_url}/health-check" '{}')
printf '%s' "$health" | jq '{health: .data.status, setup_state: .data.setup_state, issues: .data.issues}'

if [[ -n "${WHATSAPP_TEMPLATE_ID:-}" ]]; then
  template_variables="${WHATSAPP_TEMPLATE_VARIABLES_JSON:-{}}"
  if ! printf '%s' "$template_variables" | jq -e 'type == "object"' >/dev/null; then
    printf 'WHATSAPP_TEMPLATE_VARIABLES_JSON must be a JSON object.\n' >&2
    exit 2
  fi
  payload=$(jq -nc --arg recipient "$WHATSAPP_TEST_RECIPIENT" --arg template_id "$WHATSAPP_TEMPLATE_ID" --argjson variables "$template_variables" '{recipient: $recipient, template_id: $template_id, template_variables: $variables}')
elif [[ "${WHATSAPP_EXPECT_OPEN_WINDOW:-}" == "YES" ]]; then
  payload=$(jq -nc --arg recipient "$WHATSAPP_TEST_RECIPIENT" --arg message "${WHATSAPP_TEST_MESSAGE:-Zidi WhatsApp connection test. Reply to confirm inbound delivery.}" '{recipient: $recipient, message: $message}')
else
  printf 'Set WHATSAPP_TEMPLATE_ID for an approved template, or WHATSAPP_EXPECT_OPEN_WINDOW=YES only after confirming a current inbound service window.\n' >&2
  exit 2
fi
sent=$(auth_post "${connection_url}/test-message" "$payload")
printf '%s' "$sent" | jq '{status: .data.status, recipient: .data.recipient_display, sent_at: .data.sent_at}'
printf 'Reply from the approved test recipient. Waiting up to 60 seconds for the signed inbound webhook.\n'

for _ in {1..12}; do
  state=$(auth_get "$connection_url")
  if [[ "$(printf '%s' "$state" | jq -r '.data.checklist.inbound_test_received')" == "true" ]]; then
    printf '%s' "$state" | jq '{setup_state: .data.setup_state, inbound_test_received: .data.checklist.inbound_test_received, signature_status: .data.signature_status, recent_events: [.data.operational_events[:5][] | {title, status, occurred_at}]}'
    exit 0
  fi
  sleep 5
done

state=$(auth_get "$connection_url")
printf '%s' "$state" | jq '{setup_state: .data.setup_state, inbound_test_received: .data.checklist.inbound_test_received, signature_status: .data.signature_status, recent_events: [.data.operational_events[:5][] | {title, status, guidance, occurred_at}]}'
printf 'No matching inbound reply was observed before the timeout.\n' >&2
exit 1
