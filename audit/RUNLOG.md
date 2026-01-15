# Run Log

## [Wave 0] Baseline Assessment - 2026-01-14
- `go test ./...`: **PASS** (100% success)
- `go test -race ./...`: **PASS** (Verified concurrency safety, minimal macOS linker warnings noted)
- `SQLite Check`: Schema for `devices` table verified. `WAL` mode enabled.
- `Security Issue Found`: Whitelist code exists but is bypassed in `/api/login`.

## [Wave 1] Blocking Security Fixes - 2026-01-14
- **Auth Model**: Updated `/api/login` to require `device_id` and check store.
- **Frontend**: Updated `app.js` to generate UUID for `device_id`.
- **IP Trust**: Implemented `TRUSTED_PROXIES` logic in `middleware.go` and `ip_test.go`.
- **Config**: Enforced `SESSION_KEY` in production mode.
- **Verification**: `go test -v ./internal/handler` PASSED (Login flow, IP logic).

## [Wave 2] High Priority Fixes - 2026-01-14
- **409 Conflict**: Verified `DuplicateDevice` test case returns 409.
- **CheckOrigin**: Enforced `APP_DOMAIN` requirement in prod.
- **Verification**: `go test -v ./internal/handler` PASSED.

## [Wave 3] Suggestions & Hardening - 2026-01-14
- **TTL**: Made `SESSION_TTL` configurable (env).
- **Active Messages**: Added `maxActiveMsgs` limit (100) per client.
- **Headers**: Added `SecurityHeadersMiddleware` (Nosniff, Frame-Options).
- **Cleanup**: Deprecated `internal/auth/session.go`.
- **Verification**: All tests passed.

## Phase B Verification (2026-01-15)
- Command: `go test ./...`
  - Output summary: all packages passed after device attestation changes.
- Command: `go vet ./...`
  - Output summary: no issues.
- Command: `make lint`
  - Output summary: go vet passed.

## Phase C Verification (2026-01-15)
- Command: `go test ./...`
  - Output summary: all packages passed after session key/proxy changes.
- Command: `go vet ./...`
  - Output summary: no issues.
- Command: `make lint`
  - Output summary: go vet passed.

## Phase D Verification (2026-01-15)
- Command: `go test ./...`
  - Output summary: all packages passed after config/Docker/WS changes.
- Command: `go vet ./...`
  - Output summary: no issues.
- Command: `make lint`
  - Output summary: go vet passed.
- Command: `go test -race ./...`
  - Output summary: tests passed; linker emitted LC_DYSYMTAB warnings on macOS (no test failures).
