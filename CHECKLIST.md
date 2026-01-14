# FileFlow MVP — Task Checklist

## Phase 0: Project Setup
- [x] P0.1 Initialize Go module
- [x] P0.2 Create directory structure
- [x] P0.3 Setup development environment

## Phase 1: Database Layer
- [x] T1.1.1 Create `internal/store/sqlite.go`
- [x] T1.1.2 Create `devices` table
- [x] T1.1.3 Create `config` table
- [x] T1.1.4 Implement device CRUD operations
- [x] T1.1.5 Implement config operations
- [x] T1.1.6 Write unit tests for store

## Phase 2: Authentication Module
- [x] T2.1.1 Create `internal/auth/challenge.go`
- [x] T2.1.2 Implement `GenerateChallenge()`
- [x] T2.1.3 Implement `VerifyChallenge()`
- [x] T2.2.1 Implement ECDSA P-256 signature verification
- [x] T2.2.2 Implement `VerifyDeviceAttestation()`
- [x] T2.3.1 Implement Argon2id hashing
- [x] T2.3.2 Create `internal/auth/session.go`
- [x] T2.3.3 Implement session management
- [x] T2.3.4 Write unit tests for auth module

## Phase 3: HTTP API
- [x] T3.1.1 Create `internal/handler/api.go`
- [x] T3.1.2 Implement middleware
- [x] T3.2.1 `GET /healthz`
- [x] T3.2.2 `POST /api/device/challenge`
- [x] T3.2.3 `POST /api/device/attest`
- [x] T3.2.4 `POST /api/auth/secret`
- [x] T3.2.5 `GET /api/presence`
- [x] T3.2.6 `POST /api/admin/devices`
- [x] T3.2.7 Static file serving
- [x] T3.2.8 Write integration tests for API

## Phase 4: WebSocket Realtime
- [x] T4.1.1 Create `internal/realtime/hub.go`
- [x] T4.1.2 Implement `Hub.Run()`
- [x] T4.1.3 Implement `Hub.OnlineCount()`
- [x] T4.1.4 Implement `Hub.Broadcast()`
- [x] T4.2.1 Create `internal/realtime/client.go`
- [x] T4.2.2 Implement `Client.ReadPump()`
- [x] T4.2.3 Implement `Client.WritePump()`
- [x] T4.3.1 Create `internal/realtime/events.go`
- [x] T4.3.2 Implement event types
- [x] T4.3.3 Implement online-only gating
- [x] T4.3.4 Implement message forwarding
- [x] T4.3.5 Implement size limits
- [x] T4.3.6 Write integration tests for WebSocket

## Phase 5: Frontend
- [x] T5.1.1 Create `web/static/app.js`
- [x] T5.1.2 Implement keypair generation
- [x] T5.1.3 Implement IndexedDB storage
- [x] T5.1.4 Implement device_id computation
- [x] T5.2.1 Implement challenge flow
- [x] T5.2.2 Implement unauthorized view
- [x] T5.2.3 Implement secret prompt modal
- [x] T5.2.4 Implement secret submission
- [x] T5.3.1 Implement WebSocket manager
- [x] T5.3.2 Implement event handlers
- [x] T5.4.1 Implement text input area
- [x] T5.4.2 Implement paragraph parser
- [x] T5.4.3 Implement chunking
- [x] T5.4.4 Implement send flow
- [x] T5.5.1 Implement message bubble creation
- [x] T5.5.2 Implement paragraph container
- [x] T5.5.3 Implement streaming text append
- [x] T5.5.4 Implement ack sending
- [x] T5.6.1 Create `web/static/style.css`
- [x] T5.6.2 Implement presence bar
- [x] T5.6.3 Implement composer area
- [x] T5.6.4 Implement message stream
- [x] T5.6.5 Implement paragraph styling
- [x] T5.7.1 Create `web/static/index.html`

## Phase 6: Main Server
- [x] T6.1 Create `cmd/server/main.go`
- [x] T6.2 Implement graceful shutdown
- [x] T6.3 Implement startup validation

## Phase 7: Docker Deployment
- [x] T7.1.1 Create multi-stage Dockerfile
- [x] T7.1.2 Add healthcheck
- [ ] T7.1.3 Build and test locally
- [x] T7.2.1 Create `docker-compose.yml`
- [x] T7.2.2 Create `docker-compose.prod.yml`
- [x] T7.2.3 Create `Caddyfile`
- [x] T7.2.4 Create `.env.example`
- [x] T7.3.1 Create `scripts/init-secret.sh`
- [x] T7.3.2 Create `scripts/enroll-device.sh`
- [ ] T7.4.1 Test docker compose up
- [ ] T7.4.2 Test HTTPS access
- [ ] T7.4.3 Test WebSocket connection
- [ ] T7.4.4 Test device enrollment
- [ ] T7.4.5 Test full flow (two devices)

## Phase 8: Testing & Verification
- [x] T8.1.1 Paragraph parser tests
- [x] T8.1.2 Chunking tests
- [x] T8.1.3 Signature verification tests
- [x] T8.1.4 Argon2id tests
- [x] T8.2.1 Two WS clients, presence 2/2
- [x] T8.2.2 Streaming events reconstruct paragraphs
- [x] T8.2.3 Offline gating returns send_fail
- [x] T8.2.4 Ack returns to sender
- [x] T8.2.5 Unauthorized device blocked
- [ ] AC1 Non-whitelisted device cannot see secret prompt
- [ ] AC2 Non-whitelisted device cannot connect to WS
- [ ] AC3 Whitelisted device must pass secret
- [ ] AC4 Single device online → send fails
- [ ] AC5 Two devices online → streaming works
- [ ] AC6 Paragraph boundaries preserved
- [ ] AC7 Sender receives ack → shows delivered
- [ ] AC8 Restart preserves whitelist (no messages)
- [ ] AC9 Docker deployment works

## Phase 9: Documentation
- [x] T9.1 Create README.md
- [ ] T9.2 Document environment variables
- [ ] T9.3 Document deployment steps
- [ ] T9.4 Document device enrollment
- [ ] T9.5 Create CHANGELOG.md

---

## Summary

| Phase | Completed | Total | Status |
|-------|-----------|-------|--------|
| P0 | 3 | 3 | ✅ |
| P1 | 6 | 6 | ✅ |
| P2 | 9 | 9 | ✅ |
| P3 | 10 | 10 | ✅ |
| P4 | 13 | 13 | ✅ |
| P5 | 21 | 21 | ✅ |
| P6 | 3 | 3 | ✅ |
| P7 | 9 | 14 | 🔶 |
| P8 | 9 | 18 | 🔶 |
| P9 | 1 | 5 | 🔶 |
| **Total** | **84** | **102** | **82%** |

Core implementation complete. Docker deployment testing and acceptance tests pending.
