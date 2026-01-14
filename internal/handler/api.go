package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
)

type Handler struct {
	store          *store.Store
	challengeStore *auth.ChallengeStore
	sessionStore   *auth.SessionStore
	hub            *realtime.Hub
	bootstrapToken string
	secureCookies  bool
	upgrader       websocket.Upgrader
}

type Config struct {
	Store          *store.Store
	ChallengeStore *auth.ChallengeStore
	SessionStore   *auth.SessionStore
	Hub            *realtime.Hub
	BootstrapToken string
	SecureCookies  bool
	AllowedOrigin  string
}

func New(cfg Config) *Handler {
	h := &Handler{
		store:          cfg.Store,
		challengeStore: cfg.ChallengeStore,
		sessionStore:   cfg.SessionStore,
		hub:            cfg.Hub,
		bootstrapToken: cfg.BootstrapToken,
		secureCookies:  cfg.SecureCookies,
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
	mux.HandleFunc("/api/auth/secret", h.handleAuthSecret)
	mux.HandleFunc("/api/presence", h.handlePresence)
	mux.HandleFunc("/api/admin/devices", h.handleAdminDevices)
	mux.HandleFunc("/ws", h.handleWebSocket)
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	return mux
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

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) handleDeviceChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID string                 `json:"device_id"`
		PubJWK   map[string]interface{} `json:"pub_jwk"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "device_id is required")
		return
	}

	if req.PubJWK == nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "pub_jwk is required")
		return
	}

	if err := auth.ValidateDeviceID(req.DeviceID, req.PubJWK); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", err.Error())
		return
	}

	if _, err := auth.ParseJWKPublicKey(req.PubJWK); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", err.Error())
		return
	}

	challenge, err := h.challengeStore.Generate(req.DeviceID, req.PubJWK)
	if err != nil {
		log.Printf("Failed to generate challenge: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate challenge")
		return
	}

	writeSuccess(w, map[string]string{
		"challenge_id": challenge.ID,
		"nonce":        challenge.NonceBase64(),
	})
}

func (h *Handler) handleDeviceAttest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChallengeID string `json:"challenge_id"`
		DeviceID    string `json:"device_id"`
		Signature   string `json:"signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	challenge, ok := h.challengeStore.Get(req.ChallengeID)
	if !ok {
		writeError(w, http.StatusBadRequest, "CHALLENGE_EXPIRED", "Challenge not found or expired")
		return
	}

	h.challengeStore.Delete(req.ChallengeID)

	if challenge.DeviceID != req.DeviceID {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_ID", "Device ID mismatch")
		return
	}

	pubKey, err := auth.ParseJWKPublicKey(challenge.PubJWK)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", err.Error())
		return
	}

	if err := auth.VerifySignature(pubKey, challenge.Nonce, req.Signature); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"device_ok": false})
		return
	}

	whitelisted, err := h.store.IsWhitelisted(req.DeviceID)
	if err != nil {
		log.Printf("Failed to check whitelist: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Database error")
		return
	}

	if !whitelisted {
		writeJSON(w, http.StatusOK, map[string]bool{"device_ok": false})
		return
	}

	auth.SetDeviceTicketCookie(w, req.DeviceID, 10*time.Minute, h.secureCookies)
	writeJSON(w, http.StatusOK, map[string]bool{"device_ok": true})
}

func (h *Handler) handleAuthSecret(w http.ResponseWriter, r *http.Request) {
	deviceID := auth.GetDeviceTicketFromRequest(r)
	if deviceID == "" {
		writeError(w, http.StatusUnauthorized, "MISSING_DEVICE_TICKET", "Device ticket required")
		return
	}

	var req struct {
		Secret string `json:"secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	secretHash, err := h.store.GetConfig(store.ConfigKeySecretHash)
	if err != nil {
		log.Printf("Failed to get secret hash: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Configuration error")
		return
	}

	if err := auth.VerifySecret(req.Secret, secretHash); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"authed": false})
		return
	}

	session, err := h.sessionStore.Create(deviceID)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Session creation failed")
		return
	}

	if err := h.store.UpdateLastSeen(deviceID); err != nil {
		log.Printf("Failed to update last seen: %v", err)
	}

	auth.SetSessionCookie(w, session, h.secureCookies)
	writeJSON(w, http.StatusOK, map[string]bool{"authed": true})
}

func (h *Handler) handlePresence(w http.ResponseWriter, r *http.Request) {
	sessionID := auth.GetSessionFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
		return
	}

	if _, err := h.sessionStore.Get(sessionID); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session")
		return
	}

	writeSuccess(w, map[string]int{
		"online":   h.hub.OnlineCount(),
		"required": 2,
	})
}

func (h *Handler) handleAdminDevices(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := auth.GetSessionFromRequest(r)
	if sessionID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	session, err := h.sessionStore.Get(sessionID)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := realtime.NewClient(h.hub, conn, session.DeviceID)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
