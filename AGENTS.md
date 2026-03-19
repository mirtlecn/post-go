# Agent Notes

This file captures repository-specific rules that are easy for agents to miss.

## Versioning

- Do not edit `internal/buildinfo/buildinfo.go` by hand when the goal is to release or bump the version.
- Use `./scripts/bump_version.sh <version>` for version bumps.
- The bump script requires a clean git working tree.
- The bump script updates `internal/buildinfo/buildinfo.go`, creates a commit, and creates a git tag.
- Tags use the `vX.Y.Z` format.
- Pushing a `v*` tag triggers the release workflow.

## Build And Test Flow

- `make build` removes the old binary and rebuilds `./post-server`.
- `make test` runs `make build` first, then `go test ./...`.
- `make smoke` runs `./scripts/smoke_all.sh`.
- `./scripts/smoke_all.sh` is the full local regression entrypoint:
  - rebuilds a temporary server binary
  - runs `go test ./...`
  - runs render smoke
  - runs HTTP API smoke
  - runs topic API smoke
  - runs Redis storage smoke

## Smoke Test Requirements

- Full smoke tests expect local Redis on `localhost:6379`.
- Smoke tests use Redis DB 15, 14, and 13.
- Smoke tests start temporary servers on ports 3012, 3013, and 3014.
- Smoke tests build a temporary binary at `/tmp/post-server-smoke-all` unless `SERVER_BIN` is overridden.
- When adding file-upload smoke coverage, clean up uploaded files at the end of the test flow.

## Embedded Assets

- Embedded frontend assets are generated, not hand-maintained.
- Run `make assets-sync` after changing source assets or anything that affects embedded asset output.
- CI and release workflows both run `make assets-sync` before building artifacts.

## CI And Release

- CI runs `make assets-sync` and `go test ./...`.
- CI also runs a GoReleaser snapshot build on every push and pull request.
- Release is driven by pushing a `v*` git tag.
- Release workflow runs GoReleaser after `make assets-sync`.

## Runtime Assumptions

- Required env: `LINKS_REDIS_URL`, `SECRET_KEY`.
- File uploads require S3-compatible storage configuration.
- Local manual startup usually follows:
  - `cp .env.example .env.local`
  - `make assets-sync`
  - `make`
  - `./post-server`

## Change Guidance

- If a change affects file uploads, prefer covering:
  - unit tests in `internal/httpapi/handlers_test.go`
  - smoke checks in `scripts/smoke_http_api.sh`
- If a change affects topics, check `scripts/smoke_topic_api.sh`.
- If a change affects rendering or embedded assets, check `scripts/smoke_render.sh` and `scripts/smoke_embedded_assets.sh`.
