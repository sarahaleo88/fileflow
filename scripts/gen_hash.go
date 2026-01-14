package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

func main() {
	secret := "test123"
	if len(os.Args) > 1 {
		secret = os.Args[1]
	}

	salt := make([]byte, 16)
	rand.Read(salt)

	hash := argon2.IDKey([]byte(secret), salt, 1, 64*1024, 4, 32)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	fmt.Printf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s\n", 64*1024, 1, 4, saltB64, hashB64)
}
