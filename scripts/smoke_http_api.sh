#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3012}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/15}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
configure_redis "$REDIS_URL"

SMOKE_PREFIX="smoke-$(date +%s)"

cleanup_http_api_smoke() {
  if [[ -n "${POST_BASE_URL:-}" ]]; then
    curl -sS -X POST \
      -H "Authorization: Bearer $POST_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"path\":\"$SMOKE_PREFIX-file.txt\"}" \
      "$POST_BASE_URL/delete" >/dev/null 2>&1 || true
    curl -sS -X POST \
      -H "Authorization: Bearer $POST_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"path\":\"$SMOKE_PREFIX-file-zero.txt\"}" \
      "$POST_BASE_URL/delete" >/dev/null 2>&1 || true
    curl -sS -X POST \
      -H "Authorization: Bearer $POST_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"path\":\"$SMOKE_PREFIX-file-sh.sh\"}" \
      "$POST_BASE_URL/delete" >/dev/null 2>&1 || true
  fi
  redis_flush >/dev/null 2>&1 || true
  cleanup_smoke_tmp
}

trap cleanup_http_api_smoke EXIT

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api_json POST "$POST_BASE_URL/create" '{"url":"hello"}' no
assert_status 401 "unauthorized create"
assert_jq '.code == "unauthorized"' "unauthorized create code"
pass "unauthorized create"

api_json POST "$POST_BASE_URL/create" '{"url":"hello","path":"'"$SMOKE_PREFIX"' bad"}'
assert_status 400 "invalid path"
assert_jq '.code == "invalid_request"' "invalid path code"
pass "invalid path"

api_json POST "$POST_BASE_URL/create" '{"url":"hello","path":"'"$SMOKE_PREFIX"'-text","title":"Greeting Card"}'
assert_status 201 "create text"
assert_jq '.type == "text"' "create text type"
assert_jq '.path == "'"$SMOKE_PREFIX"'-text"' "create text path"
assert_jq '.title == "Greeting Card"' "create text title"
assert_jq '.created | type == "string"' "create text created"
pass "create text"

RAW_VALUE="$(redis_get "surl:$SMOKE_PREFIX-text")"
assert_contains "$RAW_VALUE" '"type":"text"' "redis json type"
assert_contains "$RAW_VALUE" '"content":"hello"' "redis json content"
assert_contains "$RAW_VALUE" '"title":"Greeting Card"' "redis json title"
pass "redis json storage"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$SMOKE_PREFIX"'-text"}' yes x-export true
assert_status 200 "lookup text"
assert_jq '.content == "hello"' "lookup text export"
assert_jq '.title == "Greeting Card"' "lookup text title"
assert_jq '.created | type == "string"' "lookup text created"
pass "lookup text"

PUBLIC_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-text")"
assert_contains "$PUBLIC_TEXT" "hello" "public text read"
pass "public text read"

api_json POST "$POST_BASE_URL/create" '{"url":"https://example.com/path?q=1","path":"'"$SMOKE_PREFIX"'-link"}'
assert_status 201 "create url"
assert_jq '.type == "url"' "create url type"
assert_jq '.title == ""' "create url empty title"
pass "create url"

REDIRECT_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-link")"
assert_contains "$REDIRECT_HEADERS" "Location: https://example.com/path?q=1" "public url redirect"
pass "public url redirect"

LINK_RAW="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-link?raw")"
if [[ "$LINK_RAW" != "https://example.com/path?q=1" ]]; then
  fail "public url raw" "expected raw url, got: $LINK_RAW"
fi
pass "public url raw"

api_json POST "$POST_BASE_URL/create" '{"url":"example.com/path","path":"'"$SMOKE_PREFIX"'-badurl","type":"url"}'
assert_status 400 "reject invalid url"
assert_jq '.code == "invalid_request"' "reject invalid url code"
pass "reject invalid url"

api_json POST "$POST_BASE_URL/create" '{"url":"alias","path":"'"$SMOKE_PREFIX"'-alias","type":"text","convert":"text"}'
assert_status 201 "matching type convert alias"
pass "matching type convert alias"

api_json POST "$POST_BASE_URL/create" '{"url":"alias","path":"'"$SMOKE_PREFIX"'-alias-bad","type":"text","convert":"html"}'
assert_status 400 "mismatched type convert alias"
assert_jq '.code == "invalid_request"' "mismatched type convert alias code"
pass "mismatched type convert alias"

api_json POST "$POST_BASE_URL/create" '{"url":"# Title\n\nHello from Markdown","path":"'"$SMOKE_PREFIX"'-md","convert":"md2html","title":"Markdown Title"}'
assert_status 201 "create md2html"
assert_jq '.type == "md"' "create md2html type"
assert_jq '.title == "Markdown Title"' "create md2html title"
pass "create md2html"

MD_HTML="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-md")"
assert_contains "$MD_HTML" "<article class=\"markdown-body\">" "rendered html shell"
assert_contains "$MD_HTML" "<h1 id=\"title\">Title</h1>" "rendered markdown heading"
assert_contains "$MD_HTML" "<title>Markdown Title</title>" "rendered markdown title"
assert_contains "$MD_HTML" "/asset/ravel_gfm_css" "rendered markdown uses embedded base asset"
EMBEDDED_ASSET_PATH="$(printf '%s' "$MD_HTML" | rg -o '/asset/[^"]+' -m 1)"
EMBEDDED_ASSET_BODY="$(curl -sS -H "Referer: $POST_BASE_URL/$SMOKE_PREFIX-md" "$POST_BASE_URL$EMBEDDED_ASSET_PATH")"
test -n "$EMBEDDED_ASSET_BODY"
pass "rendered html"

MD_RAW="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-md?raw")"
if [[ "$MD_RAW" != $'# Title\n\nHello from Markdown' ]]; then
  fail "public markdown raw" "expected raw markdown, got: $MD_RAW"
fi
pass "public markdown raw"

api_json POST "$POST_BASE_URL/create" '{"url":"<p>hi</p>","path":"'"$SMOKE_PREFIX"'-html","type":"html"}'
assert_status 201 "create raw html"
assert_jq '.title == ""' "create raw html empty title"
RAW_HTML="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-html")"
assert_contains "$RAW_HTML" "<p>hi</p>" "public html read"
pass "public html read"

HTML_RAW_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-html?raw")"
assert_contains "$HTML_RAW_HEADERS" "Content-Type: text/plain; charset=utf-8" "public html raw content type"
HTML_RAW="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-html?raw")"
if [[ "$HTML_RAW" != "<p>hi</p>" ]]; then
  fail "public html raw" "expected raw html text, got: $HTML_RAW"
fi
pass "public html raw"

api_json POST "$POST_BASE_URL/create" '{"url":"https://example.com/qr","path":"'"$SMOKE_PREFIX"'-qr","convert":"qrcode"}'
assert_status 201 "create qrcode"
assert_jq '.type == "qrcode"' "create qrcode type"
assert_jq '.title == ""' "create qrcode empty title"
pass "create qrcode"

QR_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-qr")"
assert_contains "$QR_TEXT" "Scan this QR code" "public qrcode text"
pass "public qrcode text"

QR_RAW="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-qr?raw")"
if [[ "$QR_RAW" != "https://example.com/qr" ]]; then
  fail "public qrcode raw" "expected qrcode source, got: $QR_RAW"
fi
pass "public qrcode raw"

api_json POST "$POST_BASE_URL/create" '{"url":"first","path":"'"$SMOKE_PREFIX"'-conflict","title":"First Title"}'
assert_status 201 "create conflict seed"
api_json POST "$POST_BASE_URL/create" '{"url":"second","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 409 "detect conflict"
assert_jq '.code == "conflict"' "detect conflict code"
assert_jq '.details.existing.title == "First Title"' "detect conflict existing title"
pass "detect conflict"

api_json POST "$POST_BASE_URL/update" '{"url":"updated","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 200 "overwrite existing"
assert_jq '.overwritten == "first"' "overwrite existing body"
assert_jq '.title == ""' "overwrite existing empty title"
assert_jq '.created | type == "string"' "overwrite existing created"
pass "overwrite existing"

api_json POST "$POST_BASE_URL/create" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl","ttl":0}'
assert_status 201 "ttl zero means infinite"
assert_jq '.ttl == null' "ttl zero means no expiration"
assert_jq '.title == ""' "ttl zero empty title"
assert_jq '.warning == null or .warning == ""' "ttl zero no warning"
pass "ttl zero means infinite"

api_json POST "$POST_BASE_URL/create" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl-decimal","ttl":1.5}'
assert_status 400 "ttl decimal rejected"
assert_jq '.error == "`ttl` must be a natural number"' "ttl decimal rejected message"
pass "ttl decimal rejected"

api_json POST "$POST_BASE_URL/create" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl-string","ttl":"10"}'
assert_status 400 "ttl string rejected"
assert_jq '.error == "`ttl` must be a natural number"' "ttl string rejected message"
pass "ttl string rejected"

api_json POST "$POST_BASE_URL/create" '{"url":"https://example.com/topic","path":"'"$SMOKE_PREFIX"'-ttl-live","ttl":3,"type":"url"}'
assert_status 201 "ttl positive create"
assert_jq '.ttl == 3' "ttl positive create body"
pass "ttl positive create"

api_json POST "$POST_BASE_URL/create" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl-too-large","ttl":525601}'
assert_status 400 "ttl too large rejected"
assert_jq '.error == "`ttl` must be between 0 and 525600 minutes"' "ttl too large rejected message"
pass "ttl too large rejected"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$SMOKE_PREFIX"'-ttl-live"}'
assert_status 200 "lookup ttl positive"
assert_jq '.ttl == 3' "lookup ttl positive body"
pass "lookup ttl positive"

api_json POST "$POST_BASE_URL/query" ''
assert_status 200 "list items"
assert_jq 'map(.path) | index("'"$SMOKE_PREFIX"'-text") != null' "list includes text item"
assert_jq 'map(select(.path == "'"$SMOKE_PREFIX"'-text"))[0].title == "Greeting Card"' "list includes title"
assert_jq 'map(select(.path == "'"$SMOKE_PREFIX"'-text"))[0].created | type == "string"' "list includes created"
pass "list items"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$SMOKE_PREFIX"'-md"}' yes x-export true
assert_status 200 "lookup markdown export"
assert_jq '.type == "md"' "lookup markdown export type"
assert_jq '.content == "# Title\n\nHello from Markdown"' "lookup markdown export body"
assert_jq '.title == "Markdown Title"' "lookup markdown export title"
pass "lookup markdown export"

WILDCARD_PREFIX="$SMOKE_PREFIX-wild-note"
TOPIC_WILDCARD_PREFIX="$SMOKE_PREFIX-wild-topic"

api_json POST "$POST_BASE_URL/create" '{"url":"wild-a-content-body","path":"'"$WILDCARD_PREFIX"'-a","title":"Wild A"}'
assert_status 201 "create wildcard item a"
api_json POST "$POST_BASE_URL/create" '{"url":"wild-b-content-body","path":"'"$WILDCARD_PREFIX"'-b","title":"Wild B"}'
assert_status 201 "create wildcard item b"
api_json POST "$POST_BASE_URL/create" '{"path":"'"$WILDCARD_PREFIX"'-topic","type":"topic"}'
assert_status 201 "create wildcard topic home"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$WILDCARD_PREFIX"'*"}' yes x-export true
assert_status 200 "wildcard get items"
assert_jq 'length == 2' "wildcard get items count"
assert_jq 'map(.path) | index("'"$WILDCARD_PREFIX"'-a") != null' "wildcard get includes a"
assert_jq 'map(.path) | index("'"$WILDCARD_PREFIX"'-b") != null' "wildcard get includes b"
assert_jq 'map(.path) | index("'"$WILDCARD_PREFIX"'-topic") == null' "wildcard get excludes topic home"
assert_jq 'map(select(.path == "'"$WILDCARD_PREFIX"'-a"))[0].content == "wild-a-content-body"' "wildcard get export content a"
pass "wildcard get items"

api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC_WILDCARD_PREFIX"'-a","type":"topic","title":"Topic A"}'
assert_status 201 "create wildcard topic a"
api_json POST "$POST_BASE_URL/create" '{"path":"'"$TOPIC_WILDCARD_PREFIX"'-b","type":"topic","title":"Topic B"}'
assert_status 201 "create wildcard topic b"
api_json POST "$POST_BASE_URL/create" '{"topic":"'"$TOPIC_WILDCARD_PREFIX"'-a","path":"entry","url":"topic-a-child","type":"text"}'
assert_status 201 "create wildcard topic child a"
api_json POST "$POST_BASE_URL/create" '{"topic":"'"$TOPIC_WILDCARD_PREFIX"'-b","path":"entry","url":"topic-b-child","type":"text"}'
assert_status 201 "create wildcard topic child b"

api_json POST "$POST_BASE_URL/query" '{"path":"'"$TOPIC_WILDCARD_PREFIX"'*","type":"topic"}'
assert_status 200 "wildcard get topics"
assert_jq 'length == 2' "wildcard get topics count"
assert_jq 'map(.path) | index("'"$TOPIC_WILDCARD_PREFIX"'-a") != null' "wildcard get includes topic a"
assert_jq 'map(.path) | index("'"$TOPIC_WILDCARD_PREFIX"'-b") != null' "wildcard get includes topic b"
assert_jq 'all(.[]; .type == "topic")' "wildcard get topics type"
pass "wildcard get topics"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$WILDCARD_PREFIX"'*"}'
assert_status 200 "wildcard delete items"
assert_jq '.errors == []' "wildcard delete items errors"
assert_jq '.deleted | length == 2' "wildcard delete items count"
assert_jq '.deleted | map(.deleted) | index("'"$WILDCARD_PREFIX"'-a") != null' "wildcard delete includes a"
assert_jq '.deleted | map(.deleted) | index("'"$WILDCARD_PREFIX"'-b") != null' "wildcard delete includes b"
if [[ "$(redis_exists "surl:$WILDCARD_PREFIX-a")" != "0" ]]; then
  fail "wildcard delete removed item a" "exists: $(redis_exists "surl:$WILDCARD_PREFIX-a")"
fi
if [[ "$(redis_exists "surl:$WILDCARD_PREFIX-topic")" != "1" ]]; then
  fail "wildcard delete keeps topic home" "exists: $(redis_exists "surl:$WILDCARD_PREFIX-topic")"
fi
pass "wildcard delete items"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$TOPIC_WILDCARD_PREFIX"'*","type":"topic"}'
assert_status 200 "wildcard delete topics"
assert_jq '.errors == []' "wildcard delete topics errors"
assert_jq '.deleted | length == 2' "wildcard delete topics count"
assert_jq '.deleted | map(.deleted) | index("'"$TOPIC_WILDCARD_PREFIX"'-a") != null' "wildcard delete includes topic a"
assert_jq '.deleted | map(.deleted) | index("'"$TOPIC_WILDCARD_PREFIX"'-b") != null' "wildcard delete includes topic b"
TOPIC_CHILD_A="$(curl -sS "$POST_BASE_URL/$TOPIC_WILDCARD_PREFIX-a/entry")"
assert_contains "$TOPIC_CHILD_A" "topic-a-child" "wildcard delete topic keeps child a"
TOPIC_CHILD_B="$(curl -sS "$POST_BASE_URL/$TOPIC_WILDCARD_PREFIX-b/entry")"
assert_contains "$TOPIC_CHILD_B" "topic-b-child" "wildcard delete topic keeps child b"
pass "wildcard delete topics"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$SMOKE_PREFIX"'-missing"}'
assert_status 404 "delete missing"
assert_jq '.code == "not_found"' "delete missing code"
pass "delete missing"

FILE_PATH="$TMP_DIR/upload.txt"
printf 'upload-body\n' >"$FILE_PATH"
FILE_BODY="$TMP_DIR/file.body"
FILE_STATUS="$TMP_DIR/file.status"
curl -sS -o "$FILE_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "path=$SMOKE_PREFIX-file" \
  -F "title=Upload Attachment" \
  "$POST_BASE_URL/create" >"$FILE_STATUS"
FILE_HTTP_STATUS="$(cat "$FILE_STATUS")"
FILE_HTTP_BODY="$(cat "$FILE_BODY")"
if [[ "$FILE_HTTP_STATUS" != "201" ]]; then
  fail "file upload" "expected HTTP 201, got $FILE_HTTP_STATUS, body: $FILE_HTTP_BODY"
fi
if ! jq -e '.type == "file"' >/dev/null <<<"$FILE_HTTP_BODY"; then
  fail "file upload type" "body: $FILE_HTTP_BODY"
fi
if ! jq -e '.title == "Upload Attachment"' >/dev/null <<<"$FILE_HTTP_BODY"; then
  fail "file upload title" "body: $FILE_HTTP_BODY"
fi
FILE_EXT_VALUE="$(redis_get "surl:$SMOKE_PREFIX-file.txt")"
assert_contains "$FILE_EXT_VALUE" '"type":"file"' "redis file json type"
assert_contains "$FILE_EXT_VALUE" '"title":"Upload Attachment"' "redis file title"
pass "file upload"

FILE_OBJECT_KEY="$(jq -r '.content' <<<"$FILE_EXT_VALUE")"
FILE_RAW="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file.txt?raw")"
if [[ "$FILE_RAW" != "$FILE_OBJECT_KEY" ]]; then
  fail "public file raw" "expected object key $FILE_OBJECT_KEY, got: $FILE_RAW"
fi
pass "public file raw"

FILE_AUTO_BODY="$TMP_DIR/file-auto.body"
FILE_AUTO_STATUS="$TMP_DIR/file-auto.status"
curl -sS -o "$FILE_AUTO_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH" \
  -F "path=$SMOKE_PREFIX-file-auto" \
  "$POST_BASE_URL/create" >"$FILE_AUTO_STATUS"
FILE_AUTO_HTTP_STATUS="$(cat "$FILE_AUTO_STATUS")"
FILE_AUTO_HTTP_BODY="$(cat "$FILE_AUTO_BODY")"
if [[ "$FILE_AUTO_HTTP_STATUS" != "201" ]]; then
  fail "file upload auto content type" "expected HTTP 201, got $FILE_AUTO_HTTP_STATUS, body: $FILE_AUTO_HTTP_BODY"
fi
if ! jq -e '.type == "file"' >/dev/null <<<"$FILE_AUTO_HTTP_BODY"; then
  fail "file upload auto content type type" "body: $FILE_AUTO_HTTP_BODY"
fi
FILE_AUTO_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-file-auto.txt")"
assert_contains "$FILE_AUTO_HEADERS" "Content-Type: text/plain" "file upload auto content type header"
FILE_AUTO_PUBLIC_BODY="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file-auto.txt")"
assert_contains "$FILE_AUTO_PUBLIC_BODY" "upload-body" "file upload auto content type body"
api_json POST "$POST_BASE_URL/delete" '{"path":"'"$SMOKE_PREFIX"'-file-auto.txt"}'
assert_status 200 "delete auto content type file"
assert_jq '.type == "file"' "delete auto content type file type"
curl -sS -o "$TMP_DIR/file-auto-delete.body" -w "%{http_code}" "$POST_BASE_URL/$SMOKE_PREFIX-file-auto.txt" >"$TMP_DIR/file-auto-delete.status"
if [[ "$(cat "$TMP_DIR/file-auto-delete.status")" != "404" ]]; then
  fail "delete auto content type file public lookup" "expected HTTP 404 after delete, got $(cat "$TMP_DIR/file-auto-delete.status"), body: $(cat "$TMP_DIR/file-auto-delete.body")"
fi
pass "file upload auto content type"

FILE_OCTET_BODY="$TMP_DIR/file-octet.body"
FILE_OCTET_STATUS="$TMP_DIR/file-octet.status"
curl -sS -o "$FILE_OCTET_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=application/octet-stream" \
  -F "path=$SMOKE_PREFIX-file-octet" \
  "$POST_BASE_URL/create" >"$FILE_OCTET_STATUS"
FILE_OCTET_HTTP_STATUS="$(cat "$FILE_OCTET_STATUS")"
FILE_OCTET_HTTP_BODY="$(cat "$FILE_OCTET_BODY")"
if [[ "$FILE_OCTET_HTTP_STATUS" != "201" ]]; then
  fail "file upload octet stream repair" "expected HTTP 201, got $FILE_OCTET_HTTP_STATUS, body: $FILE_OCTET_HTTP_BODY"
fi
FILE_OCTET_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-file-octet.txt")"
assert_contains "$FILE_OCTET_HEADERS" "Content-Type: text/plain; charset=utf-8" "file upload octet stream repair header"
FILE_OCTET_PUBLIC_BODY="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file-octet.txt")"
assert_contains "$FILE_OCTET_PUBLIC_BODY" "upload-body" "file upload octet stream repair body"
api_json POST "$POST_BASE_URL/delete" '{"path":"'"$SMOKE_PREFIX"'-file-octet.txt"}'
assert_status 200 "delete octet stream repair file"
assert_jq '.type == "file"' "delete octet stream repair file type"
curl -sS -o "$TMP_DIR/file-octet-delete.body" -w "%{http_code}" "$POST_BASE_URL/$SMOKE_PREFIX-file-octet.txt" >"$TMP_DIR/file-octet-delete.status"
if [[ "$(cat "$TMP_DIR/file-octet-delete.status")" != "404" ]]; then
  fail "delete octet stream repair file public lookup" "expected HTTP 404 after delete, got $(cat "$TMP_DIR/file-octet-delete.status"), body: $(cat "$TMP_DIR/file-octet-delete.body")"
fi
pass "file upload octet stream repair"

FILE_SH_PATH="$TMP_DIR/upload.sh"
printf '#!/bin/sh\necho upload-body\n' >"$FILE_SH_PATH"
FILE_SH_BODY="$TMP_DIR/file-sh.body"
FILE_SH_STATUS="$TMP_DIR/file-sh.status"
curl -sS -o "$FILE_SH_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_SH_PATH;type=application/x-sh" \
  -F "path=$SMOKE_PREFIX-file-sh" \
  "$POST_BASE_URL/create" >"$FILE_SH_STATUS"
FILE_SH_HTTP_STATUS="$(cat "$FILE_SH_STATUS")"
FILE_SH_HTTP_BODY="$(cat "$FILE_SH_BODY")"
if [[ "$FILE_SH_HTTP_STATUS" != "201" ]]; then
  fail "file upload shell charset" "expected HTTP 201, got $FILE_SH_HTTP_STATUS, body: $FILE_SH_HTTP_BODY"
fi
FILE_SH_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-file-sh.sh")"
assert_contains "$FILE_SH_HEADERS" "Content-Type: application/x-sh; charset=utf-8" "file upload shell charset header"
FILE_SH_PUBLIC_BODY="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file-sh.sh")"
assert_contains "$FILE_SH_PUBLIC_BODY" "upload-body" "file upload shell charset body"
api_json POST "$POST_BASE_URL/delete" '{"path":"'"$SMOKE_PREFIX"'-file-sh.sh"}'
assert_status 200 "delete shell charset file"
assert_jq '.type == "file"' "delete shell charset file type"
pass "file upload shell charset"

FILE_ZERO_BODY="$TMP_DIR/file-zero.body"
FILE_ZERO_STATUS="$TMP_DIR/file-zero.status"
curl -sS -o "$FILE_ZERO_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "path=$SMOKE_PREFIX-file-zero" \
  -F "ttl=0" \
  "$POST_BASE_URL/create" >"$FILE_ZERO_STATUS"
FILE_ZERO_HTTP_STATUS="$(cat "$FILE_ZERO_STATUS")"
FILE_ZERO_HTTP_BODY="$(cat "$FILE_ZERO_BODY")"
if [[ "$FILE_ZERO_HTTP_STATUS" != "201" ]]; then
  fail "file upload ttl zero" "expected HTTP 201, got $FILE_ZERO_HTTP_STATUS, body: $FILE_ZERO_HTTP_BODY"
fi
if ! jq -e '.ttl == null' >/dev/null <<<"$FILE_ZERO_HTTP_BODY"; then
  fail "file upload ttl zero ttl" "body: $FILE_ZERO_HTTP_BODY"
fi
if ! jq -e '.title == ""' >/dev/null <<<"$FILE_ZERO_HTTP_BODY"; then
  fail "file upload ttl zero title" "body: $FILE_ZERO_HTTP_BODY"
fi
pass "file upload ttl zero"

FILE_BAD_TTL_BODY="$TMP_DIR/file-bad-ttl.body"
FILE_BAD_TTL_STATUS="$TMP_DIR/file-bad-ttl.status"
curl -sS -o "$FILE_BAD_TTL_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "path=$SMOKE_PREFIX-file-bad-ttl" \
  -F "ttl=1.5" \
  "$POST_BASE_URL/create" >"$FILE_BAD_TTL_STATUS"
if [[ "$(cat "$FILE_BAD_TTL_STATUS")" != "400" ]]; then
  fail "file upload bad ttl" "body: $(cat "$FILE_BAD_TTL_BODY")"
fi
if ! jq -e '.error == "`ttl` must be a natural number"' >/dev/null <<<"$(cat "$FILE_BAD_TTL_BODY")"; then
  fail "file upload bad ttl message" "body: $(cat "$FILE_BAD_TTL_BODY")"
fi
pass "file upload bad ttl"

FILE_TOO_LARGE_TTL_BODY="$TMP_DIR/file-too-large-ttl.body"
FILE_TOO_LARGE_TTL_STATUS="$TMP_DIR/file-too-large-ttl.status"
curl -sS -o "$FILE_TOO_LARGE_TTL_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "path=$SMOKE_PREFIX-file-too-large-ttl" \
  -F "ttl=525601" \
  "$POST_BASE_URL/create" >"$FILE_TOO_LARGE_TTL_STATUS"
if [[ "$(cat "$FILE_TOO_LARGE_TTL_STATUS")" != "400" ]]; then
  fail "file upload ttl too large" "body: $(cat "$FILE_TOO_LARGE_TTL_BODY")"
fi
if ! jq -e '.error == "`ttl` must be between 0 and 525600 minutes"' >/dev/null <<<"$(cat "$FILE_TOO_LARGE_TTL_BODY")"; then
  fail "file upload ttl too large message" "body: $(cat "$FILE_TOO_LARGE_TTL_BODY")"
fi
pass "file upload ttl too large"

FILE_PUBLIC="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file.txt")"
assert_contains "$FILE_PUBLIC" "upload-body" "public file read"
pass "public file read"

MISSING_FILE_BODY="$TMP_DIR/file-missing.body"
MISSING_FILE_STATUS="$TMP_DIR/file-missing.status"
curl -sS -o "$MISSING_FILE_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=$SMOKE_PREFIX-missing-file" \
  "$POST_BASE_URL/create" >"$MISSING_FILE_STATUS"
if [[ "$(cat "$MISSING_FILE_STATUS")" != "400" ]]; then
  fail "missing file field" "body: $(cat "$MISSING_FILE_BODY")"
fi
pass "missing file field"

api_json POST "$POST_BASE_URL/delete" '{"path":"'"$SMOKE_PREFIX"'-conflict"}' yes x-export true
assert_status 200 "delete existing"
assert_jq '.deleted == "'"$SMOKE_PREFIX"'-conflict"' "delete existing path"
assert_jq '.content == "updated"' "delete existing content"
assert_jq '.title == ""' "delete existing title"
pass "delete existing"

echo "HTTP API smoke checks completed successfully."
