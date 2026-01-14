package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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
	loginLimiter := limit.NewIPLimiter(rate.Every(time.Second), 5)
	hub := realtime.NewHub()
	go hub.Run()

	h := New(Config{
		Store:         s,
		TokenManager:  tokenManager,
		LoginLimiter:  loginLimiter,
		SecretHash:    secretHash,
		Hub:           hub,
		SecureCookies: false,
		AllowedOrigin: "",
	})

	cleanup := func() {
		hub.Stop()
		s.Close()
	}

	return h, cleanup
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
		body := `{"secret":"test-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
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
		body := `{"secret":"wrong-secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
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

	t.Run("InvalidMethod", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", rec.Code)
		}
	})
}

func TestSessionEndpoint(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	sid := "test-sid"
	validToken, _ := h.tokenManager.Sign(sid, 1, time.Hour)

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
		validToken, _ := h.tokenManager.Sign(sid, 1, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/api/presence", nil)
		req.AddCookie(&http.Cookie{Name: "ff_session", Value: validToken})
		rec := httptest.NewRecorder()

		h.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}
	})
}
