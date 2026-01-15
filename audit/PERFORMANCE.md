# Performance Review

Date: 2026-01-15

## Current State
- HTTP server timeouts are set (Read/Write/Idle) in `cmd/server/main.go:144-149`.
- WebSocket read limits and ping/pong keepalive exist in `internal/realtime/client.go`.
- Global/per-IP connection caps exist in `internal/limit/limiter.go`.

## P1
- IP-based rate limiting could be bypassed due to unparsed `RemoteAddr` and raw `X-Forwarded-For` usage.
  - Evidence: `internal/handler/middleware.go:80-98` parses XFF/X-Real-IP and splits host/port (fix applied).
  - Command: `nl -ba internal/handler/middleware.go | sed -n '80,98p'`
  - Output summary: request IP is normalized before rate limiting.
  - Fix applied: parse XFF/X-Real-IP and split host/port (`internal/handler/middleware.go:80-98`).
  - Verify: `go test ./...` (logic exercised indirectly); manual: send multiple requests with varying ports, observe stable key.

## P2
- No request/WS metrics or tracing; limited ability to diagnose latency spikes or dropped WS messages.
  - Evidence: only `log.Printf` access logging (`internal/handler/middleware.go:101-109`) and websocket logs.
  - Command: `rg -n \"metrics|trace|prom\" internal cmd`
  - Output summary: no metrics/tracing instrumentation found.
  - Suggestion: add structured logs with request IDs + basic counters (requests, WS connects, send_fail) or expose `/metrics`.
  - Verify: log contains consistent fields or metrics endpoint returns counters.

- WS backpressure handling is drop-and-disconnect only (no queue visibility).
  - Evidence: `internal/realtime/hub.go:90-104` drops/evicts when send channel is full.
  - Command: `nl -ba internal/realtime/hub.go | sed -n '90,104p'`
  - Output summary: full send channel triggers unregister without metrics.
  - Suggestion: track per-client drop counters to detect slow consumers; consider bounded queue size config.

## Validation Commands
- `go test ./...`
- `go vet ./...`
