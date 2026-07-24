package lib

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func GenerateSecureToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of an opaque token. Used to store
// refresh tokens: they are high-entropy random strings, so a fast digest is
// enough (bcrypt would prevent the indexed hash lookup we need). ponytail:
// SHA-256, not bcrypt — correct trade-off for random tokens, not passwords.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RandomHex returns a random lowercase hex string of the given length. Hex is
// safe for Milvus collection names (only [0-9a-f]), unlike base64url.
func RandomHex(nChars int) string {
	b := make([]byte, (nChars+1)/2)
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:nChars]
}
