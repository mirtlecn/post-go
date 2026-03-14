#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3012}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/15}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
trap cleanup_smoke_tmp EXIT
configure_redis "$REDIS_URL"

SMOKE_PREFIX="smoke-$(date +%s)"

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

redis_flush

api_json POST "$POST_BASE_URL/" '{"url":"hello"}' no
assert_status 401 "unauthorized create"
assert_jq '.code == "unauthorized"' "unauthorized create code"
pass "unauthorized create"

api_json POST "$POST_BASE_URL/" '{"url":"hello","path":"'"$SMOKE_PREFIX"' bad"}'
assert_status 400 "invalid path"
assert_jq '.code == "invalid_request"' "invalid path code"
pass "invalid path"

api_json POST "$POST_BASE_URL/" '{"url":"hello","path":"'"$SMOKE_PREFIX"'-text","title":"Greeting Card"}'
assert_status 201 "create text"
assert_jq '.type == "text"' "create text type"
assert_jq '.path == "'"$SMOKE_PREFIX"'-text"' "create text path"
assert_jq '.title == "Greeting Card"' "create text title"
pass "create text"

RAW_VALUE="$(redis_get "surl:$SMOKE_PREFIX-text")"
assert_contains "$RAW_VALUE" '"type":"text"' "redis json type"
assert_contains "$RAW_VALUE" '"content":"hello"' "redis json content"
assert_contains "$RAW_VALUE" '"title":"Greeting Card"' "redis json title"
pass "redis json storage"

api_json GET "$POST_BASE_URL/" '{"path":"'"$SMOKE_PREFIX"'-text"}' yes x-export true
assert_status 200 "lookup text"
assert_jq '.content == "hello"' "lookup text export"
assert_jq '.title == "Greeting Card"' "lookup text title"
pass "lookup text"

PUBLIC_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-text")"
assert_contains "$PUBLIC_TEXT" "hello" "public text read"
pass "public text read"

api_json POST "$POST_BASE_URL/" '{"url":"https://example.com/path?q=1","path":"'"$SMOKE_PREFIX"'-link"}'
assert_status 201 "create url"
assert_jq '.type == "url"' "create url type"
assert_jq '.title == ""' "create url empty title"
pass "create url"

REDIRECT_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-link")"
assert_contains "$REDIRECT_HEADERS" "Location: https://example.com/path?q=1" "public url redirect"
pass "public url redirect"

api_json POST "$POST_BASE_URL/" '{"url":"example.com/path","path":"'"$SMOKE_PREFIX"'-badurl","type":"url"}'
assert_status 400 "reject invalid url"
assert_jq '.code == "invalid_request"' "reject invalid url code"
pass "reject invalid url"

api_json POST "$POST_BASE_URL/" '{"url":"alias","path":"'"$SMOKE_PREFIX"'-alias","type":"text","convert":"text"}'
assert_status 201 "matching type convert alias"
pass "matching type convert alias"

api_json POST "$POST_BASE_URL/" '{"url":"alias","path":"'"$SMOKE_PREFIX"'-alias-bad","type":"text","convert":"html"}'
assert_status 400 "mismatched type convert alias"
assert_jq '.code == "invalid_request"' "mismatched type convert alias code"
pass "mismatched type convert alias"

api_json POST "$POST_BASE_URL/" '{"url":"# Title\n\nHello from Markdown","path":"'"$SMOKE_PREFIX"'-md","convert":"md2html","title":"Markdown Title"}'
assert_status 201 "create md2html"
assert_jq '.type == "html"' "create md2html type"
assert_jq '.title == "Markdown Title"' "create md2html title"
pass "create md2html"

MD_HTML="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-md")"
assert_contains "$MD_HTML" "<article class=\"markdown-body\">" "rendered html shell"
assert_contains "$MD_HTML" "<h1 id=\"title\">Title</h1>" "rendered markdown heading"
assert_contains "$MD_HTML" "<title></title>" "rendered markdown title empty"
pass "rendered html"

api_json POST "$POST_BASE_URL/" '{"url":"<p>hi</p>","path":"'"$SMOKE_PREFIX"'-html","type":"html"}'
assert_status 201 "create raw html"
assert_jq '.title == ""' "create raw html empty title"
RAW_HTML="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-html")"
assert_contains "$RAW_HTML" "<p>hi</p>" "public html read"
pass "public html read"

api_json POST "$POST_BASE_URL/" '{"url":"https://example.com/qr","path":"'"$SMOKE_PREFIX"'-qr","convert":"qrcode"}'
assert_status 201 "create qrcode"
assert_jq '.type == "text"' "create qrcode type"
assert_jq '.title == ""' "create qrcode empty title"
pass "create qrcode"

QR_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-qr")"
assert_contains "$QR_TEXT" "Scan this QR code" "public qrcode text"
pass "public qrcode text"

api_json POST "$POST_BASE_URL/" '{"url":"first","path":"'"$SMOKE_PREFIX"'-conflict","title":"First Title"}'
assert_status 201 "create conflict seed"
api_json POST "$POST_BASE_URL/" '{"url":"second","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 409 "detect conflict"
assert_jq '.code == "conflict"' "detect conflict code"
assert_jq '.details.existing.title == "First Title"' "detect conflict existing title"
pass "detect conflict"

api_json PUT "$POST_BASE_URL/" '{"url":"updated","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 200 "overwrite existing"
assert_jq '.overwritten == "first"' "overwrite existing body"
assert_jq '.title == ""' "overwrite existing empty title"
pass "overwrite existing"

api_json POST "$POST_BASE_URL/" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl","ttl":0}'
assert_status 201 "ttl zero means infinite"
assert_jq '.ttl == null' "ttl zero means no expiration"
assert_jq '.title == ""' "ttl zero empty title"
assert_jq '.warning == null or .warning == ""' "ttl zero no warning"
pass "ttl zero means infinite"

api_json POST "$POST_BASE_URL/" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl-decimal","ttl":1.5}'
assert_status 400 "ttl decimal rejected"
assert_jq '.error == "`ttl` must be a natural number"' "ttl decimal rejected message"
pass "ttl decimal rejected"

api_json POST "$POST_BASE_URL/" '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl-string","ttl":"10"}'
assert_status 400 "ttl string rejected"
assert_jq '.error == "`ttl` must be a natural number"' "ttl string rejected message"
pass "ttl string rejected"

api_json GET "$POST_BASE_URL/" ''
assert_status 200 "list items"
assert_jq 'map(.path) | index("'"$SMOKE_PREFIX"'-text") != null' "list includes text item"
assert_jq 'map(select(.path == "'"$SMOKE_PREFIX"'-text"))[0].title == "Greeting Card"' "list includes title"
pass "list items"

api_json GET "$POST_BASE_URL/" '{"path":"'"$SMOKE_PREFIX"'-md"}' yes x-export true
assert_status 200 "lookup html export"
assert_jq '.type == "html"' "lookup html export type"
assert_jq '.content | contains("<title></title>")' "lookup html export body"
assert_jq '.title == "Markdown Title"' "lookup html export title"
pass "lookup html export"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$SMOKE_PREFIX"'-missing"}'
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
  "$POST_BASE_URL/" >"$FILE_STATUS"
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

FILE_ZERO_BODY="$TMP_DIR/file-zero.body"
FILE_ZERO_STATUS="$TMP_DIR/file-zero.status"
curl -sS -o "$FILE_ZERO_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@$FILE_PATH;type=text/plain" \
  -F "path=$SMOKE_PREFIX-file-zero" \
  -F "ttl=0" \
  "$POST_BASE_URL/" >"$FILE_ZERO_STATUS"
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
  "$POST_BASE_URL/" >"$FILE_BAD_TTL_STATUS"
if [[ "$(cat "$FILE_BAD_TTL_STATUS")" != "400" ]]; then
  fail "file upload bad ttl" "body: $(cat "$FILE_BAD_TTL_BODY")"
fi
if ! jq -e '.error == "`ttl` must be a natural number"' >/dev/null <<<"$(cat "$FILE_BAD_TTL_BODY")"; then
  fail "file upload bad ttl message" "body: $(cat "$FILE_BAD_TTL_BODY")"
fi
pass "file upload bad ttl"

FILE_PUBLIC="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-file.txt")"
assert_contains "$FILE_PUBLIC" "upload-body" "public file read"
pass "public file read"

MISSING_FILE_BODY="$TMP_DIR/file-missing.body"
MISSING_FILE_STATUS="$TMP_DIR/file-missing.status"
curl -sS -o "$MISSING_FILE_BODY" -w "%{http_code}" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=$SMOKE_PREFIX-missing-file" \
  "$POST_BASE_URL/" >"$MISSING_FILE_STATUS"
if [[ "$(cat "$MISSING_FILE_STATUS")" != "400" ]]; then
  fail "missing file field" "body: $(cat "$MISSING_FILE_BODY")"
fi
pass "missing file field"

api_json DELETE "$POST_BASE_URL/" '{"path":"'"$SMOKE_PREFIX"'-conflict"}' yes x-export true
assert_status 200 "delete existing"
assert_jq '.deleted == "'"$SMOKE_PREFIX"'-conflict"' "delete existing path"
assert_jq '.content == "updated"' "delete existing content"
assert_jq '.title == ""' "delete existing title"
pass "delete existing"

echo "HTTP API smoke checks completed successfully."
