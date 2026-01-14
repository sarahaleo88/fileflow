package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

var ErrInvalidSecret = errors.New("invalid secret")

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

func HashSecret(secret string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, saltB64, hashB64), nil
}

func VerifySecret(secret, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return fmt.Errorf("%w: invalid hash format", ErrInvalidSecret)
	}

	if parts[1] != "argon2id" {
		return fmt.Errorf("%w: unsupported algorithm", ErrInvalidSecret)
	}

	var memory, time uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return fmt.Errorf("%w: invalid parameters", ErrInvalidSecret)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("%w: invalid salt", ErrInvalidSecret)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return fmt.Errorf("%w: invalid hash", ErrInvalidSecret)
	}

	computedHash := argon2.IDKey([]byte(secret), salt, time, memory, threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(computedHash, expectedHash) != 1 {
		return ErrInvalidSecret
	}

	return nil
}
