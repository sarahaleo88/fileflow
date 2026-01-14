# KNOWLEDGE BASE: internal/realtime

**Context:** WebSocket management, event-driven streaming, and peer-to-peer message forwarding.

## OVERVIEW
Handles real-time communication between authorized devices. The server acts as a zero-persistence forwarder, ensuring that message content only exists in transit.

## STRUCTURE
- `hub.go`: Central registry and event loop for client management and broadcasting.
- `client.go`: WebSocket wrapper handling read/write pumps, rate limiting, and message validation.
- `events.go`: Event envelope definitions and serialization logic.

## WHERE TO LOOK
- **Hub**: `Hub.Run()` is the main event loop managing `register`/`unregister` channels and presence broadcasting.
- **Client**: `ReadPump()` handles incoming WS messages; `WritePump()` manages outgoing buffers and pings.
- **Events**: `Event` struct defines the `{t, v, ts}` envelope used for all communications.

## CONVENTIONS
- **Envelope Format**: All messages use `{"t": type, "v": value, "ts": timestamp}`.
- **Max Bytes**:
    - `MaxMessageSize`: 256KB (total message limit).
    - `MaxChunkSize`: 4KB (per `para_chunk` payload).
- **Limits**: Max 512 paragraphs per message.
- **Online-Only**: Messages are only forwarded if `Hub.HasPeer(sender)` returns true.

## ANTI-PATTERNS
- **Blocking Send**: Avoid blocking the Hub event loop. `Client.send` is buffered (256); if full, the client is unregistered.
- **Infinite Loops**: Always ensure `ReadPump` and `WritePump` exit on connection close or error.
- **Persistence**: NEVER store message content (`para_chunk`) in memory or on disk. Forward and forget.

## KEY COMPONENTS
- **Event Loop**: Centralized in `Hub.Run()` to avoid race conditions in client management.
- **Broadcast Channels**: Used for presence updates (`broadcastPresence`) and message forwarding.
