#!/usr/bin/env bash
set -euo pipefail

POST_BASE_URL="${POST_BASE_URL:-http://127.0.0.1:3012}"
POST_TOKEN="${POST_TOKEN:-demo}"
REDIS_URL="${LINKS_REDIS_URL:-redis://localhost:6379/15}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
trap cleanup_smoke_tmp EXIT
configure_redis "$REDIS_URL"

echo "Using POST_BASE_URL=$POST_BASE_URL"
echo "Using Redis DB=$REDIS_DB"

BASE_ASSET_PATH="/asset/md-base-7f7c1c5a.css"
UNKNOWN_ASSET_PATH="/asset/not-exist.txt"
RESERVED_INPUT_PATH="asset/md-base-7f7c1c5a.css"
INTERNAL_REFERER="$POST_BASE_URL/internal-page"

DIRECT_ASSET_BODY="$TMP_DIR/direct-asset.body"
DIRECT_ASSET_STATUS="$TMP_DIR/direct-asset.status"
curl -sS -o "$DIRECT_ASSET_BODY" -w "%{http_code}" "$POST_BASE_URL$BASE_ASSET_PATH" >"$DIRECT_ASSET_STATUS"
if [[ "$(cat "$DIRECT_ASSET_STATUS")" != "403" ]]; then
  fail "direct embedded asset forbidden" "expected HTTP 403, got $(cat "$DIRECT_ASSET_STATUS")"
fi
if ! jq -e '.code == "forbidden"' >/dev/null <"$DIRECT_ASSET_BODY"; then
  fail "direct embedded asset forbidden body" "body: $(cat "$DIRECT_ASSET_BODY")"
fi
pass "direct embedded asset forbidden"

BASE_ASSET_BODY="$(curl -sS -H "Referer: $INTERNAL_REFERER" "$POST_BASE_URL$BASE_ASSET_PATH")"
assert_contains "$BASE_ASSET_BODY" ".markdown-body" "embedded base asset body"
pass "embedded base asset body"

BASE_ASSET_HEADERS="$(curl -sSI -H "Referer: $INTERNAL_REFERER" "$POST_BASE_URL$BASE_ASSET_PATH")"
assert_contains "$BASE_ASSET_HEADERS" "HTTP/1.1 200" "embedded base asset status"
assert_contains "$BASE_ASSET_HEADERS" "Cache-Control: public, max-age=31536000, immutable" "embedded base asset cache"
pass "embedded base asset headers"

UNKNOWN_ASSET_HEADERS="$(curl -sSI "$POST_BASE_URL$UNKNOWN_ASSET_PATH")"
assert_contains "$UNKNOWN_ASSET_HEADERS" "404" "unknown asset path not found"
pass "unknown asset path not found"

RESERVED_DELETE_BODY="$TMP_DIR/reserved-delete.body"
RESERVED_DELETE_STATUS="$TMP_DIR/reserved-delete.status"
curl -sS -o "$RESERVED_DELETE_BODY" -w "%{http_code}" \
  -X DELETE \
  "$POST_BASE_URL$BASE_ASSET_PATH" >"$RESERVED_DELETE_STATUS"
if [[ "$(cat "$RESERVED_DELETE_STATUS")" != "405" ]]; then
  fail "reserved asset delete method" "expected HTTP 405, got $(cat "$RESERVED_DELETE_STATUS")"
fi
pass "reserved asset delete method"

api_json POST "$POST_BASE_URL/" '{"url":"hello","path":"'"$RESERVED_INPUT_PATH"'","type":"text"}'
assert_status 400 "reserved asset create rejected"
assert_jq '.code == "invalid_request"' "reserved asset create code"
pass "reserved asset create rejected"
