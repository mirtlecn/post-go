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

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","convert":"topic","title":"Anime Archive"}'
assert_status 201 "create topic via alias"
assert_jq '.type == "topic"' "create topic type"
assert_jq '.content == "0"' "create topic count"
assert_jq '.title == "Anime Archive"' "create topic title"
assert_jq '.created | type == "string"' "create topic created"
pass "create topic via alias"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
assert_contains "$TOPIC_RAW" '"type":"topic"' "topic stored type"
pass "topic stored type"

if [[ "$(redis_type "topic:$TOPIC:items")" != "zset" ]]; then
  fail "topic items key exists" "type: $(redis_type "topic:$TOPIC:items")"
fi
pass "topic items key exists"

api_json GET "$POST_BASE_URL/" '{"type":"topic"}'
assert_status 200 "topic list"
assert_jq 'map(.path) | index("'"$TOPIC"'") != null' "topic list contains created topic"
assert_jq 'map(select(.path == "'"$TOPIC"'"))[0].title == "Anime Archive"' "topic list title"
pass "topic list"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","convert":"topic"}'
assert_status 200 "topic lookup"
assert_jq '.type == "topic"' "topic lookup type"
assert_jq '.content == "0"' "topic lookup count"
assert_jq '.title == "Anime Archive"' "topic lookup title"
assert_jq '.created | type == "string"' "topic lookup created"
pass "topic lookup"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "<title>Anime Archive</title>" "topic home title"
assert_contains "$TOPIC_HOME" "<div style=\"font-size: 1.3em; font-weight: bold\">Anime Archive</div>" "topic home heading"
pass "topic home render"

TOPIC_HOME_HEADERS="$(curl -sSI "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME_HEADERS" "Cache-Control: public, max-age=600, s-maxage=600" "topic home cache"
pass "topic home cache"

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

api_json POST "$POST_BASE_URL/" '{"topic":"'"$TOPIC"'","path":"castle-notes","url":"# Castle\n\nHello","type":"md2html","title":"Castle Notes","created":"2022-10-11"}'
assert_status 201 "create topic item via topic"
assert_jq '.path == "'"$TOPIC"'/castle-notes"' "topic path rewrite"
assert_jq '.title == "Castle Notes"' "create topic item title"
assert_jq '.created == "2022-10-10T16:00:00Z"' "create topic item created"
pass "create topic item via topic"

ITEM_HTML="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ITEM_HTML" "<title>Castle Notes</title>" "topic item title"
assert_contains "$ITEM_HTML" "<div style=\"font-size: 1.3em; font-weight: bold\">Anime Archive</div>" "topic item header"
assert_contains "$ITEM_HTML" "href=\"/$TOPIC\"" "topic item backlink href"
assert_contains "$ITEM_HTML" "<strong>Home</strong>" "topic item home link label"
pass "topic item render"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'/screening-signup","url":"https://example.com/screening","type":"url","created":"2023-10-11"}'
assert_status 201 "create topic item via full path"
assert_jq '.title == ""' "create topic item full path empty title"
assert_jq '.created == "2023-10-10T16:00:00Z"' "create topic item full path created"
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
if ! jq -e '.title == "Asset File"' >/dev/null <<<"$(cat "$FILE_BODY")"; then
  fail "topic file upload title" "body: $(cat "$FILE_BODY")"
fi
pass "topic file upload"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after items"
assert_jq '.content == "3"' "topic count after items"
pass "topic count after items"

api_json PUT "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic","title":"Anime Notes"}'
assert_status 200 "update topic title"
assert_jq '.title == "Anime Notes"' "update topic title body"
pass "update topic title"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after title update"
assert_jq '.title == "Anime Notes"' "topic lookup after title update body"
pass "topic lookup after title update"

TOPIC_HOME="$(curl -sS "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME" "<title>Anime Notes</title>" "topic home updated title"
assert_contains "$TOPIC_HOME" "<div style=\"font-size: 1.3em; font-weight: bold\">Anime Notes</div>" "topic home updated heading"
assert_contains "$TOPIC_HOME" "screening-signup" "topic home fallback title"
assert_contains "$TOPIC_HOME" "↗" "topic home url mark"
assert_contains "$TOPIC_HOME" "Asset File" "topic home file title"
assert_contains "$TOPIC_HOME" "◫" "topic home file mark"
assert_contains "$TOPIC_HOME" "href=\"/$TOPIC/castle-notes\"" "topic home absolute href"
if [[ "$(printf '%s' "$TOPIC_HOME" | grep -bo 'screening-signup' | head -n1 | cut -d: -f1)" -ge "$(printf '%s' "$TOPIC_HOME" | grep -bo 'Castle Notes' | head -n1 | cut -d: -f1)" ]]; then
  fail "topic home created order" "expected screening-signup before Castle Notes"
fi
pass "topic home rebuild"

TOPIC_HOME_HEADERS="$(curl -sSI "$POST_BASE_URL/$TOPIC")"
assert_contains "$TOPIC_HOME_HEADERS" "Cache-Control: public, max-age=600, s-maxage=600" "topic home cache after rebuild"
pass "topic home cache after rebuild"

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
assert_jq '.title == "Anime Notes"' "delete topic title"
pass "delete topic"

ORPHAN="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ORPHAN" "Castle Notes" "orphan item survives topic delete"
pass "orphan item survives topic delete"

api_json POST "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "recreate topic"
assert_jq '.content == "2"' "recreate topic adopts orphan"
assert_jq '.title == "'"$TOPIC"'"' "recreate topic title"
pass "recreate topic"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup recreated topic"
assert_jq '.content == "2"' "lookup recreated topic count"
pass "lookup recreated topic"

redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" DEL "surl:$TOPIC/castle-notes" >/dev/null
api_json PUT "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "refresh topic cleans stale members"
assert_jq '.content == "1"' "refresh topic cleans stale members body"
pass "refresh topic cleans stale members"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup refreshed topic count"
assert_jq '.content == "1"' "lookup refreshed topic count body"
pass "lookup refreshed topic count"

NESTED_TOPIC="$TOPIC/2026"
api_json POST "$POST_BASE_URL/" '{"path":"'"$NESTED_TOPIC"'","type":"topic","title":"Anime Archive 2026"}'
assert_status 201 "create nested topic"
assert_jq '.title == "Anime Archive 2026"' "create nested topic title"
pass "create nested topic"

api_json POST "$POST_BASE_URL/" '{"path":"'"$NESTED_TOPIC"'/post-1","url":"nested body","type":"text"}'
assert_status 201 "nested topic full path matches longest prefix"
assert_jq '.path == "'"$NESTED_TOPIC"'/post-1"' "nested topic full path response"
pass "nested topic full path matches longest prefix"

api_json GET "$POST_BASE_URL/" '{"path":"'"$NESTED_TOPIC"'","type":"topic"}'
assert_status 200 "nested topic count after full path create"
assert_jq '.content == "1"' "nested topic count after full path create body"
pass "nested topic count after full path create"

api_json GET "$POST_BASE_URL/" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "parent topic count excludes nested topic item"
assert_jq '.content == "1"' "parent topic count excludes nested topic item body"
pass "parent topic count excludes nested topic item"

echo "Topic API smoke checks completed successfully."
