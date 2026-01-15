package auth

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
)

func VerifyECDSASignature(pub *ecdsa.PublicKey, message, signature []byte) bool {
	if pub == nil || len(signature) == 0 {
		return false
	}

	h := sha256.Sum256(message)
	if len(signature) == 64 {
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:])
		return ecdsa.Verify(pub, h[:], r, s)
	}

	return ecdsa.VerifyASN1(pub, h[:], signature)
}
