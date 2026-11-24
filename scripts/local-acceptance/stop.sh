#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STATE="$ROOT/.local-acceptance"
# Compare process start times to avoid killing an unrelated process after PID reuse.
if [[ -f "$STATE/pids" ]]; then
  while IFS=$'\t' read -r pid started; do
    [[ "$pid" =~ ^[0-9]+$ && -n "$started" ]] || continue
    current="$(ps -p "$pid" -o lstart= 2>/dev/null || true)"
    if [[ "$current" == "$started" ]]; then
      kill "$pid" 2>/dev/null || true
      for attempt in {1..30}; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
      current="$(ps -p "$pid" -o lstart= 2>/dev/null || true)"
      [[ "$current" != "$started" ]] || kill -KILL "$pid" 2>/dev/null || true
    fi
  done < "$STATE/pids"
  rm "$STATE/pids"
fi
if [[ -f "$STATE/pg/postmaster.pid" ]]; then
  /opt/homebrew/bin/pg_ctl -D "$STATE/pg" -m fast -w stop
fi
echo 'Local acceptance stopped. Disposable data remains in .local-acceptance/.'
