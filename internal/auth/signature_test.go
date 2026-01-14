package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestParseJWKPublicKey(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKey := privKey.PublicKey

	xBytes := pubKey.X.Bytes()
	yBytes := pubKey.Y.Bytes()

	xPadded := make([]byte, 32)
	yPadded := make([]byte, 32)
	copy(xPadded[32-len(xBytes):], xBytes)
	copy(yPadded[32-len(yBytes):], yBytes)

	jwk := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(xPadded),
		"y":   base64.RawURLEncoding.EncodeToString(yPadded),
	}

	parsed, err := ParseJWKPublicKey(jwk)
	if err != nil {
		t.Fatalf("ParseJWKPublicKey failed: %v", err)
	}

	if parsed.X.Cmp(pubKey.X) != 0 || parsed.Y.Cmp(pubKey.Y) != 0 {
		t.Error("Parsed key does not match original")
	}
}

func TestParseJWKPublicKeyErrors(t *testing.T) {
	tests := []struct {
		name string
		jwk  map[string]interface{}
	}{
		{"wrong_kty", map[string]interface{}{"kty": "RSA", "crv": "P-256", "x": "test", "y": "test"}},
		{"wrong_crv", map[string]interface{}{"kty": "EC", "crv": "P-384", "x": "test", "y": "test"}},
		{"invalid_x", map[string]interface{}{"kty": "EC", "crv": "P-256", "x": "!!!", "y": "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseJWKPublicKey(tt.jwk)
			if err == nil {
				t.Error("Expected error")
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKey := &privKey.PublicKey

	nonce := make([]byte, 32)
	rand.Read(nonce)

	hash := sha256.Sum256(nonce)
	r, s, _ := ecdsa.Sign(rand.Reader, privKey, hash[:])

	rBytes := r.Bytes()
	sBytes := s.Bytes()
	rPadded := make([]byte, 32)
	sPadded := make([]byte, 32)
	copy(rPadded[32-len(rBytes):], rBytes)
	copy(sPadded[32-len(sBytes):], sBytes)

	sig := append(rPadded, sPadded...)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	if err := VerifySignature(pubKey, nonce, sigB64); err != nil {
		t.Errorf("VerifySignature failed for valid signature: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKey := &privKey.PublicKey

	nonce := make([]byte, 32)
	rand.Read(nonce)

	wrongSig := make([]byte, 64)
	rand.Read(wrongSig)
	wrongSigB64 := base64.RawURLEncoding.EncodeToString(wrongSig)

	if err := VerifySignature(pubKey, nonce, wrongSigB64); err == nil {
		t.Error("Expected error for invalid signature")
	}
}

func TestComputeDeviceID(t *testing.T) {
	jwk := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"x":   "test-x-value",
		"y":   "test-y-value",
	}

	id1, err := ComputeDeviceID(jwk)
	if err != nil {
		t.Fatalf("ComputeDeviceID failed: %v", err)
	}

	id2, _ := ComputeDeviceID(jwk)
	if id1 != id2 {
		t.Error("Expected same device ID for same JWK")
	}

	jwk2 := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"x":   "different-x",
		"y":   "test-y-value",
	}
	id3, _ := ComputeDeviceID(jwk2)
	if id1 == id3 {
		t.Error("Expected different device ID for different JWK")
	}
}

func TestValidateDeviceID(t *testing.T) {
	jwk := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"x":   "test-x",
		"y":   "test-y",
	}

	correctID, _ := ComputeDeviceID(jwk)

	if err := ValidateDeviceID(correctID, jwk); err != nil {
		t.Errorf("ValidateDeviceID failed for correct ID: %v", err)
	}

	if err := ValidateDeviceID("wrong-id", jwk); err == nil {
		t.Error("Expected error for wrong device ID")
	}
}
