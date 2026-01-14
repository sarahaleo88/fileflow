package auth

import (
	"testing"
	"time"
)

func TestChallengeStore(t *testing.T) {
	store := NewChallengeStore(60 * time.Second)
	defer store.Stop()

	t.Run("GenerateAndGet", func(t *testing.T) {
		pubJWK := map[string]interface{}{
			"kty": "EC",
			"crv": "P-256",
			"x":   "test-x",
			"y":   "test-y",
		}

		challenge, err := store.Generate("device-1", pubJWK)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		if challenge.ID == "" {
			t.Error("Expected non-empty challenge ID")
		}
		if len(challenge.Nonce) != 32 {
			t.Errorf("Expected 32-byte nonce, got %d bytes", len(challenge.Nonce))
		}
		if challenge.DeviceID != "device-1" {
			t.Errorf("DeviceID = %q, want %q", challenge.DeviceID, "device-1")
		}

		got, ok := store.Get(challenge.ID)
		if !ok {
			t.Fatal("Expected to find challenge")
		}
		if got.ID != challenge.ID {
			t.Errorf("ID = %q, want %q", got.ID, challenge.ID)
		}
	})

	t.Run("GetNonexistent", func(t *testing.T) {
		_, ok := store.Get("nonexistent")
		if ok {
			t.Error("Expected challenge to not be found")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		challenge, _ := store.Generate("device-2", nil)
		store.Delete(challenge.ID)

		_, ok := store.Get(challenge.ID)
		if ok {
			t.Error("Expected challenge to be deleted")
		}
	})

	t.Run("Expiration", func(t *testing.T) {
		shortStore := NewChallengeStore(1 * time.Millisecond)
		defer shortStore.Stop()

		challenge, _ := shortStore.Generate("device-3", nil)
		time.Sleep(10 * time.Millisecond)

		_, ok := shortStore.Get(challenge.ID)
		if ok {
			t.Error("Expected challenge to be expired")
		}
	})
}

func TestNonceBase64(t *testing.T) {
	store := NewChallengeStore(60 * time.Second)
	defer store.Stop()

	challenge, _ := store.Generate("device", nil)
	b64 := challenge.NonceBase64()

	if b64 == "" {
		t.Error("Expected non-empty base64 string")
	}
	if len(b64) != 43 {
		t.Errorf("Expected 43-char base64url (32 bytes), got %d chars", len(b64))
	}
}
