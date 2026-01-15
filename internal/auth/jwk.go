package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// ECPublicJWK represents the public portion of an EC JWK (P-256).
type ECPublicJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

var ErrInvalidJWK = errors.New("invalid public key")

func ParseECPublicJWKMap(m map[string]interface{}) (*ecdsa.PublicKey, *ECPublicJWK, error) {
	if m == nil {
		return nil, nil, ErrInvalidJWK
	}

	b, err := json.Marshal(m)
	if err != nil {
		return nil, nil, ErrInvalidJWK
	}
	return ParseECPublicJWKBytes(b)
}

func ParseECPublicJWKBytes(b []byte) (*ecdsa.PublicKey, *ECPublicJWK, error) {
	var jwk ECPublicJWK
	if err := json.Unmarshal(b, &jwk); err != nil {
		return nil, nil, ErrInvalidJWK
	}
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return nil, nil, ErrInvalidJWK
	}
	if jwk.X == "" || jwk.Y == "" {
		return nil, nil, ErrInvalidJWK
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, nil, ErrInvalidJWK
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, nil, ErrInvalidJWK
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, nil, ErrInvalidJWK
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, &jwk, nil
}

func EqualECPublicJWK(a, b *ECPublicJWK) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Kty == b.Kty && a.Crv == b.Crv && a.X == b.X && a.Y == b.Y
}

func DeviceIDFromJWK(jwk *ECPublicJWK) (string, error) {
	if jwk == nil {
		return "", ErrInvalidJWK
	}
	canonical := struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}{
		Kty: jwk.Kty,
		Crv: jwk.Crv,
		X:   jwk.X,
		Y:   jwk.Y,
	}
	b, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal jwk: %w", err)
	}
	h := sha256.Sum256(b)
	encoded := base64.RawURLEncoding.EncodeToString(h[:])
	return encoded, nil
}
