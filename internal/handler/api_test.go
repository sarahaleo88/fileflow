package handler

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
)

// generateTestKey generates a valid ECDSA P-256 key pair for testing
func generateTestKey(t *testing.T) (map[string]interface{}, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubJWK := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.Y.Bytes()),
	}

	deviceID, err := auth.ComputeDeviceID(pubJWK)
	if err != nil {
		t.Fatalf("Failed to compute device ID: %v", err)
	}

	return pubJWK, deviceID
}

func setupTestHandler(t *testing.T) (*Handler, *store.Store, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	secretHash, _ := auth.HashSecret("test-secret")
	s.SetConfig(store.ConfigKeySecretHash, secretHash)

	challengeStore := auth.NewChallengeStore(60 * time.Second)
	sessionStore := auth.NewSessionStore(12 * time.Hour)
	hub := realtime.NewHub()
	go hub.Run()

	h := New(Config{
		Store:          s,
		ChallengeStore: challengeStore,
		SessionStore:   sessionStore,
		Hub:            hub,
		BootstrapToken: "test-bootstrap-token",
		SecureCookies:  false,
		AllowedOrigin:  "",
	})

	cleanup := func() {
		hub.Stop()
		challengeStore.Stop()
		sessionStore.Stop()
		s.Close()
	}

	return h, s, cleanup
}

func TestHealthz(t *testing.T) {
	h, _, cleanup := setupTestHandler(t)
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

func TestAdminDevicesEndpoint(t *testing.T) {
	h, s, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("ValidToken", func(t *testing.T) {
		pubJWK, deviceID := generateTestKey(t)
		body, _ := json.Marshal(map[string]interface{}{
			"device_id": deviceID,
			"pub_jwk":   pubJWK,
			"label":     "Test",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "test-bootstrap-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		body := `{"device_id":"test-id-2","pub_jwk":{},"label":"Test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "wrong-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		body := `{"device_id":"test-id-3","pub_jwk":{},"label":"Test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("DuplicateDevice", func(t *testing.T) {
		pubJWK, deviceID := generateTestKey(t)
		jwkJSON, _ := json.Marshal(pubJWK)
		s.AddDevice(&store.Device{
			DeviceID:   deviceID,
			PubJWKJSON: string(jwkJSON),
			CreatedAt:  time.Now().UnixMilli(),
		})

		body, _ := json.Marshal(map[string]interface{}{
			"device_id": deviceID,
			"pub_jwk":   pubJWK,
			"label":     "Dup",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/devices", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Bootstrap", "test-bootstrap-token")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("Expected status 409, got %d", rec.Code)
		}
	})
}

func TestPresenceEndpoint(t *testing.T) {
	h, _, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("WithoutSession", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/presence", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})
}

func TestDeviceChallengeEndpoint(t *testing.T) {
	h, _, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		pubJWK, deviceID := generateTestKey(t)

		body, _ := json.Marshal(map[string]interface{}{
			"device_id": deviceID,
			"pub_jwk":   pubJWK,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp APIResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp.Success {
			t.Error("Expected success: true")
		}

		if resp.Data == nil {
			t.Fatal("Expected data in response")
		}
		data := resp.Data.(map[string]interface{})
		if data["challenge_id"] == nil || data["nonce"] == nil {
			t.Error("Expected challenge_id and nonce in response")
		}
	})

	t.Run("MissingDeviceID", func(t *testing.T) {
		body := `{"pub_jwk":{"kty":"EC"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rec.Code)
		}
	})

	t.Run("InvalidDeviceID", func(t *testing.T) {
		body := `{"device_id":"wrong-id","pub_jwk":{"kty":"EC","crv":"P-256","x":"x","y":"y"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/device/challenge", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rec.Code)
		}
	})
}

func TestAuthSecretEndpoint(t *testing.T) {
	h, _, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("WithoutDeviceTicket", func(t *testing.T) {
		body := `{"secret":"test-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/secret", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("WithDeviceTicket_CorrectSecret", func(t *testing.T) {
		body := `{"secret":"test-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/secret", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: "test-device"})
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
			if c.Name == "session" {
				hasSession = true
				break
			}
		}
		if !hasSession {
			t.Error("Expected session cookie to be set")
		}
	})

	t.Run("WithDeviceTicket_WrongSecret", func(t *testing.T) {
		body := `{"secret":"wrong-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/secret", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "device_ticket", Value: "test-device"})
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
}
