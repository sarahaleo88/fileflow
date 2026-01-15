package handler

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/limit"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
	"golang.org/x/time/rate"
)

func setupTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	secretHash, _ := auth.HashSecret("test-secret")
	tokenManager := auth.NewTokenManager([]byte("test-key"))
	loginLimiter := limit.NewIPLimiter(rate.Inf, 1000)
	connLimiter := limit.NewConnLimiter(5, 100)
	challengeStore := auth.NewChallengeStore(500 * time.Millisecond)
	hub := realtime.NewHub()
	go hub.Run()

	h := New(Config{
		Store:          s,
		TokenManager:   tokenManager,
		LoginLimiter:   loginLimiter,
		ConnLimiter:    connLimiter,
		SecretHash:     secretHash,
		ChallengeStore: challengeStore,
		Hub:            hub,
		SecureCookies:  false,
		SessionTTL:     time.Hour,
		AllowedOrigin:  "",
		BootstrapToken: "test-bootstrap-token",
	})

	cleanup := func() {
		hub.Stop()
		challengeStore.Stop()
		s.Close()
	}

	return h, cleanup
}

type testDevice struct {
	id   string
	jwk  map[string]interface{}
	priv *ecdsa.PrivateKey
}

func newTestDevice(t *testing.T) testDevice {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	x := pad32(priv.PublicKey.X.Bytes())
	y := pad32(priv.PublicKey.Y.Bytes())

	jwk := &auth.ECPublicJWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(x),
		Y:   base64.RawURLEncoding.EncodeToString(y),
	}

	deviceID, err := auth.DeviceIDFromJWK(jwk)
	if err != nil {
		t.Fatalf("Failed to compute device ID: %v", err)
	}

	jwkMap := map[string]interface{}{
		"kty": jwk.Kty,
		"crv": jwk.Crv,
		"x":   jwk.X,
		"y":   jwk.Y,
	}

	return testDevice{id: deviceID, jwk: jwkMap, priv: priv}
}

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func enrollTestDevice(t *testing.T, h *Handler, device testDevice) {
	t.Helper()

	jwkJSON, err := json.Marshal(device.jwk)
	if err != nil {
		t.Fatalf("Failed to marshal JWK: %v", err)
	}

	if err := h.store.AddDevice(&store.Device{
		DeviceID:   device.id,
		PubJWKJSON: string(jwkJSON),
		Label:      "Test Device",
		CreatedAt:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Failed to add device: %v", err)
	}
}

func decodeB64URL(t *testing.T, input string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil {
		t.Fatalf("Failed to decode base64url: %v", err)
	}
	return b
}

func signNonce(t *testing.T, priv *ecdsa.PrivateKey, nonce []byte) string {
	t.Helper()
	h := sha256.Sum256(nonce)
	r, s, err := ecdsa.Sign(rand.Reader, priv, h[:])
	if err != nil {
		t.Fatalf("Failed to sign nonce: %v", err)
	}
	sig := append(pad32(r.Bytes()), pad32(s.Bytes())...)
	return base64.RawURLEncoding.EncodeToString(sig)
}

func issueDeviceTicket(t *testing.T, h *Handler, device testDevice) string {
	t.Helper()

	challengeBody, _ := json.Marshal(map[string]interface{}{
		"device_id": device.id,
		"pub_jwk":   device.jwk,
	})
	chReq := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBuffer(challengeBody))
	chReq.Header.Set("Content-Type", "application/json")
	chRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(chRec, chReq)

	if chRec.Code != http.StatusOK {
		t.Fatalf("Challenge failed: status=%d body=%s", chRec.Code, chRec.Body.String())
	}

	var chResp struct {
		ChallengeID string `json:"challenge_id"`
		Nonce       string `json:"nonce"`
	}
	if err := json.NewDecoder(chRec.Body).Decode(&chResp); err != nil {
		t.Fatalf("Failed to decode challenge response: %v", err)
	}

	sig := signNonce(t, device.priv, decodeB64URL(t, chResp.Nonce))

	attestBody, _ := json.Marshal(map[string]string{
		"challenge_id": chResp.ChallengeID,
		"device_id":    device.id,
		"signature":    sig,
	})
	atReq := httptest.NewRequest(http.MethodPost, "/api/device/attest", bytes.NewBuffer(attestBody))
	atReq.Header.Set("Content-Type", "application/json")
	atRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(atRec, atReq)

	if atRec.Code != http.StatusOK {
		t.Fatalf("Attest failed: status=%d body=%s", atRec.Code, atRec.Body.String())
	}

	for _, c := range atRec.Result().Cookies() {
		if c.Name == "device_ticket" {
			return c.Value
		}
	}
	t.Fatalf("device_ticket cookie not set")
	return ""
}

func TestHealthz(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var resp map[string]bool
	json.NewDecoder(rec.Body).Decode(&resp)

	if !resp["ok"] {
		t.Error("Expected ok: true")
	}
}

func TestLoginEndpoint(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("CorrectSecret", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		ticket := issueDeviceTicket(t, h, device)

		body := `{"secret":"test-secret", "device_id":"` + device.id + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: ticket})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]bool
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp["authed"] {
			t.Error("Expected authed: true")
		}

		cookies := rec.Result().Cookies()
		hasSession := false
		for _, c := range cookies {
			if c.Name == "ff_session" {
				hasSession = true
				break
			}
		}
		if !hasSession {
			t.Error("Expected ff_session cookie to be set")
		}
	})

	t.Run("WrongSecret", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		ticket := issueDeviceTicket(t, h, device)

		body := `{"secret":"wrong-secret", "device_id":"` + device.id + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: ticket})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		var resp map[string]bool
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp["authed"] {
			t.Error("Expected authed: false")
		}
	})

	t.Run("UnenrolledDevice", func(t *testing.T) {
		ticket, _ := h.tokenManager.Sign("unenrolled-123", auth.TokenVersionDeviceTicket, time.Minute)
		body := `{"secret":"test-secret", "device_id":"unenrolled-123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: ticket})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rec.Code)
		}
	})

	t.Run("MissingDeviceID", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		ticket := issueDeviceTicket(t, h, device)

		body := `{"secret":"test-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: ticket})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("InvalidDeviceID", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		ticket := issueDeviceTicket(t, h, device)

		body := `{"secret":"test-secret", "device_id":"short"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: ticket})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rec.Code)
		}
	})

	t.Run("MissingDeviceTicket", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)

		body := `{"secret":"test-secret", "device_id":"` + device.id + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("InvalidDeviceTicket", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)

		body := `{"secret":"test-secret", "device_id":"` + device.id + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: "tampered"})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("ExpiredDeviceTicket", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		expired, _ := h.tokenManager.Sign(device.id, auth.TokenVersionDeviceTicket, -time.Minute)

		body := `{"secret":"test-secret", "device_id":"` + device.id + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: expired})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("InvalidMethod", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", rec.Code)
		}
	})
}

func TestAdminDevices(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("RegisterDevice", func(t *testing.T) {
		device := newTestDevice(t)
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
			"label":     "New Device",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "test-bootstrap-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DuplicateDevice", func(t *testing.T) {
		device := newTestDevice(t)
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
			"label":     "New Device",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "test-bootstrap-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		// Try to register the same device again
		req = httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "test-bootstrap-token")
		rec = httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("Expected status 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		device := newTestDevice(t)
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
			"label":     "New Device 2",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "invalid-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})
}

func TestDeviceChallengeAttest(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("UnenrolledDeviceChallenge", func(t *testing.T) {
		device := newTestDevice(t)
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rec.Code)
		}
	})

	t.Run("ChallengeAttestSuccess", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)

		ticket := issueDeviceTicket(t, h, device)
		if ticket == "" {
			t.Fatal("Expected device_ticket")
		}
	})

	t.Run("ExpiredChallenge", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)

		challengeBody, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
		})
		chReq := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBuffer(challengeBody))
		chReq.Header.Set("Content-Type", "application/json")
		chRec := httptest.NewRecorder()
		h.Routes().ServeHTTP(chRec, chReq)

		if chRec.Code != http.StatusOK {
			t.Fatalf("Challenge failed: status=%d body=%s", chRec.Code, chRec.Body.String())
		}

		var chResp struct {
			ChallengeID string `json:"challenge_id"`
			Nonce       string `json:"nonce"`
		}
		if err := json.NewDecoder(chRec.Body).Decode(&chResp); err != nil {
			t.Fatalf("Failed to decode challenge response: %v", err)
		}

		time.Sleep(600 * time.Millisecond)

		sig := signNonce(t, device.priv, decodeB64URL(t, chResp.Nonce))
		attestBody, _ := json.Marshal(map[string]string{
			"challenge_id": chResp.ChallengeID,
			"device_id":    device.id,
			"signature":    sig,
		})
		atReq := httptest.NewRequest(http.MethodPost, "/api/device/attest", bytes.NewBuffer(attestBody))
		atReq.Header.Set("Content-Type", "application/json")
		atRec := httptest.NewRecorder()
		h.Routes().ServeHTTP(atRec, atReq)

		if atRec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", atRec.Code)
		}

		var resp APIResponse
		json.NewDecoder(atRec.Body).Decode(&resp)
		if resp.Error == nil || resp.Error.Code != "CHALLENGE_EXPIRED" {
			t.Errorf("Expected CHALLENGE_EXPIRED, got %#v", resp.Error)
		}
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)

		challengeBody, _ := json.Marshal(map[string]interface{}{
			"device_id": device.id,
			"pub_jwk":   device.jwk,
		})
		chReq := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBuffer(challengeBody))
		chReq.Header.Set("Content-Type", "application/json")
		chRec := httptest.NewRecorder()
		h.Routes().ServeHTTP(chRec, chReq)

		if chRec.Code != http.StatusOK {
			t.Fatalf("Challenge failed: status=%d body=%s", chRec.Code, chRec.Body.String())
		}

		var chResp struct {
			ChallengeID string `json:"challenge_id"`
			Nonce       string `json:"nonce"`
		}
		if err := json.NewDecoder(chRec.Body).Decode(&chResp); err != nil {
			t.Fatalf("Failed to decode challenge response: %v", err)
		}

		attestBody, _ := json.Marshal(map[string]string{
			"challenge_id": chResp.ChallengeID,
			"device_id":    device.id,
			"signature":    "invalid",
		})
		atReq := httptest.NewRequest(http.MethodPost, "/api/device/attest", bytes.NewBuffer(attestBody))
		atReq.Header.Set("Content-Type", "application/json")
		atRec := httptest.NewRecorder()
		h.Routes().ServeHTTP(atRec, atReq)

		if atRec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", atRec.Code)
		}
	})
}

func TestSessionEndpoint(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	sid := "test-sid"
	validToken, _ := h.tokenManager.Sign(sid, auth.TokenVersionSession, time.Hour)

	t.Run("ValidSession", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		req.AddCookie(&http.Cookie{Name: "ff_session", Value: validToken})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		var resp map[string]bool
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp["authed"] {
			t.Error("Expected authed: true")
		}
	})

	t.Run("InvalidSession", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		req.AddCookie(&http.Cookie{Name: "ff_session", Value: "invalid-token"})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		var resp map[string]bool
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp["authed"] {
			t.Error("Expected authed: false")
		}
	})

	t.Run("NoSession", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		var resp map[string]bool
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp["authed"] {
			t.Error("Expected authed: false")
		}
	})
}

func TestPresenceEndpoint(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("WithoutSession", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/presence", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("WithSession", func(t *testing.T) {
		sid := "test-sid"
		validToken, _ := h.tokenManager.Sign(sid, auth.TokenVersionSession, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/api/presence", nil)
		req.AddCookie(&http.Cookie{Name: "ff_session", Value: validToken})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}
	})
}

func TestWebSocketAuth(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("MissingDeviceTicket", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}

		var resp APIResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp.Error == nil || resp.Error.Code != "MISSING_DEVICE_TICKET" {
			t.Errorf("Expected MISSING_DEVICE_TICKET, got %#v", resp.Error)
		}
	})

	t.Run("ExpiredDeviceTicket", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		expired, _ := h.tokenManager.Sign(device.id, auth.TokenVersionDeviceTicket, -time.Minute)

		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: expired})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("TamperedDeviceTicket", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: "bad-token"})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("AuthorizedWebSocket", func(t *testing.T) {
		device := newTestDevice(t)
		enrollTestDevice(t, h, device)
		ticket := issueDeviceTicket(t, h, device)

		sessionToken, _ := h.tokenManager.Sign("test-sid", auth.TokenVersionSession, time.Minute)

		server := httptest.NewServer(h.Routes())
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
		header := http.Header{}
		header.Set("Cookie", fmt.Sprintf("ff_session=%s; device_ticket=%s", sessionToken, ticket))

		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("WebSocket dial failed: %v (status=%d)", err, status)
		}
		conn.Close()
	})
}
