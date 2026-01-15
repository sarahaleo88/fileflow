package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run hash_secret.go <your-password>")
		os.Exit(1)
	}

	secret := os.Args[1]

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating salt: %v\n", err)
		os.Exit(1)
	}

	hash := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, saltB64, hashB64)

	fmt.Println(encoded)
}
