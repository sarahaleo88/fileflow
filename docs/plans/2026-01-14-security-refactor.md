# Security Layer Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor FileFlow from a device-keypair auth model to a lightweight, stateless shared-secret model with rate limiting and origin protection.

**Architecture:**
- **Auth:** Global Shared Secret (Argon2id hashed) -> Stateless HMAC-signed Cookie (`ff_session`).
- **Session:** JWT-like structure (HMAC-SHA256) with `ver` (version), `iat`, `exp`, `sid`. No database storage for sessions.
- **Revocation:** Global key rotation via env var or config version increment.
- **Protection:**
    - WS Handshake: Strict Cookie + Origin validation.
    - Rate Limiting: Login (IP-based), WS Connections (IP/Global limits), Event Rate (Per-conn).
- **Frontend:** Single Page App with "Unlock" Modal. No separate login page.

**Tech Stack:** Go 1.21+, Vanilla JS, SQLite (config only), `golang.org/x/time/rate`.

---

### Task 1: Clean Up Old Auth & Add Rate Limit Dependencies

**Files:**
- Modify: `go.mod`
- Remove: `internal/auth/challenge.go`, `internal/auth/signature.go`, `internal/store/device.go` (and related tables)
- Modify: `internal/store/sqlite.go` (drop device tables)

**Step 1: Add dependencies**
Run: `go get golang.org/x/time/rate`

**Step 2: Remove old files**
Run: `rm internal/auth/challenge.go internal/auth/signature.go internal/store/device.go`
*(Note: This will break the build temporarily, which is expected)*

**Step 3: Update Database Schema**
Modify `internal/store/sqlite.go` to remove `devices` table creation and add `config` table (key, value) if not exists (for secret hash if we decide to store it there, otherwise env var).
*Decision: We stick to Env Var for Secret Hash for simplicity as per design, but let's cleanup schema.*

**Step 4: Verify Clean**
Run: `go mod tidy`

**Step 5: Commit**
`git add . && git commit -m "chore: remove legacy auth and add rate limit dep"`

---

### Task 2: Implement Stateless Session & Auth Logic

**Files:**
- Create: `internal/auth/token.go` (HMAC signing/verification)
- Modify: `internal/auth/secret.go` (Keep Argon2id verify, add Hash generation helper if missing)
- Test: `internal/auth/token_test.go`

**Step 1: Write Token Test**
Create `internal/auth/token_test.go`:
```go
package auth

import (
    "testing"
    "time"
)

func TestTokenSignVerify(t *testing.T) {
    secret := []byte("my-super-secret-signing-key")
    tm := NewTokenManager(secret)

    // Case 1: Valid Token
    token, err := tm.Sign("session-id-123", 1, time.Hour)
    if err != nil {
        t.Fatalf("Sign failed: %v", err)
    }

    claims, err := tm.Verify(token)
    if err != nil {
        t.Fatalf("Verify failed: %v", err)
    }
    if claims.SID != "session-id-123" {
        t.Errorf("wrong SID: %s", claims.SID)
    }

    // Case 2: Expired Token
    expiredToken, _ := tm.Sign("session-id-456", 1, -time.Hour)
    _, err = tm.Verify(expiredToken)
    if err == nil {
        t.Error("Verify should fail for expired token")
    }

    // Case 3: Tampered Token
    // (Implementation detail: modify signature part of string)
}
```

**Step 2: Implement Token Manager**
Create `internal/auth/token.go`:
- Struct `Claims`: `ver` (int), `sid` (string), `iat` (int64), `exp` (int64).
- Function `Sign(sid string, version int, ttl time.Duration) (string, error)`: Return base64(payload).base64(hmac).
- Function `Verify(token string) (*Claims, error)`: Check sig, check exp.

**Step 3: Verify Tests**
Run: `go test -v ./internal/auth`

**Step 4: Commit**
`git add internal/auth && git commit -m "feat(auth): implement stateless session token"`

---

### Task 3: Implement Rate Limiters (Login & WS)

**Files:**
- Create: `internal/limit/limiter.go`
- Test: `internal/limit/limiter_test.go`

**Step 1: Write Rate Limiter Test**
Create `internal/limit/limiter_test.go`:
```go
package limit

import (
    "testing"
    "time"
)

func TestIPLimiter(t *testing.T) {
    // 2 requests per second
    l := NewIPLimiter(2, 5) 
    
    if !l.Allow("1.2.3.4") {
        t.Error("Should allow first request")
    }
    if !l.Allow("1.2.3.4") {
        t.Error("Should allow second request")
    }
    // ... test burst
}
```

**Step 2: Implement Limiter**
Create `internal/limit/limiter.go`:
- Use `x/time/rate`.
- `IPLimiter` struct: `map[string]*rate.Limiter`, Mutex, cleanup routine.
- `LoginLimiter`: Strict (e.g., 5 req/min).
- `ConnLimiter`: Track active connections per IP (atomic counters).

**Step 3: Verify Tests**
Run: `go test -v ./internal/limit`

**Step 4: Commit**
`git add internal/limit && git commit -m "feat(limit): implement ip rate limiters"`

---

### Task 4: API Handlers (Login & Session)

**Files:**
- Modify: `internal/handler/api.go`
- Modify: `cmd/server/main.go` (Inject secret hash & signing key)

**Step 1: Define Config & Secrets**
In `cmd/server/main.go`, load `APP_SECRET` (for admin login) and `SESSION_KEY` (for HMAC) from env.

**Step 2: Implement Login Handler**
In `internal/handler/api.go`:
- `HandleLogin`:
    - Check rate limit (429 if exceeded).
    - Parse body `{secret: "..."}`.
    - Verify against `APP_SECRET_HASH` (Argon2id).
    - If valid: Generate Token -> Set Cookie (`ff_session`, HttpOnly, Secure, Strict).
    - Return `{authed: true}`.

**Step 3: Implement Session Handler**
In `internal/handler/api.go`:
- `HandleSession`:
    - Read Cookie `ff_session`.
    - Verify Token.
    - Return `{authed: true/false}`.

**Step 4: Commit**
`git add . && git commit -m "feat(api): add login and session handlers"`

---

### Task 5: Secure WebSocket Upgrade

**Files:**
- Modify: `internal/realtime/client.go` (or wherever `ServeWs` is)
- Modify: `internal/realtime/hub.go`

**Step 1: Add Middleware/Check to ServeWs**
In `internal/realtime/client.go`:
- `ServeWs(hub, w, r)`:
    - **Origin Check**: `r.Header.Get("Origin")` MUST match configured `APP_DOMAIN`.
    - **Auth Check**: Verify `ff_session` cookie. If missing/invalid -> `http.Error(w, "Unauthorized", 401)`.
    - **Conn Limit**: Check global & IP connection count. If full -> 503.

**Step 2: Enforce Event Rate Limit**
In `Client.readPump`:
- Add a `rate.Limiter` to the client struct.
- In loop: `if !c.limiter.Allow() { break }` (disconnect on flood).

**Step 3: Commit**
`git add internal/realtime && git commit -m "feat(ws): secure websocket upgrade and limits"`

---

### Task 6: Frontend - Auth Modal & Logic

**Files:**
- Modify: `web/static/index.html` (Add Modal HTML)
- Modify: `web/static/style.css` (Modal styles)
- Modify: `web/static/app.js` (Auth flow)

**Step 1: Add HTML/CSS**
- `index.html`: Add `<div id="auth-modal" class="hidden">...<input type="password">...</div>`.
- `style.css`: Fullscreen overlay, centered box.

**Step 2: Implement JS Logic**
In `app.js`:
- `init()`:
    - Call `GET /api/session`.
    - If `authed: true` -> `connectWS()`.
    - If `authed: false` -> Show Modal.
- Modal Submit:
    - `POST /api/login`.
    - Success -> Hide Modal -> `connectWS()`.
    - Fail -> Show error / Shake animation.
- `connectWS()`:
    - Handle `onclose` (401/403) -> Show Modal again (session expired).

**Step 3: Commit**
`git add web/static && git commit -m "feat(ui): add auth modal and integration"`

---

### Task 7: Cleanup & Verify

**Files:**
- Modify: `cmd/server/main.go` (Wire everything together)
- Verify: Full manual test.

**Step 1: Wire Main**
Ensure `main.go` initializes:
- `AuthManager` (Token signing).
- `RateLimiters`.
- Pass them to `Handlers`.

**Step 2: Build**
Run: `go build -o fileflow ./cmd/server`

**Step 3: Verify Scenarios**
1. No cookie -> Access page -> See Modal.
2. Enter wrong password -> Error.
3. Enter correct password -> Unlocks -> WS Connected.
4. Incognito -> See Modal.
5. Manually delete cookie -> Refresh -> See Modal.

**Step 4: Commit**
`git add . && git commit -m "refactor: complete security layer integration"`
