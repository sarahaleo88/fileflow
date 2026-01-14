package handler

import (
	"encoding/json"
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
	store         *store.Store
	tokenManager  *auth.TokenManager
	loginLimiter  *limit.IPLimiter
	connLimiter   *limit.ConnLimiter
	secretHash    string
	hub           *realtime.Hub
	secureCookies bool
	upgrader      websocket.Upgrader
}

type Config struct {
	Store         *store.Store
	TokenManager  *auth.TokenManager
	LoginLimiter  *limit.IPLimiter
	ConnLimiter   *limit.ConnLimiter
	SecretHash    string
	Hub           *realtime.Hub
	SecureCookies bool
	AllowedOrigin string
}

func New(cfg Config) *Handler {
	h := &Handler{
		store:         cfg.Store,
		tokenManager:  cfg.TokenManager,
		loginLimiter:  cfg.LoginLimiter,
		connLimiter:   cfg.ConnLimiter,
		secretHash:    cfg.SecretHash,
		hub:           cfg.Hub,
		secureCookies: cfg.SecureCookies,
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
	mux.HandleFunc("/api/login", h.handleLogin)
	mux.HandleFunc("/api/session", h.handleSession)
	mux.HandleFunc("/api/presence", h.handlePresence)
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
		Secret string `json:"secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if err := auth.VerifySecret(req.Secret, h.secretHash); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"authed": false})
		return
	}

	sid := uuid.NewString()
	ttl := 30 * 24 * time.Hour
	token, err := h.tokenManager.Sign(sid, 1, ttl)
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

	if _, err := h.tokenManager.Verify(cookie.Value); err != nil {
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

	if _, err := h.tokenManager.Verify(cookie.Value); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session")
		return
	}

	writeSuccess(w, map[string]int{
		"online":   h.hub.OnlineCount(),
		"required": 2,
	})
}

func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("ff_session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	claims, err := h.tokenManager.Verify(cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
	client := realtime.NewClient(h.hub, conn, claims.SID, ip, h.connLimiter, 20)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
