# AGENTS.md - FileFlow

> Real-time text & paragraph transfer system. Go 1.21+ / SQLite / Vanilla JS.

---

## Build / Lint / Test Commands

```bash
# Build
go build -o fileflow ./cmd/server

# Run locally
go run ./cmd/server

# Run all tests
go test ./...

# Run single test (by name pattern)
go test -v -run TestFunctionName ./internal/auth

# Run tests in specific package
go test -v ./internal/store/...

# Run with race detection
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
golangci-lint run

# Docker build
docker compose build

# Docker run (dev)
docker compose up

# Docker run (prod)
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# Initialize secret hash
docker compose exec app ./scripts/init-secret.sh

# Enroll device
docker compose exec app ./scripts/enroll-device.sh <device_id> <label>
```

---

## Project Structure

```
fileflow/
├── cmd/server/main.go          # Entry point, config loading
├── internal/
│   ├── auth/                   # Challenge, session, secret handling
│   │   ├── challenge.go        # Nonce generation, TTL store
│   │   ├── session.go          # Cookie management
│   │   └── secret.go           # Argon2id hashing
│   ├── handler/                # HTTP handlers
│   │   ├── api.go              # Route definitions
│   │   └── middleware.go       # Rate limit, CORS, logging
│   ├── realtime/               # WebSocket handling
│   │   ├── hub.go              # Client registry, broadcast
│   │   ├── client.go           # Connection wrapper, pumps
│   │   └── events.go           # Event types, payloads
│   └── store/                  # Data layer
│       ├── sqlite.go           # DB init, migrations
│       └── device.go           # Device CRUD
├── web/static/                 # Frontend (vanilla JS)
│   ├── index.html
│   ├── app.js
│   └── style.css
├── deployment/                 # Docker files
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── docker-compose.prod.yml
│   ├── Caddyfile
│   └── .env.example
└── scripts/                    # Admin scripts
```

---

## Code Style Guidelines

### Go Conventions

**Imports** - Group in order: stdlib, external, internal. Use goimports.
```go
import (
    "context"
    "net/http"
    "time"

    "github.com/gorilla/websocket"
    "golang.org/x/crypto/argon2"

    "github.com/your/fileflow/internal/store"
)
```

**Formatting** - Use `gofmt`. No exceptions.

**Naming**:
- Packages: lowercase, single word (`auth`, `store`, `realtime`)
- Interfaces: describe behavior (`Reader`, `Verifier`)
- Structs: noun (`Client`, `Hub`, `Challenge`)
- Methods: verb or verb phrase (`Run`, `Broadcast`, `VerifySignature`)
- Constants: `SCREAMING_SNAKE` only for env vars; otherwise `camelCase`
- Exported: `PascalCase`; unexported: `camelCase`

**Error Handling**:
```go
// Always handle errors explicitly
result, err := doSomething()
if err != nil {
    return fmt.Errorf("doSomething failed: %w", err)
}

// Use sentinel errors for expected conditions
var ErrDeviceNotWhitelisted = errors.New("device not whitelisted")

// Wrap errors with context
if err := store.AddDevice(d); err != nil {
    return fmt.Errorf("add device %s: %w", d.ID, err)
}
```

**Context** - First parameter, always. Pass down call chains.
```go
func (h *Handler) CreateChallenge(ctx context.Context, req *ChallengeRequest) (*Challenge, error)
```

**Struct Tags** - JSON uses `snake_case`:
```go
type Event struct {
    Type      string      `json:"t"`
    Value     interface{} `json:"v"`
    Timestamp int64       `json:"ts"`
}
```

### Frontend (Vanilla JS)

**No frameworks** - Pure HTML/CSS/JS only.

**Module pattern** - Use IIFE or ES modules:
```javascript
const FileFlow = (function() {
    // Private state
    let ws = null;
    let keypair = null;

    // Public API
    return {
        init,
        send,
        disconnect
    };
})();
```

**Naming**:
- Functions: `camelCase`
- Constants: `UPPER_SNAKE_CASE`
- DOM elements: prefix with `$` (e.g., `$messageInput`)

**CSS**:
- BEM-lite naming: `.message-bubble`, `.presence-bar`
- Use CSS custom properties for theming
- Mobile-first responsive

---

## Architecture Constraints

### Security (CRITICAL)

1. **No message persistence** - Content is NEVER stored. Ephemeral only.
2. **Device whitelist** - Only pre-enrolled devices can authenticate.
3. **Two-phase auth** - Device attestation + secret verification.
4. **Cookie security** - Always `HttpOnly`, `Secure`, `SameSite=Strict`.
5. **No content logging** - Never log message payloads.

### WebSocket Protocol

Event envelope format:
```json
{"t": "event_type", "v": {...}, "ts": 1730000000000}
```

Event types: `presence`, `msg_start`, `para_start`, `para_chunk`, `para_end`, `msg_end`, `ack`, `send_fail`

Limits:
- `para_chunk` max: 4KB
- Message total max: 256KB
- Max paragraphs: 512

### Online-Only Delivery

Messages are ONLY delivered if peer is online at `msg_start`. No queuing.

---

## Testing Patterns

**Table-driven tests**:
```go
func TestParseParagraphs(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []string
    }{
        {"single", "hello", []string{"hello"}},
        {"two", "hello\n\nworld", []string{"hello", "world"}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseParagraphs(tt.input)
            if !reflect.DeepEqual(got, tt.expected) {
                t.Errorf("got %v, want %v", got, tt.expected)
            }
        })
    }
}
```

**Integration tests** - Use `httptest.Server` and actual WebSocket connections.

**Test file location** - Same package, `_test.go` suffix.

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_DOMAIN` | Yes | - | Domain for CORS/origin checks |
| `BOOTSTRAP_TOKEN` | Yes | - | Admin enrollment token |
| `SQLITE_PATH` | No | `/data/fileflow.db` | Database path |
| `RATE_LIMIT_RPS` | No | `5` | Requests per second limit |
| `MAX_WS_MSG_BYTES` | No | `262144` | Max WS message (256KB) |
| `SESSION_TTL_HOURS` | No | `12` | Session cookie TTL |

---

## Common Patterns

### Rate Limiting
```go
// Per-IP rate limiting using token bucket
type RateLimiter struct {
    visitors map[string]*rate.Limiter
    mu       sync.RWMutex
}
```

### Challenge Store
```go
// In-memory with TTL cleanup goroutine
type ChallengeStore struct {
    challenges map[string]*Challenge
    mu         sync.RWMutex
    ttl        time.Duration // 60s
}
```

### WebSocket Hub
```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}
```

---

## Do NOT

- Store or log message content
- Use `as any` / `@ts-ignore` equivalents
- Add external JS frameworks (React, Vue, etc.)
- Queue messages for offline delivery
- Skip device attestation
- Use weak password hashing (must be Argon2id)
- Expose internal errors to clients

---

## Reference

- Technical Spec: `FileFlow-Technical-Spec-v2.md`
- Implementation Tasks: `TASKS.md`
