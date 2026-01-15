package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenManager_SignAndVerify(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long-1234")
	tm := NewTokenManager(secret)

	sid := "session_123"
	ver := 1
	ttl := 1 * time.Hour

	// 1. Sign
	token, err := tm.Sign(sid, ver, ttl)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if token == "" {
		t.Fatal("Token is empty")
	}
	if !strings.Contains(token, ".") {
		t.Fatal("Token format invalid, missing dot")
	}

	// 2. Verify
	claims, err := tm.Verify(token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if claims.SID != sid {
		t.Errorf("expected SID %q, got %q", sid, claims.SID)
	}
	if claims.Ver != ver {
		t.Errorf("expected Ver %d, got %d", ver, claims.Ver)
	}
	if claims.Exp <= time.Now().Unix() {
		t.Error("claims should not be expired")
	}
}

func TestTokenManager_Expired(t *testing.T) {
	secret := []byte("test-secret")
	tm := NewTokenManager(secret)

	// Negative TTL
	token, err := tm.Sign("sid", TokenVersionSession, -time.Minute)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	_, err = tm.Verify(token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenManager_Tampered(t *testing.T) {
	secret := []byte("test-secret")
	tm := NewTokenManager(secret)

	token, err := tm.Sign("sid", TokenVersionSession, time.Hour)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Tamper: modify the signature (last part)
	parts := strings.Split(token, ".")
	// Append a char to signature
	badToken := parts[0] + "." + parts[1] + "a"

	_, err = tm.Verify(badToken)
	if err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestTokenManager_BadFormat(t *testing.T) {
	secret := []byte("test-secret")
	tm := NewTokenManager(secret)

	badTokens := []string{
		"nodot",
		"part1.part2.part3", // too many
		".",
	}

	for _, bt := range badTokens {
		_, err := tm.Verify(bt)
		if err == nil {
			t.Errorf("expected error for bad format %q, got nil", bt)
		}
	}
}
