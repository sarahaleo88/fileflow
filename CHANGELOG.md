# Changelog

All notable changes to FileFlow will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-01-13

### Added

- Initial MVP release
- **Authentication**
  - Device attestation with ECDSA P-256 signatures
  - Challenge-response flow with 60-second TTL
  - Shared secret verification with Argon2id hashing
  - Session cookies with 12-hour TTL (HttpOnly, Secure, SameSite=Strict)
  
- **Real-time Messaging**
  - WebSocket-based real-time text streaming
  - Paragraph-by-paragraph delivery with chunking
  - Online-only gating (both devices must be connected)
  - Acknowledgment system for delivery confirmation
  
- **Security**
  - Device whitelist - only pre-enrolled devices can connect
  - Two-phase authentication (device + secret)
  - No message persistence - content is never stored
  - No content logging
  - Per-IP rate limiting
  
- **Frontend**
  - Vanilla JavaScript (no frameworks)
  - WebCrypto API for key generation and signing
  - IndexedDB for persistent keypair storage
  - Streaming message rendering
  - Presence indicator
  
- **Deployment**
  - Multi-stage Dockerfile (alpine-based)
  - Docker Compose for development and production
  - Caddy reverse proxy with automatic HTTPS
  - Admin scripts for secret initialization and device enrollment
  
- **Testing**
  - Unit tests for store, auth modules
  - Integration tests for HTTP API and WebSocket

### Technical Details

- Go 1.21+ backend with gorilla/websocket
- SQLite for device whitelist and config storage
- Vanilla JavaScript frontend with WebCrypto
- Docker + Caddy for deployment

[0.1.0]: https://github.com/your/fileflow/releases/tag/v0.1.0
