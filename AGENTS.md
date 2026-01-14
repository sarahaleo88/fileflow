# PROJECT KNOWLEDGE BASE

**Generated:** 2026-01-14
**Context:** FileFlow - Ephemeral text transfer

## OVERVIEW
Real-time, ephemeral text transfer system.
Stack: Go 1.21+ (Server), SQLite (Device Store), Vanilla JS (Frontend), Docker (Deployment).

## STRUCTURE
```
fileflow/
├── cmd/server/         # Entry point (main.go), config loading
├── internal/
│   ├── auth/           # Security: Argon2id, Sessions, Challenges
│   ├── handler/        # HTTP API & Middleware (CORS, RateLimit)
│   ├── limit/          # Rate limiting logic
│   ├── realtime/       # WebSocket Hub & Protocol events
│   └── store/          # SQLite data layer (Device whitelist)
├── web/static/         # Frontend: Vanilla JS, CSS, HTML
├── deployment/         # Docker, Caddy, Scripts (Singular dir name)
└── scripts/            # Admin utilities
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Startup** | `cmd/server/main.go` | Config, DB init, Server run |
| **Auth Logic** | `internal/auth` | Session validation, Hashing |
| **Realtime** | `internal/realtime` | WS Hub, Event definitions |
| **Database** | `internal/store` | SQLite schemas & queries |
| **Frontend** | `web/static/app.js` | Client logic, Crypto, UI |
| **API Routes** | `internal/handler/api.go` | HTTP endpoints |

## CONVENTIONS
- **Go**: Use `internal/` for all private packages. Table-driven tests.
- **JS**: **NO FRAMEWORKS**. Pure Vanilla JS. Module pattern (IIFE).
- **Config**: Env vars loaded in `main.go`. Defaults provided.
- **Testing**: Integration tests use `httptest` + temporary `sqlite` DBs.

## ANTI-PATTERNS (THIS PROJECT)
- **Persistence**: NEVER store message content. RAM only.
- **Auth Bypass**: NEVER skip device attestation or secret check.
- **Queuing**: NO offline delivery. Fail if peer offline.
- **Logging**: NEVER log message payloads.
- **Complexity**: NO external JS libs (React/Vue). Keep it simple.

## COMMANDS
```bash
# Dev
go run ./cmd/server
go test ./...

# Docker
docker compose up               # Dev
docker compose -f deployment/docker-compose.prod.yml up -d # Prod

# Admin
./scripts/enroll-device.sh <id> <label>
./scripts/init-secret.sh
```

## NOTES
- **Security**: Device whitelist is critical. `BOOTSTRAP_TOKEN` required for enrollment API.
- **Build**: Requires `CGO_ENABLED=1` for SQLite.
- **Naming**: `deployment` dir is singular (not `deployments`).
