# Project Profile (Discovery)

Date: 2026-01-15

## Runtime & Languages
- Server: Go (go.mod declares Go 1.24.0)
- Frontend: Vanilla JS + HTML/CSS (web/static)
- Data: SQLite (device whitelist + config)

## Entry Points
- Server: cmd/server/main.go
- Frontend: web/static/index.html + web/static/app.js
- Realtime: /ws WebSocket endpoint

## Build & Test Tooling
- Go modules: go.mod / go.sum
- Tests: go test ./... (unit/integration-ish)
- Lint/format (post-audit additions): Makefile targets for gofmt/go vet/go test
- No Node package manager detected (no package.json)

## Deployment
- Dockerfile: deployment/Dockerfile (multi-stage build)
- Reverse proxy: Caddy (deployment/Caddyfile)
- Compose: deployment/docker-compose.yml + docker-compose.prod.yml

## Security Boundary (as implemented)
- Auth: shared secret -> session cookie (ff_session) signed via HMAC
- Admin: device enrollment via X-Admin-Bootstrap header
- WebSocket auth: requires ff_session cookie
- CORS: allow-list based on APP_DOMAIN
- Reverse proxy trust: X-Forwarded-For/X-Real-IP are consumed in-app

## Notes
- README documents device challenge/attest APIs, but routes are not present in code.
- Device whitelist table exists, but auth flow does not consult it.
