#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3013}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/14}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
configure_redis "$REDIS_URL"

TOPIC="anime-$(date +%s)"
TOPIC_FILE_PATH="$TOPIC/asset.txt"

cleanup_topic_api_smoke() {
  if [[ -n "${POST_BASE_URL:-}" ]]; then
    curl -sS -X POST \
      -H "Authorization: Bearer $POST_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"path\":\"$TOPIC_FILE_PATH\"}" \
      "$POST_BASE_URL/delete" >/dev/null 2>&1 || true
  fi
  redis_flush >/dev/null 2>&1 || true
  cleanup_smoke_tmp
}

trap cleanup_topic_api_smoke EXIT

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC"'","convert":"topic","title":"Anime Archive"}'
assert_status 201 "create topic via alias"
assert_jq '.type == "topic"' "create topic type"
assert_jq '.content == "0"' "create topic count"
assert_jq '.title == "Anime Archive"' "create topic title"
assert_jq '.created | type == "string"' "create topic created"
pass "create topic via alias"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
assert_contains "$TOPIC_RAW" '"type":"topic"' "topic stored type"
pass "topic stored type"
if ! jq -e '.content | contains("<div style=\"font-size: 1.3em; font-weight: bold\">Anime Archive</div>") and contains("<span style=\"color: #666;\">Home</span>") and (contains("<html") | not) and (contains("<body") | not) and (contains("post-footer") | not)' >/dev/null <<<"$TOPIC_RAW"; then
  fail "topic stored markdown" "value: $TOPIC_RAW"
fi
pass "topic stored markdown"

if [[ "$(redis_type "topic:$TOPIC:items")" != "zset" ]]; then
  fail "topic items key exists" "type: $(redis_type "topic:$TOPIC:items")"
fi
pass "topic items key exists"

api_json POST "$POST_BASE_URL/query" '{"type":"topic"}'
assert_status 200 "topic list"
assert_jq 'map(.path) | index("'"$TOPIC"'") != null' "topic list contains created topic"
assert_jq 'map(select(.path == "'"$TOPIC"'"))[0].title == "Anime Archive"' "topic list title"
pass "topic list"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","convert":"topic"}'
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

TOPIC_HOME_RAW="$(curl -sS "$POST_BASE_URL/$TOPIC?raw")"
assert_contains "$TOPIC_HOME_RAW" "<title>Anime Archive</title>" "topic home raw still renders"
TOPIC_HOME_RAW_HEADERS="$(curl -sSI "$POST_BASE_URL/$TOPIC?raw")"
assert_contains "$TOPIC_HOME_RAW_HEADERS" "Content-Type: text/html; charset=utf-8" "topic home raw keeps html content type"
assert_contains "$TOPIC_HOME_RAW_HEADERS" "Cache-Control: public, max-age=600, s-maxage=600" "topic home raw keeps cache"
pass "topic home raw still renders"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC"'","type":"topic","ttl":10}'
assert_status 400 "reject topic ttl"
assert_jq '.error == "topic does not support ttl"' "reject topic ttl body"
pass "reject topic ttl"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC"'","url":"hello"}'
assert_status 400 "protect topic home create"
pass "protect topic home create"

api_json POST "$POST_BASE_URL/create" '{"topic":"missing-'"$TOPIC"'","path":"x","url":"hello"}'
assert_status 400 "reject missing topic"
assert_jq '.error == "topic does not exist"' "reject missing topic body"
pass "reject missing topic"

api_json POST "$POST_BASE_URL/create" '{"topic":"'"$TOPIC"'","path":"other/castle","url":"hello"}'
assert_status 400 "reject mismatched topic path"
assert_jq '.error == "`topic` and `path` must match"' "reject mismatched topic path body"
pass "reject mismatched topic path"

api_json POST "$POST_BASE_URL/create" '{"topic":"'"$TOPIC"'","path":"castle-notes","url":"# Castle\n\nHello","type":"md2html","title":"Castle Notes","created":"2022-10-11"}'
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

ITEM_RAW="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes?raw")"
if [[ "$ITEM_RAW" != $'# Castle\n\nHello' ]]; then
  fail "topic item raw" "expected raw topic item markdown, got: $ITEM_RAW"
fi
pass "topic item raw"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC"'/screening-signup","url":"https://example.com/screening","type":"url","created":"2023-10-11"}'
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
  "$POST_BASE_URL/create" >"$FILE_STATUS"
if [[ "$(cat "$FILE_STATUS")" != "201" ]]; then
  fail "topic file upload" "body: $(cat "$FILE_BODY")"
fi
if ! jq -e '.title == "Asset File"' >/dev/null <<<"$(cat "$FILE_BODY")"; then
  fail "topic file upload title" "body: $(cat "$FILE_BODY")"
fi
pass "topic file upload"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after items"
assert_jq '.content == "3"' "topic count after items"
pass "topic count after items"

api_json POST "$POST_BASE_URL/update" '{"path":"'"$TOPIC"'","type":"topic","title":"Anime Notes"}'
assert_status 200 "update topic title"
assert_jq '.title == "Anime Notes"' "update topic title body"
pass "update topic title"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic lookup after title update"
assert_jq '.title == "Anime Notes"' "topic lookup after title update body"
pass "topic lookup after title update"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
if ! jq -e '.content | contains("<div style=\"font-size: 1.3em; font-weight: bold\">Anime Notes</div>") and contains("[Castle Notes](</'"$TOPIC"'/castle-notes>)") and contains("[screening-signup](</'"$TOPIC"'/screening-signup>) ↗") and contains("[Asset File](</'"$TOPIC"'/asset.txt>) ◫") and (contains("<html") | not) and (contains("<body") | not) and (contains("post-footer") | not)' >/dev/null <<<"$TOPIC_RAW"; then
  fail "topic rebuilt stored markdown" "value: $TOPIC_RAW"
fi
pass "topic rebuilt stored markdown"

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

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$TOPIC"'"}'
assert_status 400 "protect topic home delete"
pass "protect topic home delete"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$TOPIC"'/screening-signup"}'
assert_status 200 "delete topic item"
pass "delete topic item"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "topic count after delete"
assert_jq '.content == "2"' "topic count after delete body"
pass "topic count after delete"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "delete topic"
assert_jq '.content == "2"' "delete topic content count"
assert_jq '.title == "Anime Notes"' "delete topic title"
pass "delete topic"

ORPHAN="$(curl -sS "$POST_BASE_URL/$TOPIC/castle-notes")"
assert_contains "$ORPHAN" "Castle Notes" "orphan item survives topic delete"
pass "orphan item survives topic delete"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 201 "recreate topic"
assert_jq '.content == "2"' "recreate topic adopts orphan"
assert_jq '.title == "'"$TOPIC"'"' "recreate topic title"
pass "recreate topic"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup recreated topic"
assert_jq '.content == "2"' "lookup recreated topic count"
pass "lookup recreated topic"

redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" DEL "surl:$TOPIC/castle-notes" >/dev/null
api_json POST "$POST_BASE_URL/update" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "refresh topic cleans stale members"
assert_jq '.content == "1"' "refresh topic cleans stale members body"
pass "refresh topic cleans stale members"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "lookup refreshed topic count"
assert_jq '.content == "1"' "lookup refreshed topic count body"
pass "lookup refreshed topic count"

NESTED_TOPIC="$TOPIC/2026"
api_json POST "$POST_BASE_URL/create" '{"path":"'"$NESTED_TOPIC"'","type":"topic","title":"Anime Archive 2026"}'
assert_status 201 "create nested topic"
assert_jq '.title == "Anime Archive 2026"' "create nested topic title"
pass "create nested topic"

PARENT_TOPIC_MEMBERS="$(redis_zrange "topic:$TOPIC:items")"
assert_contains "$PARENT_TOPIC_MEMBERS" "2026" "parent topic indexes nested topic"
pass "parent topic indexes nested topic"

api_json POST "$POST_BASE_URL/create" '{"topic":"'"$TOPIC"'","path":"'"$NESTED_TOPIC"'/wrong-parent","url":"bad","type":"text"}'
assert_status 400 "reject explicit parent topic for nested topic item"
assert_jq '.code == "invalid_request" and .error == "`path` belongs to a nested topic"' "reject explicit parent topic for nested topic item body"
pass "reject explicit parent topic for nested topic item"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$NESTED_TOPIC"'/post-1","url":"nested body","type":"text"}'
assert_status 201 "nested topic full path matches longest prefix"
assert_jq '.path == "'"$NESTED_TOPIC"'/post-1"' "nested topic full path response"
pass "nested topic full path matches longest prefix"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$NESTED_TOPIC"'","type":"topic"}'
assert_status 200 "nested topic count after full path create"
assert_jq '.content == "1"' "nested topic count after full path create body"
pass "nested topic count after full path create"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "parent topic count includes nested topic only"
assert_jq '.content == "2"' "parent topic count includes nested topic only body"
pass "parent topic count includes nested topic only"

api_json POST "$POST_BASE_URL/update" '{"path":"'"$TOPIC"'","type":"topic"}'
assert_status 200 "refresh parent topic skips nested topic item"
assert_jq '.content == "2"' "refresh parent topic skips nested topic item body"
pass "refresh parent topic skips nested topic item"

PARENT_TOPIC_MEMBERS="$(redis_zrange "topic:$TOPIC:items")"
assert_contains "$PARENT_TOPIC_MEMBERS" "2026" "parent topic keeps nested topic member"
if [[ "$PARENT_TOPIC_MEMBERS" == *"2026/post-1"* ]]; then
  fail "parent topic excludes nested topic item member" "members: $PARENT_TOPIC_MEMBERS"
fi
pass "parent topic excludes nested topic item member"

TOPIC_RAW="$(redis_get "surl:$TOPIC")"
if ! jq -e '.content | contains("[Anime Archive 2026](</'"$NESTED_TOPIC"'>) §") and (contains("post-1") | not)' >/dev/null <<<"$TOPIC_RAW"; then
  fail "parent topic markdown skips nested topic item" "value: $TOPIC_RAW"
fi
pass "parent topic markdown skips nested topic item"

echo "Topic API smoke checks completed successfully."
