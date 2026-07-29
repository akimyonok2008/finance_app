#!/usr/bin/env bash
set -euo pipefail

: "${API_BINARY:?API_BINARY is required}"
: "${DATABASE_URL:?DATABASE_URL is required}"

api_port="${PORT:-18082}"
operations_port="${OPERATIONS_PORT:-18083}"
log_file="${API_LOG_FILE:-api-startup.log}"

APP_ENV="${APP_ENV:-test}" \
STORAGE_PROVIDER=postgres \
PRICE_PROVIDER=mock \
ENABLE_BACKGROUND_WORKERS=false \
ENABLE_QUOTE_REFRESH_WORKER=false \
PORT="$api_port" \
OPERATIONS_ADDR="127.0.0.1:${operations_port}" \
"$API_BINARY" >"$log_file" 2>&1 &
api_pid=$!

cleanup() {
  if kill -0 "$api_pid" 2>/dev/null; then
    kill -TERM "$api_pid"
    wait "$api_pid"
  fi
}
trap cleanup EXIT

for _ in {1..60}; do
  if curl --fail --silent "http://127.0.0.1:${operations_port}/ready" >/dev/null; then
    kill -TERM "$api_pid"
    wait "$api_pid"
    trap - EXIT
    exit 0
  fi
  if ! kill -0 "$api_pid" 2>/dev/null; then
    cat "$log_file" >&2
    exit 1
  fi
  sleep 1
done

cat "$log_file" >&2
echo "API did not become ready" >&2
exit 1
