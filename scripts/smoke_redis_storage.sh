#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3014}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/13}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
trap cleanup_smoke_tmp EXIT
configure_redis "$REDIS_URL"

SMOKE_PREFIX="redis-smoke-$(date +%s)"
TOPIC="$SMOKE_PREFIX-topic"

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api_json POST "$POST_BASE_URL/" '{"url":"hello redis","path":"'"$SMOKE_PREFIX"'-text","title":"Redis Title","ttl":0}'
assert_status 201 "create text ttl zero"
assert_jq '.ttl == null' "create text ttl zero response"
TEXT_VALUE="$(redis_get "surl:$SMOKE_PREFIX-text")"
if ! jq -e '.type == "text" and .content == "hello redis" and .title == "Redis Title"' >/dev/null <<<"$TEXT_VALUE"; then
  fail "redis text json value" "value: $TEXT_VALUE"
fi
TEXT_TTL="$(redis_ttl "surl:$SMOKE_PREFIX-text")"
if [[ "$TEXT_TTL" != "-1" ]]; then
  fail "redis text ttl" "expected -1, got $TEXT_TTL"
fi
pass "redis text storage"

api_json POST "$POST_BASE_URL/" '{"url":"expiring","path":"'"$SMOKE_PREFIX"'-ttl","ttl":5}'
assert_status 201 "create ttl item"
assert_jq '.ttl == 5' "create ttl item response"
TTL_SECONDS="$(redis_ttl "surl:$SMOKE_PREFIX-ttl")"
if [[ "$TTL_SECONDS" -le 0 || "$TTL_SECONDS" -gt 300 ]]; then
  fail "redis ttl seconds" "expected 1..300, got $TTL_SECONDS"
fi
pass "redis ttl storage"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "create topic"
TOPIC_VALUE="$(redis_get "surl:$TOPIC")"
if ! jq -e '.type == "topic"' >/dev/null <<<"$TOPIC_VALUE"; then
  fail "redis topic home type" "value: $TOPIC_VALUE"
fi
if [[ "$(redis_type "topic:$TOPIC:items")" != "zset" ]]; then
  fail "redis topic zset init type" "type: $(redis_type "topic:$TOPIC:items")"
fi
if [[ "$(redis_zcard "topic:$TOPIC:items")" != "1" ]]; then
  fail "redis topic zset init count" "zcard: $(redis_zcard "topic:$TOPIC:items")"
fi
pass "redis topic init"

api_json POST "$POST_BASE_URL/" '{"topic":"'"$TOPIC"'","path":"entry","url":"# Entry\n\nHello","type":"md2html","title":"Entry Title"}'
assert_status 201 "create topic item"
ITEM_VALUE="$(redis_get "surl:$TOPIC/entry")"
if ! jq -e '.type == "md" and .title == "Entry Title" and .content == "# Entry\n\nHello"' >/dev/null <<<"$ITEM_VALUE"; then
  fail "redis topic item json" "value: $ITEM_VALUE"
fi
if [[ "$(redis_zcard "topic:$TOPIC:items")" != "2" ]]; then
  fail "redis topic zset count after create" "zcard: $(redis_zcard "topic:$TOPIC:items")"
fi
if [[ "$(redis_type "topic:$TOPIC:items")" != "zset" ]]; then
  fail "redis topic zset type after create" "type: $(redis_type "topic:$TOPIC:items")"
fi
if [[ "$(redis_zrange "topic:$TOPIC:items")" != $'__topic_placeholder__\nentry' ]]; then
  fail "redis topic zset member" "members: $(redis_zrange "topic:$TOPIC:items")"
fi
SCORE_BEFORE="$(redis_zscore "topic:$TOPIC:items" "entry")"
if [[ -z "$SCORE_BEFORE" ]]; then
  fail "redis topic zset score" "score missing"
fi
pass "redis topic item create"

sleep 1
api_json PUT "$POST_BASE_URL/" '{"topic":"'"$TOPIC"'","path":"entry","url":"# Entry\n\nUpdated","type":"md2html","title":"Entry Title 2"}'
assert_status 200 "update topic item"
ITEM_VALUE="$(redis_get "surl:$TOPIC/entry")"
if ! jq -e '.type == "md" and .title == "Entry Title 2" and .content == "# Entry\n\nUpdated"' >/dev/null <<<"$ITEM_VALUE"; then
  fail "redis topic item update json" "value: $ITEM_VALUE"
fi
SCORE_AFTER="$(redis_zscore "topic:$TOPIC:items" "entry")"
if [[ -z "$SCORE_AFTER" || "$SCORE_AFTER" -le "$SCORE_BEFORE" ]]; then
  fail "redis topic zset score update" "before: $SCORE_BEFORE after: $SCORE_AFTER"
fi
pass "redis topic item update"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$TOPIC"'/entry"}'
assert_status 200 "delete topic item"
if [[ "$(redis_exists "surl:$TOPIC/entry")" != "0" ]]; then
  fail "redis topic item delete key" "exists: $(redis_exists "surl:$TOPIC/entry")"
fi
if [[ "$(redis_zcard "topic:$TOPIC:items")" != "1" ]]; then
  fail "redis topic item delete zset" "zcard: $(redis_zcard "topic:$TOPIC:items")"
fi
pass "redis topic item delete"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'/orphan","url":"hello orphan","type":"text"}'
assert_status 201 "create orphan candidate"
if [[ "$(redis_zcard "topic:$TOPIC:items")" != "2" ]]; then
  fail "redis orphan candidate indexed" "zcard: $(redis_zcard "topic:$TOPIC:items")"
fi
api_json DELETE "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "delete topic"
if [[ "$(redis_exists "surl:$TOPIC")" != "0" ]]; then
  fail "redis topic home delete" "exists: $(redis_exists "surl:$TOPIC")"
fi
if [[ "$(redis_exists "topic:$TOPIC:items")" != "0" ]]; then
  fail "redis topic zset delete" "exists: $(redis_exists "topic:$TOPIC:items")"
fi
if [[ "$(redis_exists "surl:$TOPIC/orphan")" != "1" ]]; then
  fail "redis orphan survives" "exists: $(redis_exists "surl:$TOPIC/orphan")"
fi
pass "redis topic delete semantics"

echo "Redis storage smoke checks completed successfully."
