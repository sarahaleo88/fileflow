# Engineering Quality Review

Date: 2026-01-15

## P1 (Resolved)
- Documentation/API mismatch addressed: device challenge/attest endpoints implemented and README updated.
  - Evidence: `internal/handler/api.go` now includes `/api/device/challenge` and `/api/device/attest`; README reflects `/api/login` with device_ticket.
  - Command: `rg -n \"device/challenge|device/attest\" internal/handler/api.go` and `rg -n \"Authentication Flow\" -n README.md`
  - Output summary: device endpoints exist; docs match current flow.

## P2 (Resolved)
- Config drift fixed: `SESSION_TTL_HOURS` and `MAX_WS_MSG_BYTES` are now wired into runtime config.
  - Evidence: `cmd/server/main.go` reads `SESSION_TTL_HOURS` and `MAX_WS_MSG_BYTES`; handler uses configured `SessionTTL` and WS limits.
  - Command: `rg -n \"SESSION_TTL_HOURS|MAX_WS_MSG_BYTES\" cmd/server/main.go` and `rg -n \"maxWSMsgBytes\" internal/handler/api.go`
  - Output summary: env vars are used; defaults preserved.

- CI coverage was missing prior to audit; Playwright e2e script exists but is not runnable in CI (no package.json).
  - Evidence: `e2e-test.js` exists, but no Node dependency manifest; no `.github/workflows/*` present pre-audit.
  - Command: `rg --files -g 'package.json'`
  - Output summary: no `package.json` found (Playwright not installable by default).
  - Fix applied: added GitHub Actions CI for gofmt/go vet/go test and govulncheck.

## Tests Summary
- Existing tests: auth, handler (device challenge/attest + ws auth), realtime, store, limit.
- Gaps: no automated E2E test covering full UI flow.

## Validation Commands
- `go test ./...`
- `go vet ./...`

## Applied Changes Verification (Phase A)
- Mismatch: `internal/handler/middleware.go` contains trusted proxy logic and new tests, beyond the previously reported “IP parsing only” change.
  - Evidence: `internal/handler/middleware.go` now defines `SetTrustedProxies` and `isTrusted`; `internal/handler/ip_test.go` exists.
  - Command: `sed -n '1,120p' internal/handler/middleware.go` and `sed -n '1,120p' internal/handler/ip_test.go`
  - Output summary: proxy trust boundary logic present but not reflected in prior applied-change list.
- Mismatch: `cmd/server/main.go` now includes SESSION_KEY enforcement and TRUSTED_PROXIES wiring.
  - Evidence: `cmd/server/main.go:109-136`.
  - Command: `nl -ba cmd/server/main.go | sed -n '100,140p'`
  - Output summary: SESSION_KEY fail-fast in `ENV=prod` and TRUSTED_PROXIES handling are present but were not reported.
- Mismatch: `internal/handler/api.go` now enforces device ID for login and uses configurable `SessionTTL`.
  - Evidence: `internal/handler/api.go:184-221`.
  - Command: `nl -ba internal/handler/api.go | sed -n '180,222p'`
  - Output summary: login now validates device ID and checks whitelist, not reported in applied-change list.
