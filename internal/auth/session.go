package auth

// Deprecated: This file contains legacy session logic that is replaced by stateless tokens (token.go).
// It will be removed in future versions.

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	ID        string
	DeviceID  string
	ExpiresAt time.Time
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	stopCh   chan struct{}
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	ss := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}
	go ss.cleanupLoop()
	return ss
}

func (ss *SessionStore) Stop() {
	close(ss.stopCh)
}

func (ss *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ss.cleanup()
		case <-ss.stopCh:
			return
		}
	}
}

func (ss *SessionStore) cleanup() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	now := time.Now()
	for id, s := range ss.sessions {
		if now.After(s.ExpiresAt) {
			delete(ss.sessions, id)
		}
	}
}

func (ss *SessionStore) Create(deviceID string) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}

	s := &Session{
		ID:        base64.RawURLEncoding.EncodeToString(tokenBytes),
		DeviceID:  deviceID,
		ExpiresAt: time.Now().Add(ss.ttl),
	}

	ss.mu.Lock()
	ss.sessions[s.ID] = s
	ss.mu.Unlock()

	return s, nil
}

func (ss *SessionStore) Get(sessionID string) (*Session, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	s, ok := ss.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, ErrSessionNotFound
	}
	return s, nil
}

func (ss *SessionStore) Delete(sessionID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.sessions, sessionID)
}

func SetSessionCookie(w http.ResponseWriter, session *Session, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func SetDeviceTicketCookie(w http.ResponseWriter, ticket string, ttl time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "device_ticket",
		Value:    ticket,
		Path:     "/",
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func GetSessionFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func GetDeviceTicketFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("device_ticket")
	if err != nil {
		return ""
	}
	return cookie.Value
}
