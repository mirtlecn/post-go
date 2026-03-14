#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3013}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/14}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
trap cleanup_smoke_tmp EXIT
configure_redis "$REDIS_URL"

TOPIC="anime-$(date +%s)"

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","convert":"topic"}'
assert_status 201 "create topic via alias"
assert_jq '.type == "topic"' "create topic type"
assert_jq '.content == "0"' "create topic count"
pass "create topic via alias"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
assert_contains "$TOPIC_RAW" '"type":"topic"' "topic stored type"
pass "topic stored type"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","convert":"topic"}'
assert_status 200 "topic lookup"
assert_jq '.type == "topic"' "topic lookup type"
assert_jq '.content == "0"' "topic lookup count"
pass "topic lookup"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "<title>$TOPIC</title>" "topic home title"
assert_contains "$TOPIC_HOME" "<h1 id=\"$TOPIC\">$TOPIC</h1>" "topic home heading"
pass "topic home render"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic","ttl":10}'
assert_status 400 "reject topic ttl"
assert_jq '.error == "topic does not support ttl"' "reject topic ttl body"
pass "reject topic ttl"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","url":"hello"}'
assert_status 400 "protect topic home create"
pass "protect topic home create"

api_json POST "$POST_BASE_URL/" '{"topic":"missing-'"$TOPIC"'","path":"x","url":"hello"}'
assert_status 400 "reject missing topic"
assert_jq '.error == "topic does not exist"' "reject missing topic body"
pass "reject missing topic"

api_json POST "$POST_BASE_URL/" '{"topic":"'"$TOPIC"'","path":"other/castle","url":"hello"}'
assert_status 400 "reject mismatched topic path"
assert_jq '.error == "`topic` and `path` must match"' "reject mismatched topic path body"
pass "reject mismatched topic path"

api_json POST "$POST_BASE_URL/" '{"topic":"'"$TOPIC"'","path":"castle-notes","url":"# Castle\n\nHello","type":"md2html","title":"Castle Notes"}'
assert_status 201 "create topic item via topic"
assert_jq '.path == "'"$TOPIC"'/castle-notes"' "topic path rewrite"
pass "create topic item via topic"

ITEM_HTML="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ITEM_HTML" "<title>Castle Notes</title>" "topic item title"
assert_contains "$ITEM_HTML" "Back to $TOPIC" "topic item backlink"
pass "topic item render"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'/screening-signup","url":"https://example.com/screening","type":"url"}'
assert_status 201 "create topic item via full path"
pass "create topic item via full path"

FILE_PATH="$TMP_DIR/topic-upload.txt"
printf 'topic-file\n' >"$FILE_PATH"
FILE_BODY="$TMP_DIR/file.body"
FILE_STATUS="$TMP_DIR/file.status"
curl -sS -o "$FILE_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "topic=$TOPIC" \
  -F "path=asset" \
  -F "title=Asset File" \
  "$POST_BASE_URL/" >"$FILE_STATUS"
if [[ "$(cat "$FILE_STATUS")" != "201" ]]; then
  fail "topic file upload" "body: $(cat "$FILE_BODY")"
fi
pass "topic file upload"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after items"
assert_jq '.content == "3"' "topic count after items"
pass "topic count after items"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "Castle Notes" "topic home first item"
assert_contains "$TOPIC_HOME" "screening-signup" "topic home fallback title"
assert_contains "$TOPIC_HOME" "↗" "topic home url mark"
assert_contains "$TOPIC_HOME" "Asset File" "topic home file title"
assert_contains "$TOPIC_HOME" "◫" "topic home file mark"
pass "topic home rebuild"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$TOPIC"'"}'
assert_status 400 "protect topic home delete"
pass "protect topic home delete"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$TOPIC"'/screening-signup"}'
assert_status 200 "delete topic item"
pass "delete topic item"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic count after delete"
assert_jq '.content == "2"' "topic count after delete body"
pass "topic count after delete"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "delete topic"
assert_jq '.content == "2"' "delete topic content count"
pass "delete topic"

ORPHAN="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ORPHAN" "Castle Notes" "orphan item survives topic delete"
pass "orphan item survives topic delete"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "recreate topic"
assert_jq '.content == "2"' "recreate topic adopts orphan"
pass "recreate topic"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup recreated topic"
assert_jq '.content == "2"' "lookup recreated topic count"
pass "lookup recreated topic"

echo "Smoke PLAN3 checks completed successfully."
