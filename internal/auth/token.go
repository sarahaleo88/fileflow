package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidFormat    = errors.New("invalid token format")
)

type Claims struct {
	Ver int    `json:"v"`
	SID string `json:"sid"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret []byte) *TokenManager {
	return &TokenManager{secret: secret}
}

func (tm *TokenManager) Sign(sid string, version int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		Ver: version,
		SID: sid,
		Iat: now.Unix(),
		Exp: now.Add(ttl).Unix(),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := tm.computeHMAC(encodedPayload)
	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return fmt.Sprintf("%s.%s", encodedPayload, encodedSignature), nil
}

func (tm *TokenManager) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	encodedPayload := parts[0]
	encodedSignature := parts[1]

	// 1. Verify Signature
	expectedSignature := tm.computeHMAC(encodedPayload)
	actualSignature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil {
		return nil, ErrInvalidSignature
	}

	if subtle.ConstantTimeCompare(expectedSignature, actualSignature) != 1 {
		return nil, ErrInvalidSignature
	}

	// 2. Decode Payload
	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	// 3. Check Expiry
	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func (tm *TokenManager) computeHMAC(data string) []byte {
	h := hmac.New(sha256.New, tm.secret)
	h.Write([]byte(data))
	return h.Sum(nil)
}
