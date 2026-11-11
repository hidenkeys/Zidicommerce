#!/usr/bin/env bash
set -uo pipefail

# Public, read-only release preflight. This script never authenticates, mutates
# application state, reads Railway variables, or prints configured URLs.

for command in curl git jq mktemp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$command" >&2
    exit 2
  fi
done

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
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

check_api_health() {
  local label="$1"
  local configured_url="$2"
  if [[ -z "$configured_url" ]]; then
    add_result "$label API health" "unverified" "No URL was supplied."
    return
  fi

  local base_url="${configured_url%/}"
  local body_file="$work_dir/${label// /_}-health.json"
  local http_code
  http_code=$(curl --silent --show-error --max-time 15 --output "$body_file" --write-out '%{http_code}' "$base_url/health" 2>/dev/null || true)
  if [[ "$http_code" == "200" ]] && jq -e '.status == "ok" and .checks.database == "ok"' "$body_file" >/dev/null 2>&1; then
    add_result "$label API health" "pass" "The public health endpoint reports API and database healthy."
  else
    add_result "$label API health" "fail" "The public health endpoint did not return the required healthy response."
  fi
}

check_admin_health() {
  local configured_url="$1"
  if [[ -z "$configured_url" ]]; then
    add_result "Admin public health" "unverified" "No Admin URL was supplied."
    return
  fi
  local http_code
  http_code=$(curl --silent --show-error --max-time 15 --output /dev/null --write-out '%{http_code}' "${configured_url%/}/" 2>/dev/null || true)
  if [[ "$http_code" == "200" ]]; then
    add_result "Admin public health" "pass" "The Admin root returned HTTP 200."
  else
    add_result "Admin public health" "fail" "The Admin root did not return HTTP 200."
  fi
}

cd "$repo_root"

migration_count=$(find migrations -maxdepth 1 -type f -name '*.sql' | wc -l | tr -d ' ')
latest_migration=$(find migrations -maxdepth 1 -type f -name '*.sql' -print | sort | tail -1)
latest_migration=${latest_migration##*/}
if [[ "$migration_count" -gt 0 && -n "$latest_migration" ]]; then
  add_result "Local migration inventory" "pass" "The release candidate contains ${migration_count} migrations ending at ${latest_migration}."
else
  add_result "Local migration inventory" "fail" "No migration inventory was found."
fi

dirty_count=$(git status --porcelain --untracked-files=normal | wc -l | tr -d ' ')
if [[ "$dirty_count" == "0" ]]; then
  add_result "Immutable release tree" "pass" "The working tree is clean."
else
  add_result "Immutable release tree" "fail" "The working tree has uncommitted or untracked release inputs."
fi

release_commit="${PRODUCTION_RELEASE_COMMIT:-}"
if [[ -z "$release_commit" ]]; then
  add_result "Production artifact migration inventory" "unverified" "No deployed production commit was supplied."
elif ! git cat-file -e "$release_commit^{commit}" 2>/dev/null; then
  add_result "Production artifact migration inventory" "fail" "The supplied production revision is not available in this repository."
else
  release_migration_count=$(git ls-tree -r --name-only "$release_commit" migrations | awk '/\.sql$/ {count++} END {print count+0}')
  release_latest=$(git ls-tree -r --name-only "$release_commit" migrations | awk '/\.sql$/ {print}' | sort | tail -1)
  release_latest=${release_latest##*/}
  if [[ "$release_migration_count" == "$migration_count" && "$release_latest" == "$latest_migration" ]]; then
    add_result "Production artifact migration inventory" "pass" "The deployed revision contains the candidate migration inventory."
  else
    add_result "Production artifact migration inventory" "fail" "The deployed revision migration inventory differs from the candidate."
  fi
fi

check_api_health "Pilot" "${PILOT_API_BASE_URL:-}"
check_api_health "Production" "${PRODUCTION_API_BASE_URL:-}"
check_admin_health "${ADMIN_BASE_URL:-}"

add_result "Production database migration status" "unverified" "Query schema_migrations through the protected database console; public health cannot attest migration completeness."
add_result "Production integration mode" "unverified" "Paystack mode and Meta asset/callback configuration require a redacted operator attestation."
add_result "Backup and restore" "unverified" "Attach a fresh backup and disposable restore rehearsal to the promotion record."

if [[ "${READINESS_REPORT_FORMAT:-markdown}" == "json" ]]; then
  jq '{results: ., summary: (group_by(.status) | map({key: .[0].status, value: length}) | from_entries)}' "$report_file"
else
  printf '| Check | Status | Note |\n'
  printf '|---|---|---|\n'
  jq -r '.[] | "| " + (.check | gsub("\\|"; "\\|")) + " | `" + .status + "` | " + (.note | gsub("\\|"; "\\|")) + " |"' "$report_file"
fi

if jq -e 'any(.[]; .status == "fail")' "$report_file" >/dev/null; then
  exit 1
fi
if jq -e 'any(.[]; .status == "unverified")' "$report_file" >/dev/null; then
  exit 2
fi
