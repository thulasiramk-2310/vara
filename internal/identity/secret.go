package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// Session and API-token secrets (RFC-0020 §6.2). Unlike passwords these are
// high-entropy (256 bits from a CSPRNG), so they need no KDF: the store holds
// only a SHA-256 hash, and lookup hashes the presented secret and compares. The
// plaintext is returned to the caller exactly once at creation and is never
// stored, logged, or recoverable (S4).

// NewSecret mints a fresh credential secret, returning the plaintext (shown once)
// and the hash that is stored.
func NewSecret() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	return plaintext, HashSecret(plaintext), nil
}

// HashSecret returns the stored form of a bearer secret: hex(SHA-256). A fast
// hash is correct here precisely because the input is high-entropy.
func HashSecret(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
