#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
trap 'stop_server; cleanup_smoke_tmp' EXIT

SERVER_BIN="${SERVER_BIN:-/tmp/post-server-smoke-all}"
POST_TOKEN="${POST_TOKEN:-demo}"

cd "$ROOT_DIR"

echo "Building server binary..."
make build BINARY="$SERVER_BIN"

echo "Running regression tests..."
go test ./...

echo "Running render smoke..."
"$SCRIPT_DIR/smoke_render.sh"

echo "Running HTTP API smoke..."
start_server "$SERVER_BIN" 3012 "redis://localhost:6379/15" "$POST_TOKEN"
POST_BASE_URL="http://127.0.0.1:3012" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/15" "$SCRIPT_DIR/smoke_embedded_assets.sh"
POST_BASE_URL="http://127.0.0.1:3012" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/15" "$SCRIPT_DIR/smoke_http_api.sh"
stop_server

echo "Running topic API smoke..."
start_server "$SERVER_BIN" 3013 "redis://localhost:6379/14" "$POST_TOKEN"
POST_BASE_URL="http://127.0.0.1:3013" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/14" "$SCRIPT_DIR/smoke_topic_api.sh"
stop_server

echo "Running Redis storage smoke..."
start_server "$SERVER_BIN" 3014 "redis://localhost:6379/13" "$POST_TOKEN"
POST_BASE_URL="http://127.0.0.1:3014" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/13" "$SCRIPT_DIR/smoke_redis_storage.sh"
stop_server

echo "All smoke and regression checks completed successfully."
