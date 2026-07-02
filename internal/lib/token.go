package lib

import (
	"crypto/rand"
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

// RandomHex returns a random lowercase hex string of the given length. Hex is
// safe for Milvus collection names (only [0-9a-f]), unlike base64url.
func RandomHex(nChars int) string {
	b := make([]byte, (nChars+1)/2)
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:nChars]
}
