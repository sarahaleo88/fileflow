# Remediation Plan (Final)

Date: 2026-01-15

## P0
1) Device challenge/attest + device_ticket enforcement
- Status: Done
- Evidence: `internal/handler/api.go` implements `/api/device/challenge` and `/api/device/attest`; `/api/login` and `/ws` require `device_ticket`.
- Verification: `go test ./...` (see `audit/RUNLOG.md`), manual flow: challenge → attest → login → ws.
- Rollback: revert handlers and device_ticket checks in `internal/handler/api.go`; remove challenge store wiring in `cmd/server/main.go`.

## P1
2) SESSION_KEY fail-fast with dev escape hatch
- Status: Done
- Evidence: `resolveSessionKey()` in `cmd/server/main.go` enforces `SESSION_KEY` unless `APP_ENV=dev` or `FF_DEV=1` and `SECURE_COOKIES=false`.
- Verification: `go test ./cmd/server -run TestResolveSessionKey` and `go test ./...`.
- Rollback: revert `resolveSessionKey` usage and related tests in `cmd/server/main.go` and `cmd/server/main_test.go`.

3) Trusted proxy boundary with CIDR/IP list
- Status: Done
- Evidence: `internal/handler/middleware.go` `SetTrustedProxies` accepts CIDR/IP; `cmd/server/main.go` reads `TRUSTED_PROXY_CIDRS` (fallback `TRUSTED_PROXIES`).
- Verification: `go test ./internal/handler -run TestGetClientIP`.
- Rollback: revert proxy parsing logic and env wiring.

## P2
4) Config drift: SESSION_TTL_HOURS + MAX_WS_MSG_BYTES wired
- Status: Done
- Evidence: `cmd/server/main.go` reads `SESSION_TTL_HOURS` and `MAX_WS_MSG_BYTES`; handler uses configurable TTL and WS limits.
- Verification: `go test ./...` and manual check of cookie expiry + WS message size limit.
- Rollback: revert config wiring and restore constants.

5) WS error envelope consistency
- Status: Done
- Evidence: `internal/handler/api.go` uses `writeError` for WS auth failures; tests in `internal/handler/api_test.go`.
- Verification: `go test ./internal/handler -run TestWebSocketAuth`.
- Rollback: revert WS error handling to `http.Error`.

6) Container non-root runtime
- Status: Done
- Evidence: `deployment/Dockerfile` creates `fileflow` user and sets `USER fileflow`.
- Verification: `docker build -f deployment/Dockerfile .` and `docker run --rm <image> id`.
- Rollback: remove user creation and `USER` line in Dockerfile.

## Remaining
- None identified at P0/P1/P2.
