package main

import "testing"

func TestResolveSessionKey(t *testing.T) {
	t.Run("DevAllowsDefault", func(t *testing.T) {
		t.Setenv("APP_ENV", "dev")
		t.Setenv("SESSION_KEY", "")
		key, err := resolveSessionKey(false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "dev-session-key" {
			t.Fatalf("expected dev-session-key, got %q", key)
		}
	})

	t.Run("DevSecureCookiesRequiresKey", func(t *testing.T) {
		t.Setenv("APP_ENV", "dev")
		t.Setenv("SESSION_KEY", "")
		if _, err := resolveSessionKey(true); err == nil {
			t.Fatal("expected error for missing SESSION_KEY")
		}
	})

	t.Run("ProdRequiresKey", func(t *testing.T) {
		t.Setenv("APP_ENV", "prod")
		t.Setenv("SESSION_KEY", "")
		if _, err := resolveSessionKey(false); err == nil {
			t.Fatal("expected error for missing SESSION_KEY")
		}
	})

	t.Run("CustomKey", func(t *testing.T) {
		t.Setenv("APP_ENV", "prod")
		t.Setenv("SESSION_KEY", "custom-key")
		key, err := resolveSessionKey(true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "custom-key" {
			t.Fatalf("expected custom-key, got %q", key)
		}
	})

	t.Run("DevSessionKeyLiteralRejectedInProd", func(t *testing.T) {
		t.Setenv("APP_ENV", "prod")
		t.Setenv("SESSION_KEY", "dev-session-key")
		if _, err := resolveSessionKey(false); err == nil {
			t.Fatal("expected error for dev-session-key in prod")
		}
	})
}
