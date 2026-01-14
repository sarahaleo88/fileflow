# PROJECT KNOWLEDGE BASE (Frontend: web/static)

**Generated:** 2026-01-14
**Context:** Vanilla JS Frontend for FileFlow

## OVERVIEW
Ephemeral text transfer client. Pure Vanilla JS (No Frameworks).
Focus: Device attestation, real-time streaming, and memory-only persistence.

## STRUCTURE
```
web/static/
├── app.js         # Main application logic (IIFE module)
├── index.html     # Single page layout, view containers
└── style.css      # Layout, presence indicators, message bubbles
```

## WHERE TO LOOK
| Area | Location | Notes |
|------|----------|-------|
| **DOM** | `app.js` (top) | Elements cached with `$` prefix |
| **WS** | `connectWebSocket` | JSON protocol, auto-reconnect logic |
| **Crypto** | `crypto.subtle` | ECDSA P-256 for attestation |
| **Storage** | `IndexedDB` | Stores `keypair` only, NO message content |
| **Events** | `handleEvent` | Dispatcher for `msg_start`, `para_chunk`, etc. |

## CONVENTIONS
- **Module**: Entire logic wrapped in an IIFE `FileFlow` module.
- **DOM Naming**: Prefix variables holding DOM elements with `$` (e.g., `$app`, `$viewMain`).
- **Real-time**: Text is parsed into paragraphs and streamed in chunks (`CHUNK_SIZE`).
- **Persistence**: IndexedDB used ONLY for device identity (`keypair`).
- **Views**: Switched via `showView(name)` by toggling `display: flex/none`.

## ANTI-PATTERNS
- **Frameworks**: NO React, Vue, or any external JS libraries allowed.
- **Main Thread**: Avoid blocking heavy crypto; use `async/await`.
- **Content Storage**: NEVER store message text in `localStorage` or `IndexedDB`.
- **Globals**: Keep everything inside the IIFE; avoid polluting `window`.

## HIGHLIGHTS
- **Identity**: `deviceId` is a SHA-256 hash of the public key (Base64URL).
- **Security**: Mandatory `crypto.subtle.sign` for device attestation challenge.
- **Protocol**: Envelope format `{ t: type, v: value, ts: timestamp }`.
