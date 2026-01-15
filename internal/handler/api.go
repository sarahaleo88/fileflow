package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/limit"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
)

type Handler struct {
	store           *store.Store
	tokenManager    *auth.TokenManager
	loginLimiter    *limit.IPLimiter
	connLimiter     *limit.ConnLimiter
	secretHash      string
	bootstrapToken  string
	hub             *realtime.Hub
	secureCookies   bool
	sessionTTL      time.Duration
	deviceTicketTTL time.Duration
	challengeStore  *auth.ChallengeStore
	maxWSMsgBytes   int
	upgrader        websocket.Upgrader
}

type Config struct {
	Store           *store.Store
	TokenManager    *auth.TokenManager
	LoginLimiter    *limit.IPLimiter
	ConnLimiter     *limit.ConnLimiter
	SecretHash      string
	BootstrapToken  string
	Hub             *realtime.Hub
	SecureCookies   bool
	SessionTTL      time.Duration
	DeviceTicketTTL time.Duration
	ChallengeStore  *auth.ChallengeStore
	MaxWSMsgBytes   int
	AllowedOrigin   string
}

func New(cfg Config) *Handler {
	ttl := cfg.DeviceTicketTTL
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	maxWSMsgBytes := cfg.MaxWSMsgBytes
	if maxWSMsgBytes == 0 {
		maxWSMsgBytes = realtime.MaxMessageSize
	}
	challengeStore := cfg.ChallengeStore
	if challengeStore == nil {
		challengeStore = auth.NewChallengeStore(60 * time.Second)
	}

	h := &Handler{
		store:           cfg.Store,
		tokenManager:    cfg.TokenManager,
		loginLimiter:    cfg.LoginLimiter,
		connLimiter:     cfg.ConnLimiter,
		secretHash:      cfg.SecretHash,
		bootstrapToken:  cfg.BootstrapToken,
		hub:             cfg.Hub,
		secureCookies:   cfg.SecureCookies,
		sessionTTL:      cfg.SessionTTL,
		deviceTicketTTL: ttl,
		challengeStore:  challengeStore,
		maxWSMsgBytes:   maxWSMsgBytes,
	}

	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if cfg.AllowedOrigin == "" {
				return true
			}
			origin := r.Header.Get("Origin")
			return origin == cfg.AllowedOrigin || origin == "https://"+cfg.AllowedOrigin
		},
	}

	return h
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/api/device/challenge", h.handleDeviceChallenge)
	mux.HandleFunc("/api/device/attest", h.handleDeviceAttest)
	mux.HandleFunc("/api/login", h.handleLogin)
	mux.HandleFunc("/api/session", h.handleSession)
	mux.HandleFunc("/api/presence", h.handlePresence)
	mux.HandleFunc("/api/admin/devices", h.handleAdminDevices)
	mux.HandleFunc("/ws", h.handleWebSocket)
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	return mux
}

// ... existing code ...

func (h *Handler) handleAdminDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	token := r.Header.Get("X-Admin-Bootstrap")
	if token == "" || token != h.bootstrapToken {
		writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid bootstrap token")
		return
	}

	var req struct {
		DeviceID string                 `json:"device_id"`
		PubJWK   map[string]interface{} `json:"pub_jwk"`
		Label    string                 `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if err := auth.ValidateDeviceID(req.DeviceID, req.PubJWK); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", err.Error())
		return
	}

	jwkJSON, err := json.Marshal(req.PubJWK)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "Failed to serialize public key")
		return
	}

	device := &store.Device{
		DeviceID:   req.DeviceID,
		PubJWKJSON: string(jwkJSON),
		Label:      req.Label,
		CreatedAt:  time.Now().UnixMilli(),
	}

	if err := h.store.AddDevice(device); err != nil {
		if err == store.ErrDeviceExists {
			writeError(w, http.StatusConflict, "DEVICE_EXISTS", "Device already enrolled")
			return
		}
		log.Printf("Failed to add device: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add device")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"added": true})
}

func (h *Handler) handleDeviceChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req struct {
		DeviceID string                 `json:"device_id"`
		PubJWK   map[string]interface{} `json:"pub_jwk"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if !auth.ValidateDeviceIDFormat(req.DeviceID) {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "Invalid device ID format")
		return
	}

	_, reqJWK, err := auth.ParseECPublicJWKMap(req.PubJWK)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "Invalid public key")
		return
	}

	device, err := h.store.GetDevice(req.DeviceID)
	if err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusForbidden, "DEVICE_NOT_ENROLLED", "Device not enrolled")
			return
		}
		log.Printf("Failed to load device: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load device")
		return
	}

	_, storedJWK, err := auth.ParseECPublicJWKBytes([]byte(device.PubJWKJSON))
	if err != nil || !auth.EqualECPublicJWK(reqJWK, storedJWK) {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "Public key does not match enrollment")
		return
	}

	challenge, err := h.challengeStore.Create(req.DeviceID)
	if err != nil {
		log.Printf("Failed to create challenge: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create challenge")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"challenge_id": challenge.ID,
		"nonce":        base64.RawURLEncoding.EncodeToString(challenge.Nonce),
	})
}

func (h *Handler) handleDeviceAttest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req struct {
		ChallengeID string `json:"challenge_id"`
		DeviceID    string `json:"device_id"`
		Signature   string `json:"signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.ChallengeID == "" || !auth.ValidateDeviceIDFormat(req.DeviceID) {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "Invalid request")
		return
	}

	challenge, err := h.challengeStore.Consume(req.ChallengeID)
	if err != nil {
		if errors.Is(err, auth.ErrChallengeExpired) || errors.Is(err, auth.ErrChallengeNotFound) {
			writeError(w, http.StatusBadRequest, "CHALLENGE_EXPIRED", "Challenge expired")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read challenge")
		return
	}

	if challenge.DeviceID != req.DeviceID {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "Device mismatch")
		return
	}

	device, err := h.store.GetDevice(req.DeviceID)
	if err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusForbidden, "DEVICE_NOT_ENROLLED", "Device not enrolled")
			return
		}
		log.Printf("Failed to load device: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load device")
		return
	}

	pubKey, _, err := auth.ParseECPublicJWKBytes([]byte(device.PubJWKJSON))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "Invalid enrolled public key")
		return
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", "Invalid signature")
		return
	}

	if !auth.VerifyECDSASignature(pubKey, challenge.Nonce, sigBytes) {
		writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", "Signature verification failed")
		return
	}

	ticket, err := h.tokenManager.Sign(req.DeviceID, auth.TokenVersionDeviceTicket, h.deviceTicketTTL)
	if err != nil {
		log.Printf("Failed to sign device ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to sign ticket")
		return
	}

	auth.SetDeviceTicketCookie(w, ticket, h.deviceTicketTTL, h.secureCookies)
	writeJSON(w, http.StatusOK, map[string]bool{"device_ok": true})
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errMissingDeviceTicket = errors.New("missing device ticket")

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: message},
	})
}

func (h *Handler) verifyDeviceTicket(r *http.Request) (string, error) {
	cookie, err := r.Cookie("device_ticket")
	if err != nil {
		return "", errMissingDeviceTicket
	}

	claims, err := h.tokenManager.VerifyWithVersion(cookie.Value, auth.TokenVersionDeviceTicket)
	if err != nil {
		return "", err
	}

	if !auth.ValidateDeviceIDFormat(claims.SID) {
		return "", errors.New("invalid device id")
	}

	return claims.SID, nil
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	ip := getClientIP(r)
	if !h.loginLimiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
		return
	}

	var req struct {
		Secret   string `json:"secret"`
		DeviceID string `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	deviceID, err := h.verifyDeviceTicket(r)
	if err != nil {
		if errors.Is(err, errMissingDeviceTicket) {
			writeError(w, http.StatusUnauthorized, "MISSING_DEVICE_TICKET", "Device ticket required")
			return
		}
		writeError(w, http.StatusUnauthorized, "INVALID_DEVICE_TICKET", "Invalid device ticket")
		return
	}

	if req.DeviceID == "" {
		writeError(w, http.StatusUnauthorized, "DEVICE_REQUIRED", "Device ID is required")
		return
	}
	if !auth.ValidateDeviceIDFormat(req.DeviceID) {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "Invalid device ID format")
		return
	}
	if req.DeviceID != deviceID {
		writeError(w, http.StatusUnauthorized, "DEVICE_TICKET_MISMATCH", "Device ticket mismatch")
		return
	}

	if _, err := h.store.GetDevice(deviceID); err != nil {
		if err == store.ErrDeviceNotFound {
			writeError(w, http.StatusForbidden, "DEVICE_NOT_ENROLLED", "Device not enrolled")
			return
		}
		log.Printf("Store error during login: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	// Verify Shared Secret
	if err := auth.VerifySecret(req.Secret, h.secretHash); err != nil {
		// Return generic error to avoid enumeration
		writeJSON(w, http.StatusOK, map[string]bool{"authed": false})
		return
	}

	sid := uuid.NewString()
	ttl := h.sessionTTL
	token, err := h.tokenManager.Sign(sid, auth.TokenVersionSession, ttl)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate token")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ff_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"authed": true})
}

func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("ff_session")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"authed": false})
		return
	}

	if _, err := h.tokenManager.VerifyWithVersion(cookie.Value, auth.TokenVersionSession); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"authed": false})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"authed": true})
}

func (h *Handler) handlePresence(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("ff_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
		return
	}

	if _, err := h.tokenManager.VerifyWithVersion(cookie.Value, auth.TokenVersionSession); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session")
		return
	}

	writeSuccess(w, map[string]int{
		"online":   h.hub.OnlineCount(),
		"required": 2,
	})
}

func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	deviceID, err := h.verifyDeviceTicket(r)
	if err != nil {
		if errors.Is(err, errMissingDeviceTicket) {
			writeError(w, http.StatusUnauthorized, "MISSING_DEVICE_TICKET", "Device ticket required")
			return
		}
		writeError(w, http.StatusUnauthorized, "INVALID_DEVICE_TICKET", "Invalid device ticket")
		return
	}

	if _, err := h.store.GetDevice(deviceID); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusForbidden, "DEVICE_NOT_ENROLLED", "Device not enrolled")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	cookie, err := r.Cookie("ff_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
		return
	}

	claims, err := h.tokenManager.VerifyWithVersion(cookie.Value, auth.TokenVersionSession)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session")
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	ip := getClientIP(r)
	if h.connLimiter != nil && !h.connLimiter.Increment(ip) {
		conn.Close()
		log.Printf("Connection limit exceeded for %s", ip)
		return
	}

	// Use Claims SID as DeviceID (now ClientID)
	// Rate limit: 20 messages/second per client
	client := realtime.NewClient(h.hub, conn, claims.SID, ip, h.connLimiter, 20, h.maxWSMsgBytes)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
