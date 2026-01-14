package auth

import (
	"testing"
)

func TestHashAndVerifySecret(t *testing.T) {
	secret := "my-secure-secret-123"

	hash, err := HashSecret(secret)
	if err != nil {
		t.Fatalf("HashSecret failed: %v", err)
	}

	if hash == "" {
		t.Error("Expected non-empty hash")
	}

	t.Run("VerifyCorrectSecret", func(t *testing.T) {
		if err := VerifySecret(secret, hash); err != nil {
			t.Errorf("VerifySecret failed for correct secret: %v", err)
		}
	})

	t.Run("VerifyWrongSecret", func(t *testing.T) {
		err := VerifySecret("wrong-secret", hash)
		if err == nil {
			t.Error("Expected error for wrong secret")
		}
		if err != ErrInvalidSecret {
			t.Errorf("Expected ErrInvalidSecret, got %v", err)
		}
	})

	t.Run("DifferentHashesForSameSecret", func(t *testing.T) {
		hash2, _ := HashSecret(secret)
		if hash == hash2 {
			t.Error("Expected different hashes due to random salt")
		}

		if err := VerifySecret(secret, hash2); err != nil {
			t.Errorf("Second hash should also verify: %v", err)
		}
	})
}

func TestVerifyInvalidHashFormat(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"not_enough_parts", "$argon2id$v=19"},
		{"wrong_algorithm", "$bcrypt$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA"},
		{"invalid_params", "$argon2id$v=19$invalid$c2FsdA$aGFzaA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySecret("secret", tt.hash)
			if err == nil {
				t.Error("Expected error for invalid hash format")
			}
		})
	}
}
