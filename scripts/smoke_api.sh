#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3012}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/15}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli is required" >&2
  exit 1
fi

REDIS_HOSTPORT="${REDIS_URL#redis://}"
REDIS_HOSTPORT="${REDIS_HOSTPORT%%/*}"
REDIS_DB="${REDIS_URL##*/}"
REDIS_HOST="${REDIS_HOSTPORT%%:*}"
REDIS_PORT="${REDIS_HOSTPORT##*:}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SMOKE_PREFIX="smoke-$(date +%s)"

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
  local path="$2"
  local body="${3:-}"
  local content_type="${4:-application/json}"
  local auth="${5:-yes}"
  local extra_header_name="${6:-}"
  local extra_header_value="${7:-}"
  local response_file="$TMP_DIR/response.body"
  local status_file="$TMP_DIR/response.status"
  local -a args
  args=(-sS -o "$response_file" -w "%{http_code}" -X "$method")
  if [[ "$auth" == "yes" ]]; then
    args+=(-H "Authorization: Bearer $POST_TOKEN")
  fi
  if [[ -n "$content_type" ]]; then
    args+=(-H "Content-Type: $content_type")
  fi
  if [[ -n "$extra_header_name" ]]; then
    args+=(-H "$extra_header_name: $extra_header_value")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  args+=("$POST_BASE_URL$path")
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

api POST / '{"url":"hello"}' application/json no
assert_status 401 "unauthorized create"
assert_jq '.code == "unauthorized"' "unauthorized create code"
pass "unauthorized create"

api POST / '{"url":"hello","path":"'"$SMOKE_PREFIX"' bad"}'
assert_status 400 "invalid path"
assert_jq '.code == "invalid_request"' "invalid path code"
pass "invalid path"

api POST / '{"url":"hello","path":"'"$SMOKE_PREFIX"'-text","title":"Greeting Card"}'
assert_status 201 "create text"
assert_jq '.type == "text"' "create text type"
assert_jq '.path == "'"$SMOKE_PREFIX"'-text"' "create text path"
pass "create text"

RAW_VALUE="$(redis_get "surl:$SMOKE_PREFIX-text")"
assert_contains "$RAW_VALUE" '"type":"text"' "redis json type"
assert_contains "$RAW_VALUE" '"content":"hello"' "redis json content"
assert_contains "$RAW_VALUE" '"title":"Greeting Card"' "redis json title"
pass "redis json storage"

api GET / '{"path":"'"$SMOKE_PREFIX"'-text"}' application/json yes x-export true
assert_status 200 "lookup text"
assert_jq '.content == "hello"' "lookup text export"
pass "lookup text"

PUBLIC_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-text")"
assert_contains "$PUBLIC_TEXT" "hello" "public text read"
pass "public text read"

api POST / '{"url":"https://example.com/path?q=1","path":"'"$SMOKE_PREFIX"'-link"}'
assert_status 201 "create url"
assert_jq '.type == "url"' "create url type"
pass "create url"

REDIRECT_HEADERS="$(curl -sSI "$POST_BASE_URL/$SMOKE_PREFIX-link")"
assert_contains "$REDIRECT_HEADERS" "Location: https://example.com/path?q=1" "public url redirect"
pass "public url redirect"

api POST / '{"url":"example.com/path","path":"'"$SMOKE_PREFIX"'-badurl","type":"url"}'
assert_status 400 "reject invalid url"
assert_jq '.code == "invalid_request"' "reject invalid url code"
pass "reject invalid url"

api POST / '{"url":"# Title\n\nHello from Markdown","path":"'"$SMOKE_PREFIX"'-md","convert":"md2html"}'
assert_status 201 "create md2html"
assert_jq '.type == "html"' "create md2html type"
pass "create md2html"

MD_HTML="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-md")"
assert_contains "$MD_HTML" "<article class=\"markdown-body\">" "rendered html shell"
assert_contains "$MD_HTML" "<h1 id=\"title\">Title</h1>" "rendered markdown heading"
pass "rendered html"

api POST / '{"url":"https://example.com/qr","path":"'"$SMOKE_PREFIX"'-qr","convert":"qrcode"}'
assert_status 201 "create qrcode"
assert_jq '.type == "text"' "create qrcode type"
pass "create qrcode"

QR_TEXT="$(curl -sS "$POST_BASE_URL/$SMOKE_PREFIX-qr")"
assert_contains "$QR_TEXT" "Scan this QR code" "public qrcode text"
pass "public qrcode text"

api POST / '{"url":"first","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 201 "create conflict seed"
api POST / '{"url":"second","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 409 "detect conflict"
assert_jq '.code == "conflict"' "detect conflict code"
pass "detect conflict"

api PUT / '{"url":"updated","path":"'"$SMOKE_PREFIX"'-conflict"}'
assert_status 200 "overwrite existing"
assert_jq '.overwritten == "first"' "overwrite existing body"
pass "overwrite existing"

api POST / '{"url":"ttl item","path":"'"$SMOKE_PREFIX"'-ttl","ttl":0}'
assert_status 201 "ttl fallback"
assert_jq '.expires_in == 1' "ttl fallback minutes"
assert_jq '.warning == "invalid ttl, fallback to 1 minute"' "ttl fallback warning"
pass "ttl fallback"

api GET / '' '' '' yes
assert_status 200 "list items"
assert_jq 'map(.path) | index("'"$SMOKE_PREFIX"'-text") != null' "list includes text item"
pass "list items"

api DELETE / '{"path":"'"$SMOKE_PREFIX"'-missing"}'
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
FILE_EXT_VALUE="$(redis_get "surl:$SMOKE_PREFIX-file.txt")"
assert_contains "$FILE_EXT_VALUE" '"type":"file"' "redis file json type"
assert_contains "$FILE_EXT_VALUE" '"title":"Upload Attachment"' "redis file title"
pass "file upload"

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

api DELETE / '{"path":"'"$SMOKE_PREFIX"'-conflict"}' application/json yes x-export true
assert_status 200 "delete existing"
assert_jq '.deleted == "'"$SMOKE_PREFIX"'-conflict"' "delete existing path"
assert_jq '.content == "updated"' "delete existing content"
pass "delete existing"

echo "Smoke API checks completed successfully."
