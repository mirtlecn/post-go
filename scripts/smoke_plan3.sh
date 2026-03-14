#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3013}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/14}"

if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1 || ! command -v redis-cli >/dev/null 2>&1; then
  echo "curl, jq and redis-cli are required" >&2
  exit 1
fi

REDIS_HOSTPORT="${REDIS_URL#redis://}"
REDIS_HOSTPORT="${REDIS_HOSTPORT%%/*}"
REDIS_DB="${REDIS_URL##*/}"
REDIS_HOST="${REDIS_HOSTPORT%%:*}"
REDIS_PORT="${REDIS_HOSTPORT##*:}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

TOPIC="anime-$(date +%s)"

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

api() {
  local method="$1"
  local body="${2:-}"
  local response_file="$TMP_DIR/response.body"
  local status_file="$TMP_DIR/response.status"
  local -a args
  args=(-sS -o "$response_file" -w "%{http_code}" -X "$method" -H "Authorization: Bearer $POST_TOKEN" -H "Content-Type: application/json")
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  args+=("$POST_BASE_URL/")
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

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    fail "$label" "expected to find '$needle' in '$haystack'"
  fi
}

redis_get() {
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" GET "$1"
}

redis_flush() {
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" FLUSHDB >/dev/null
}

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api POST '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "create topic"
assert_jq '.type == "topic"' "create topic type"
assert_jq '.content == "0"' "create topic count"
pass "create topic"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
assert_contains "$TOPIC_RAW" '"type":"topic"' "topic stored type"
pass "topic stored type"

api GET '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup"
assert_jq '.type == "topic"' "topic lookup type"
assert_jq '.content == "0"' "topic lookup count"
pass "topic lookup"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "<title>$TOPIC</title>" "topic home title"
assert_contains "$TOPIC_HOME" "<h1 id=\"$TOPIC\">$TOPIC</h1>" "topic home heading"
pass "topic home render"

api POST '{"path":"'"$TOPIC"'","type":"topic","ttl":10}'
assert_status 400 "reject topic ttl"
assert_jq '.error == "topic does not support ttl"' "reject topic ttl body"
pass "reject topic ttl"

api POST '{"path":"'"$TOPIC"'","url":"hello"}'
assert_status 400 "protect topic home create"
pass "protect topic home create"

api POST '{"topic":"'"$TOPIC"'","path":"castle-notes","url":"# Castle\n\nHello","type":"md2html","title":"Castle Notes"}'
assert_status 201 "create topic item via topic"
assert_jq '.path == "'"$TOPIC"'/castle-notes"' "topic path rewrite"
pass "create topic item via topic"

ITEM_HTML="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ITEM_HTML" "<title>Castle Notes</title>" "topic item title"
assert_contains "$ITEM_HTML" "Back to $TOPIC" "topic item backlink"
pass "topic item render"

api POST '{"path":"'"$TOPIC"'/screening-signup","url":"https://example.com/screening","type":"url"}'
assert_status 201 "create topic item via full path"
pass "create topic item via full path"

api GET '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after items"
assert_jq '.content == "2"' "topic count after items"
pass "topic count after items"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "Castle Notes" "topic home first item"
assert_contains "$TOPIC_HOME" "screening-signup" "topic home fallback title"
assert_contains "$TOPIC_HOME" "↗" "topic home url mark"
pass "topic home rebuild"

api DELETE '{"path":"'"$TOPIC"'"}'
assert_status 400 "protect topic home delete"
pass "protect topic home delete"

api DELETE '{"path":"'"$TOPIC"'/screening-signup"}'
assert_status 200 "delete topic item"
pass "delete topic item"

api GET '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic count after delete"
assert_jq '.content == "1"' "topic count after delete body"
pass "topic count after delete"

api DELETE '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "delete topic"
assert_jq '.content == "1"' "delete topic content count"
pass "delete topic"

ORPHAN="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ORPHAN" "Castle Notes" "orphan item survives topic delete"
pass "orphan item survives topic delete"

api POST '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "recreate topic"
assert_jq '.content == "1"' "recreate topic adopts orphan"
pass "recreate topic"

api GET '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup recreated topic"
assert_jq '.content == "1"' "lookup recreated topic count"
pass "lookup recreated topic"

echo "Smoke PLAN3 checks completed successfully."
