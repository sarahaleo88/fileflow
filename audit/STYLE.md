# Style Review

Date: 2026-01-15

## P1
- No enforced formatting/lint configuration prior to audit.
  - Evidence: repo root had no `.editorconfig` or lint config (see `ls` in RUNLOG).
  - Fix applied: added `.editorconfig`, `Makefile` targets for format/lint, and CI to enforce gofmt/go vet (see `.editorconfig`, `Makefile`, `.github/workflows/ci.yml`).
  - Verify: `make lint` (format check + go vet), `make fmt` (optional).

## P2
- Go formatting drift detected in `internal/realtime/events.go`.
  - Evidence: `gofmt -l .` reported `internal/realtime/events.go` (see `audit/RUNLOG.md`).
  - Command: `gofmt -l .`
  - Output summary: `internal/realtime/events.go` listed.
  - Fix applied: `gofmt -w internal/realtime/events.go`.
  - Verify: `gofmt -l .` returns no output.

- Inconsistent error response style for WebSocket auth failures (plain `http.Error` vs JSON in REST handlers).
  - Evidence: `internal/handler/api.go:252-262` uses `http.Error` in `handleWebSocket` while REST APIs use `writeError`.
  - Command: `nl -ba internal/handler/api.go | sed -n '252,262p'`
  - Output summary: WS auth failures use `http.Error` instead of JSON envelope.
  - Impact: minor consistency issue; not user-facing in browser-based WS; no immediate fix applied to avoid protocol changes.
  - Suggestion: standardize error envelope for HTTP endpoints and document WS close codes.
