# FileFlow

Real-time, ephemeral text transfer between two authorized devices.

## Overview

FileFlow enables secure, paragraph-by-paragraph streaming of text content between exactly two pre-authorized devices. Messages are **never stored** - content exists only in transit and in browser memory.

### Key Features

- **Ephemeral messaging** - No server-side message persistence
- **Device whitelisting** - Only pre-enrolled devices can connect
- **Two-phase authentication** - Device attestation + shared secret
- **Real-time streaming** - Text streams paragraph-by-paragraph as you type
- **Online-only delivery** - Both devices must be online to exchange messages

### Security Model

1. **Device Attestation**: Each device generates an ECDSA P-256 keypair stored in browser IndexedDB. The public key must be whitelisted server-side before the device can authenticate.

2. **Shared Secret**: After device attestation, users must enter a shared secret (Argon2id hashed) to establish a session.

3. **No Content Logging**: Message content is never logged or stored on the server.

---

## Quick Start

### Prerequisites

- Docker and Docker Compose
- A domain with DNS pointing to your server (for automatic HTTPS)

### 1. Clone and Configure

```bash
git clone https://github.com/your/fileflow.git
cd fileflow

# Copy environment template
cp deployment/.env.example deployment/.env

# Edit with your domain
nano deployment/.env
```

Required environment variables:

```bash
APP_DOMAIN=fileflow.example.com
BOOTSTRAP_TOKEN=your-secure-random-token
```

### 2. Start Services

```bash
cd deployment
docker compose up -d
```

### 3. Initialize Shared Secret

```bash
docker compose exec app ./scripts/init-secret.sh
```

You'll be prompted to enter the shared secret that users will use to authenticate.

### 4. Enroll Devices

On each device you want to authorize:

1. Open `https://your-domain.com` in a browser
2. The browser console will show the device's public key info
3. Copy the device_id from the console

Then on the server:

```bash
docker compose exec app ./scripts/enroll-device.sh <device_id> "Device Label"
```

### 5. Use FileFlow

1. Open the app on an enrolled device
2. Enter the shared secret when prompted
3. Repeat on the second device
4. Start sending messages!

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_DOMAIN` | Yes | - | Domain for CORS/origin validation and cookie scope |
| `BOOTSTRAP_TOKEN` | Yes | - | Admin token for device enrollment API |
| `SESSION_KEY` | Yes (prod) | - | HMAC key for session + device ticket tokens |
| `SQLITE_PATH` | No | `/data/fileflow.db` | Path to SQLite database file |
| `RATE_LIMIT_RPS` | No | `5` | Requests per second rate limit per IP |
| `MAX_WS_MSG_BYTES` | No | `262144` | Maximum WebSocket message size (256KB) |
| `SESSION_TTL_HOURS` | No | `12` | Session cookie time-to-live (hours) |
| `SECURE_COOKIES` | No | `true` | Require Secure cookies (HTTPS) |
| `TRUSTED_PROXY_CIDRS` | No | - | Comma-separated CIDR/IPs to trust for X-Forwarded-For |

---

## Deployment

### Development

```bash
cd deployment
docker compose up
```

Access at `http://localhost:8080` (HTTP only, no HTTPS in dev mode).

### Production

```bash
cd deployment
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

This enables:
- Automatic HTTPS via Caddy
- JSON logging
- Resource limits
- Restart policies

### Architecture

```
                    ┌─────────────┐
                    │   Caddy     │
                    │ (HTTPS/WSS) │
                    └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │  FileFlow   │
                    │   (Go App)  │
                    └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │   SQLite    │
                    │ (Whitelist) │
                    └─────────────┘
```

---

## Device Enrollment

Devices must be whitelisted before they can authenticate.

### Getting Device ID

1. Open FileFlow in a browser on the device
2. Open browser DevTools (F12) → Console
3. The device_id will be logged on page load
4. Or check the network request to `/api/device/challenge` - the `device_id` is in the request body

### Enrolling via Script

```bash
# SSH into your server, then:
docker compose exec app ./scripts/enroll-device.sh <device_id> "My iPhone"
```

### Enrolling via API

```bash
curl -X POST https://your-domain.com/api/admin/devices \
  -H "X-Admin-Bootstrap: your-bootstrap-token" \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "abc123...",
    "pub_jwk": {...},
    "label": "My iPhone"
  }'
```

### Listing Enrolled Devices

```bash
docker compose exec app sqlite3 /data/fileflow.db \
  "SELECT device_id, label, created_at FROM devices;"
```

---

## API Reference

### Health Check

```
GET /healthz
Response: {"ok": true}
```

### Authentication Flow

```
1. POST /api/device/challenge
   Body: { device_id, pub_jwk }
   Response: { challenge_id, nonce }

2. POST /api/device/attest
   Body: { challenge_id, device_id, signature }
   Response: Sets device_ticket cookie
   
3. POST /api/login
   Requires: device_ticket cookie
   Body: { secret, device_id }
   Response: Sets ff_session cookie
```

### WebSocket

```
GET /ws
Requires: session cookie + device_ticket cookie
Protocol: JSON events with envelope { t: type, v: value, ts: timestamp }
```

Event types: `presence`, `msg_start`, `para_start`, `para_chunk`, `para_end`, `msg_end`, `ack`, `send_fail`

---

## Development

### Build from Source

```bash
# Requires Go 1.21+
go build -o fileflow ./cmd/server

# Run locally
SQLITE_PATH=./data/fileflow.db \
APP_DOMAIN=localhost \
BOOTSTRAP_TOKEN=dev-token \
./fileflow
```

### Run Tests

```bash
go test ./...

# With race detection
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Project Structure

```
fileflow/
├── cmd/server/          # Entry point
├── internal/
│   ├── auth/            # Authentication (challenge, session, secret)
│   ├── handler/         # HTTP handlers and middleware
│   ├── realtime/        # WebSocket hub and clients
│   └── store/           # SQLite data layer
├── web/static/          # Frontend (vanilla JS)
├── deployment/          # Docker and Caddy config
└── scripts/             # Admin utilities
```

---

## Troubleshooting

### "Unauthorized device" message

The device is not whitelisted. Enroll it using the steps above.

### "Peer offline" when sending

Both devices must be online and authenticated to send messages. Check that:
1. The other device has the app open
2. The other device has entered the correct secret
3. The WebSocket connection is established (check browser DevTools)

### Session expired

Sessions expire after 12 hours by default. Re-enter the shared secret to re-authenticate.

### HTTPS certificate issues

Caddy automatically provisions certificates via Let's Encrypt. Ensure:
1. Your domain's DNS points to the server
2. Ports 80 and 443 are accessible
3. Check Caddy logs: `docker compose logs caddy`

---

## Security Considerations

- **Never share your BOOTSTRAP_TOKEN** - it allows adding devices to the whitelist
- **Use a strong shared secret** - it's hashed with Argon2id but should still be complex
- **HTTPS is required in production** - session cookies are Secure-only
- **Content is ephemeral but visible in memory** - clear browser data for full cleanup

---

## License

MIT
