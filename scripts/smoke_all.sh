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
go build -o "$SERVER_BIN" ./cmd/post-server

echo "Running regression tests..."
go test ./...

echo "Running PLAN2 smoke..."
"$SCRIPT_DIR/smoke_plan2.sh"

echo "Running API smoke..."
start_server "$SERVER_BIN" 3012 "redis://localhost:6379/15" "$POST_TOKEN"
POST_BASE_URL="http://127.0.0.1:3012" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/15" "$SCRIPT_DIR/smoke_api.sh"
stop_server

echo "Running PLAN3 smoke..."
start_server "$SERVER_BIN" 3013 "redis://localhost:6379/14" "$POST_TOKEN"
POST_BASE_URL="http://127.0.0.1:3013" POST_TOKEN="$POST_TOKEN" LINKS_REDIS_URL="redis://localhost:6379/14" "$SCRIPT_DIR/smoke_plan3.sh"
stop_server

echo "All smoke and regression checks completed successfully."
