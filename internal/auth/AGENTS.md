# AUTH PACKAGE KNOWLEDGE BASE

**Context:** internal/auth - Stateless session and secret verification

## OVERVIEW
Lightweight security package providing Argon2id secret verification and HMAC-SHA256 stateless session tokens.

## STRUCTURE
- `secret.go`: Argon2id password hashing and constant-time verification.
- `token.go`: HMAC-signed session tokens (`Claims`: `ver`, `sid`, `iat`, `exp`).
- `session.go`: Legacy session types and cookie helpers (partially deprecated by stateless tokens).

## WHERE TO LOOK
| Type/Function | Location | Purpose |
|---------------|----------|---------|
| `TokenManager` | `token.go` | HMAC-SHA256 signing and verification of session tokens. |
| `VerifySecret` | `secret.go` | Constant-time Argon2id hash verification. |
| `HashSecret` | `secret.go` | Secure Argon2id password hashing with random salt. |
| `Claims` | `token.go` | JSON-serializable session data structure. |

## CONVENTIONS
- **Hashing**: Use Argon2id with parameters: time=1, memory=64MB, threads=4.
- **Tokens**: Stateless HMAC-SHA256. Format: `base64(payload).base64(signature)`.
- **Security**: Always use `subtle.ConstantTimeCompare` for hash and signature checks.
- **Cookies**: HTTP-only, Secure (in prod), SameSite=Strict.

## ANTI-PATTERNS
- **Logging**: NEVER log raw secrets or session tokens.
- **Persistence**: Auth is stateless; do not add database dependencies for session storage.
- **Hashing**: Avoid using legacy `bcrypt` or `sha1` for password storage.
- **Comparison**: NEVER use `==` for sensitive byte comparisons.

## HIGHLIGHTS
- **TokenManager**: Centralizes session security with a single server-side secret key.
- **Statelessness**: Sessions are fully contained in signed cookies, enabling easy restarts.
- **Device Attestation**: Legacy ECDSA code exists in tests but is currently disabled in favor of shared-secret model.
