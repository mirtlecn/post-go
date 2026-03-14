#!/usr/bin/env bash

set -euo pipefail

if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1 || ! command -v redis-cli >/dev/null 2>&1; then
  echo "curl, jq and redis-cli are required" >&2
  exit 1
fi

TMP_DIR="${TMP_DIR:-$(mktemp -d)}"

cleanup_smoke_tmp() {
  rm -rf "$TMP_DIR"
}

pass() {
  echo "PASS $1"
}

fail() {
  echo "FAIL $1" >&2
  if [[ $# -gt 1 ]]; then
    echo "$2" >&2
  fi
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    fail "$label" "expected to find '$needle' in '$haystack'"
  fi
}

configure_redis() {
  REDIS_URL="${1:?redis url is required}"
  REDIS_HOSTPORT="${REDIS_URL#redis://}"
  REDIS_HOSTPORT="${REDIS_HOSTPORT%%/*}"
  REDIS_DB="${REDIS_URL##*/}"
  REDIS_HOST="${REDIS_HOSTPORT%%:*}"
  REDIS_PORT="${REDIS_HOSTPORT##*:}"
}

redis_get() {
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" GET "$1"
}

redis_flush() {
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" FLUSHDB >/dev/null
}

api_json() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  local auth="${4:-yes}"
  local extra_header_name="${5:-}"
  local extra_header_value="${6:-}"
  local response_file="$TMP_DIR/response.body"
  local status_file="$TMP_DIR/response.status"
  local -a args
  args=(-sS -o "$response_file" -w "%{http_code}" -X "$method" -H "Content-Type: application/json")
  if [[ "$auth" == "yes" ]]; then
    args+=(-H "Authorization: Bearer $POST_TOKEN")
  fi
  if [[ -n "$extra_header_name" ]]; then
    args+=(-H "$extra_header_name: $extra_header_value")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  args+=("$url")
  curl "${args[@]}" >"$status_file"
  API_STATUS="$(cat "$status_file")"
  API_BODY="$(cat "$response_file")"
}

assert_status() {
  local expected="$1"
  local label="$2"
  if [[ "$API_STATUS" != "$expected" ]]; then
    fail "$label" "expected HTTP $expected, got $API_STATUS, body: $API_BODY"
  fi
}

assert_jq() {
  local expr="$1"
  local label="$2"
  if ! jq -e "$expr" >/dev/null <<<"$API_BODY"; then
    fail "$label" "body assertion failed: $expr, body: $API_BODY"
  fi
}

start_server() {
  local binary_path="$1"
  local port="$2"
  local redis_url="$3"
  local secret_key="${4:-demo}"
  local log_file="$TMP_DIR/server-$port.log"
  PORT="$port" LINKS_REDIS_URL="$redis_url" SECRET_KEY="$secret_key" "$binary_path" >"$log_file" 2>&1 &
  SERVER_PID=$!
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -sS "http://127.0.0.1:$port/" >/dev/null 2>&1 || grep -q "Server running" "$log_file" 2>/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  cat "$log_file" >&2 || true
  fail "start server" "server did not become ready on port $port"
}

stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
    unset SERVER_PID
  fi
}
