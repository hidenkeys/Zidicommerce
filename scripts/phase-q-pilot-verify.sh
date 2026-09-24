#!/usr/bin/env bash
set -uo pipefail

printf 'WARNING: phase-q-pilot-verify.sh is retired historical tooling. Use scripts/staging-smoke.sh for current staging validation.\n' >&2

for command in curl jq mktemp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$command" >&2
    exit 2
  fi
done

if [[ -z "${ZIDI_API_BASE_URL:-}" ]]; then
  printf 'Missing required environment variable: ZIDI_API_BASE_URL\n' >&2
  exit 2
fi

base_url="${ZIDI_API_BASE_URL%/}"
if [[ "$base_url" == */v1 ]]; then
  api_url="$base_url"
  root_url="${base_url%/v1}"
else
  api_url="${base_url}/v1"
  root_url="$base_url"
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
report_file="$work_dir/report.json"
printf '[]' > "$report_file"

add_result() {
  local check="$1"
  local status="$2"
  local note="$3"
  local next="$work_dir/report.next.json"
  jq --arg check "$check" --arg status "$status" --arg note "$note" \
    '. + [{check: $check, status: $status, note: $note}]' "$report_file" > "$next"
  mv "$next" "$report_file"
}

http_code=""
response_file=""

public_request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  response_file="$work_dir/response.json"
  if [[ -n "$body" ]]; then
    printf '%s' "$body" > "$work_dir/request.json"
    http_code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' \
      --request "$method" --header 'Content-Type: application/json' --data-binary "@$work_dir/request.json" "$url" 2>/dev/null || true)
  else
    http_code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' \
      --request "$method" "$url" 2>/dev/null || true)
  fi
}

token="${ZIDI_API_TOKEN:-}"
if [[ -z "$token" ]]; then
  if [[ -z "${ZIDI_PILOT_EMAIL:-}" || -z "${ZIDI_PILOT_PASSWORD:-}" ]]; then
    printf 'Set ZIDI_API_TOKEN, or both ZIDI_PILOT_EMAIL and ZIDI_PILOT_PASSWORD.\n' >&2
    exit 2
  fi
  login_body=$(jq -nc --arg email "$ZIDI_PILOT_EMAIL" --arg password "$ZIDI_PILOT_PASSWORD" '{email: $email, password: $password}')
  public_request POST "$api_url/auth/login" "$login_body"
  if [[ "$http_code" != "200" ]]; then
    printf 'Pilot login failed without exposing the response body.\n' >&2
    exit 1
  fi
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

auth_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  response_file="$work_dir/response.json"
  if [[ -n "$body" ]]; then
    printf '%s' "$body" > "$work_dir/request.json"
    http_code=$(curl --config "$auth_config" --output "$response_file" --write-out '%{http_code}' \
      --request "$method" --data-binary "@$work_dir/request.json" "$api_url$path" 2>/dev/null || true)
  else
    http_code=$(curl --config "$auth_config" --output "$response_file" --write-out '%{http_code}' \
      --request "$method" "$api_url$path" 2>/dev/null || true)
  fi
}

public_request GET "$root_url/health"
if [[ "$http_code" == "200" ]]; then
  add_result "API health" "pass" "The isolated pilot health endpoint returned HTTP 200."
else
  add_result "API health" "fail" "The isolated pilot health endpoint did not return HTTP 200."
fi

auth_request GET "/auth/me"
if [[ "$http_code" == "200" && "$(jq -r '.data.role // empty' "$response_file")" != "" ]]; then
  add_result "Pilot authentication" "pass" "The authenticated pilot owner session is valid."
else
  add_result "Pilot authentication" "fail" "The authenticated pilot owner session could not be validated."
fi

auth_request GET "/stores"
stores_total=0
stores_active=0
if [[ "$http_code" == "200" ]]; then
  stores_total=$(jq '.data | length' "$response_file" 2>/dev/null || printf '0')
  stores_active=$(jq '[.data[] | select(.status == "active")] | length' "$response_file" 2>/dev/null || printf '0')
fi
auth_request GET "/catalogue/products"
products_total=0
products_active=0
if [[ "$http_code" == "200" ]]; then
  products_total=$(jq '.data | length' "$response_file" 2>/dev/null || printf '0')
  products_active=$(jq '[.data[] | select(.status == "active")] | length' "$response_file" 2>/dev/null || printf '0')
fi
if (( stores_total > 0 && stores_active > 0 && products_total > 0 && products_active > 0 )); then
  add_result "Tenant commerce seed" "pass" "Tenant-scoped stores and products are present, including active records."
else
  add_result "Tenant commerce seed" "fail" "Expected active tenant commerce records were not available."
fi

auth_request GET "/channel-platform/connections"
connection_id=""
if [[ "$http_code" == "200" ]]; then
  connection_id=$(jq -r '[.data[] | select(.provider == "whatsapp" and .status != "archived")][0].id // empty' "$response_file")
fi

configuration_file="$work_dir/whatsapp-configuration.json"
if [[ -n "$connection_id" ]]; then
  auth_request POST "/channel-platform/connections/$connection_id/whatsapp/health-check" '{}'
  auth_request GET "/channel-platform/connections/$connection_id/whatsapp"
  cp "$response_file" "$configuration_file"
  connection_status=$(jq -r '.data.connection.status // empty' "$configuration_file")
  setup_state=$(jq -r '.data.setup_state // empty' "$configuration_file")
  signature_status=$(jq -r '.data.signature_status // empty' "$configuration_file")
  signature_verified=$(jq -r '.data.checklist.signature_verified // false' "$configuration_file")
  if [[ "$http_code" == "200" && "$connection_status" == "healthy" && "$setup_state" == "healthy" ]]; then
    add_result "WhatsApp setup and health" "pass" "Connection health and completed setup both report healthy."
  else
    add_result "WhatsApp setup and health" "fail" "Connection health and setup lifecycle are not coherent."
  fi
  if [[ "$signature_status" == "verified" && "$signature_verified" == "true" ]]; then
    add_result "Valid webhook signature evidence" "pass" "Durable valid-signature evidence is present independently of rejected attempts."
  else
    add_result "Valid webhook signature evidence" "fail" "Durable valid-signature evidence is missing."
  fi
else
  printf '{}' > "$configuration_file"
  add_result "WhatsApp setup and health" "fail" "No active WhatsApp connection was found for the authenticated tenant."
  add_result "Valid webhook signature evidence" "fail" "Signature evidence could not be checked without a WhatsApp connection."
fi

auth_request GET "/payments"
paid_order_id="${PHASE_Q_PAID_ORDER_ID:-}"
verified_payments=0
initialized_payments=0
payment_evidence_status="pass"
case "${PHASE_Q_PAYMENT_MODE:-unknown}" in
  sandbox) payment_evidence_status="sandbox_verified" ;;
  manual) payment_evidence_status="manually_verified" ;;
esac
if [[ "$http_code" == "200" ]]; then
  verified_payments=$(jq '[.data[] | select(.provider == "paystack" and .status == "paid" and .verified_at != null)] | length' "$response_file" 2>/dev/null || printf '0')
  initialized_payments=$(jq '[.data[] | select(.provider == "paystack" and (.authorization_url // "") != "")] | length' "$response_file" 2>/dev/null || printf '0')
fi
if (( initialized_payments > 0 )); then
  add_result "Order-to-payment initialization" "manually_verified" "The pilot retains provider checkout evidence from the controlled order-flow acceptance."
else
  add_result "Order-to-payment initialization" "not_attempted" "No provider checkout evidence was available; run the controlled customer order flow before payment verification."
fi
if (( verified_payments > 0 )); then
  add_result "Verified Paystack payment evidence" "$payment_evidence_status" "Provider-verified Paystack payment evidence is present for the asserted environment mode."
else
  add_result "Verified Paystack payment evidence" "blocked_external_action" "No provider-verified payment exists; completing provider checkout remains an external action."
fi

if [[ "${PHASE_Q_UNPAID_SAFETY_CONFIRM:-}" == "YES" && -n "${PHASE_Q_UNPAID_ORDER_ID:-}" ]]; then
  unpaid_body=$(jq -nc --arg key "phase-q-unpaid-$RANDOM-$(date +%s)" '{status: "ready", idempotency_key: $key, source: "phase_q_verifier"}')
  auth_request PATCH "/fulfilment/${PHASE_Q_UNPAID_ORDER_ID}" "$unpaid_body"
  if [[ "$http_code" == "400" ]]; then
    add_result "Unpaid fulfilment safety" "expected_safety_block" "The API rejected a direct ready transition for an unpaid order."
  else
    add_result "Unpaid fulfilment safety" "fail" "The unpaid fulfilment safety probe did not return the expected HTTP 400 rejection."
  fi
else
  add_result "Unpaid fulfilment safety" "not_attempted" "Set an exact test order and PHASE_Q_UNPAID_SAFETY_CONFIRM=YES to run the guarded mutation; deterministic coverage still enforces the rule."
fi

if [[ "${PHASE_Q_FULFILMENT_CONFIRM:-}" == "YES" && -n "$paid_order_id" ]]; then
  post_payment_failed=false
  for _ in {1..6}; do
    auth_request GET "/orders/$paid_order_id/operations"
    if [[ "$http_code" != "200" ]]; then
      post_payment_failed=true
      break
    fi
    order_status=$(jq -r '.data.order.status // empty' "$response_file")
    payment_status=$(jq -r '.data.payment.status // empty' "$response_file")
    payment_verified=$(jq -r '.data.payment.verified_at != null' "$response_file")
    fulfilment_type=$(jq -r '.data.order.fulfilment_type // empty' "$response_file")
    if [[ "$payment_status" != "paid" || "$payment_verified" != "true" ]]; then
      post_payment_failed=true
      break
    fi
    case "$order_status" in
      paid) target="processing" ;;
      processing) target="ready" ;;
      ready)
        if [[ "$fulfilment_type" == "merchant_rider" ]]; then target="out_for_delivery"; else target="completed"; fi
        ;;
      out_for_delivery) target="completed" ;;
      completed) break ;;
      *) post_payment_failed=true; break ;;
    esac
    transition_body=$(jq -nc --arg status "$target" --arg key "phase-q-$target-$RANDOM-$(date +%s)" '{status: $status, reason: "Phase Q verified-payment pilot validation", idempotency_key: $key, source: "phase_q_verifier"}')
    auth_request POST "/orders/$paid_order_id/transition" "$transition_body"
    if [[ "$http_code" != "200" ]]; then
      post_payment_failed=true
      break
    fi
  done
  auth_request GET "/orders/$paid_order_id/operations"
  final_order=$(jq -r '.data.order.status // empty' "$response_file" 2>/dev/null || true)
  final_fulfilment=$(jq -r '.data.fulfilment.status // empty' "$response_file" 2>/dev/null || true)
  completed_events=$(jq '[.data.events[] | select(.event_type == "order.completed")] | length' "$response_file" 2>/dev/null || printf '0')
  if [[ "$post_payment_failed" == "false" && "$final_order" == "completed" && "$final_fulfilment" == "completed" && "$completed_events" -gt 0 ]]; then
    add_result "Post-payment fulfilment" "$payment_evidence_status" "A provider-verified paid order reached completed order and fulfilment states with timeline evidence."
  else
    add_result "Post-payment fulfilment" "fail" "The guarded verified-payment workflow did not reach a consistent completed state."
  fi
elif [[ -z "$paid_order_id" && "$verified_payments" -eq 0 ]]; then
  add_result "Post-payment fulfilment" "blocked_external_action" "A provider-verified paid test order is required before fulfilment can be exercised."
elif [[ -z "$paid_order_id" ]]; then
  add_result "Post-payment fulfilment" "not_attempted" "Set the exact PHASE_Q_PAID_ORDER_ID and PHASE_Q_FULFILMENT_CONFIRM=YES to run the guarded workflow."
else
  add_result "Post-payment fulfilment" "not_attempted" "Set PHASE_Q_FULFILMENT_CONFIRM=YES to advance one provider-verified pilot order through fulfilment."
fi

auth_request GET "/runtime/support-handoffs"
if [[ "$http_code" == "200" ]]; then
  handoff_count=$(jq '.data | length' "$response_file" 2>/dev/null || printf '0')
  if (( handoff_count > 0 )); then
    add_result "Human handoff" "manually_verified" "Tenant-scoped handoff history remains readable; the claim and release lifecycle was verified during the pilot."
  else
    add_result "Human handoff" "not_attempted" "The endpoint is available, but this tenant has no handoff history to re-check."
  fi
else
  add_result "Human handoff" "fail" "The tenant-scoped handoff endpoint failed."
fi

if [[ "${PHASE_Q_AI_ENQUIRY_CONFIRM:-}" == "YES" ]]; then
  auth_request POST "/ai/test-chat/start" '{}'
  session_id=$(jq -r '.data.session_id // empty' "$response_file" 2>/dev/null || true)
  enquiry_failed=false
  unknown_refused=false
  if [[ "$http_code" != "200" || -z "$session_id" ]]; then
    enquiry_failed=true
  else
    for question in "What do you sell?" "Where are your stores?" "Do you deliver?" "Do you sell the Phase Q imaginary product?"; do
      enquiry_body=$(jq -nc --arg session_id "$session_id" --arg text "$question" '{session_id: $session_id, text: $text}')
      for attempt in 1 2 3; do
        auth_request POST "/ai/test-chat/message" "$enquiry_body"
        if [[ "$http_code" == "200" ]]; then
          break
        fi
        if [[ "$http_code" != "429" && "$http_code" != 5?? ]]; then
          break
        fi
        sleep $((attempt * 2))
      done
      answer=$(jq -r '.data.message.body // empty' "$response_file" 2>/dev/null || true)
      provider_error=$(jq -r '.data.debug.error // empty' "$response_file" 2>/dev/null || true)
      successful_tools=$(jq '[.data.debug.tools[]? | select(.status == "ok")] | length' "$response_file" 2>/dev/null || printf '0')
      if [[ "$http_code" != "200" || -z "$answer" || -n "$provider_error" || "$successful_tools" -eq 0 ]]; then
        enquiry_failed=true
      fi
      if [[ "$question" == *"imaginary"* ]] && printf '%s' "$answer" | grep -Eiq "(don't|doesn't|isn't|do not|is not|not available|not found|couldn't|could not|unable|no matching|no .*product|cannot|haven't).*"; then
        unknown_refused=true
      fi
    done
  fi
  if [[ "$enquiry_failed" == "false" && "$unknown_refused" == "true" ]]; then
    add_result "Grounded customer enquiries" "pass" "Product, location, delivery, and unknown-product enquiries returned usable grounded responses."
  else
    add_result "Grounded customer enquiries" "fail" "One or more grounded enquiry checks failed or the unknown product was not conservatively refused."
  fi
else
  add_result "Grounded customer enquiries" "not_attempted" "Set PHASE_Q_AI_ENQUIRY_CONFIRM=YES to run live provider-backed enquiry checks."
fi

if [[ "${PHASE_Q_SIGNATURE_NEGATIVE_CONFIRM:-}" == "YES" && -n "$connection_id" ]]; then
  phone_id=$(jq -r '.data.phone_number_id // empty' "$configuration_file")
  before_rejections=$(jq -r '.data.rejected_signature_count // 0' "$configuration_file")
  before_setup=$(jq -r '.data.setup_state // empty' "$configuration_file")
  if [[ -n "$phone_id" ]]; then
    probe_id="phase-q-security-probe-$RANDOM-$(date +%s)"
    invalid_body=$(jq -nc --arg phone_id "$phone_id" --arg probe_id "$probe_id" '{object: "whatsapp_business_account", entry: [{id: $probe_id, changes: [{field: "messages", value: {messaging_product: "whatsapp", metadata: {phone_number_id: $phone_id}, messages: []}}]}]}')
    printf '%s' "$invalid_body" > "$work_dir/request.json"
    http_code=$(curl --silent --show-error --output "$response_file" --write-out '%{http_code}' \
      --request POST --header 'Content-Type: application/json' --header 'X-Hub-Signature-256: sha256=invalid-phase-q-probe' \
      --data-binary "@$work_dir/request.json" "$api_url/runtime/webhooks/whatsapp" 2>/dev/null || true)
    webhook_code="$http_code"
    auth_request GET "/channel-platform/connections/$connection_id/whatsapp"
    after_status=$(jq -r '.data.signature_status // empty' "$response_file")
    after_verified=$(jq -r '.data.checklist.signature_verified // false' "$response_file")
    after_rejections=$(jq -r '.data.rejected_signature_count // 0' "$response_file")
    after_setup=$(jq -r '.data.setup_state // empty' "$response_file")
    if [[ "$webhook_code" == "403" && "$after_status" == "verified" && "$after_verified" == "true" && "$after_setup" == "$before_setup" && "$after_rejections" -gt "$before_rejections" ]]; then
      add_result "Invalid WhatsApp signature" "pass" "The invalid webhook was rejected and recorded without erasing valid-signature readiness."
    else
      add_result "Invalid WhatsApp signature" "fail" "The rejection probe did not preserve verified signature and healthy setup state."
    fi
  else
    add_result "Invalid WhatsApp signature" "fail" "The configured WhatsApp identity was unavailable for the guarded security probe."
  fi
else
  add_result "Invalid WhatsApp signature" "not_attempted" "Set PHASE_Q_SIGNATURE_NEGATIVE_CONFIRM=YES to run the guarded HTTP 403 security probe."
fi

if [[ -n "$connection_id" ]]; then
  auth_request GET "/channel-platform/connections/$connection_id/events?limit=100"
  outbound_events=$(jq '[.data[] | select(.event_type == "outbound_send" and .normalized_status == "sent")] | length' "$response_file" 2>/dev/null || printf '0')
  delivered_events=$(jq '[.data[] | select(.event_type == "delivery_status" and .normalized_status == "delivered")] | length' "$response_file" 2>/dev/null || printf '0')
  rejected_events=$(jq '[.data[] | select(.event_type == "webhook_rejected" and .normalized_status == "rejected")] | length' "$response_file" 2>/dev/null || printf '0')
  if [[ "$http_code" == "200" && "$outbound_events" -gt 0 && "$delivered_events" -gt 0 && "$rejected_events" -gt 0 ]]; then
    add_result "Provider and runtime evidence" "pass" "Sent, delivered, and rejected-security events remain separately observable."
  else
    add_result "Provider and runtime evidence" "fail" "Expected aggregate provider event classes were not observable."
  fi
else
  add_result "Provider and runtime evidence" "not_attempted" "Provider events require an active WhatsApp connection."
fi

format="${PHASE_Q_REPORT_FORMAT:-markdown}"
if [[ "$format" == "json" ]]; then
  jq '{generated_at: (now | todateiso8601), results: .}' "$report_file"
else
  printf '# Phase Q Pilot Verification\n\n'
  printf '| Check | Classification | Note |\n'
  printf '|---|---|---|\n'
  jq -r '.[] | "| \(.check | gsub("\\|"; "\\\\|")) | `\(.status)` | \(.note | gsub("\\|"; "\\\\|")) |"' "$report_file"
fi

if jq -e 'any(.[]; .status == "fail")' "$report_file" >/dev/null; then
  exit 1
fi
