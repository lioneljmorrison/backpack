// Package auth issues bearer tokens to actors (a person or an agent) and
// checks them on every request. Only a token's SHA-256 hash is stored.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
)

const tokenPrefix = "bp_"

// NewToken returns a fresh token: the bp_ prefix and 32 random bytes,
// base64url-encoded.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash is the form in which a token is stored and looked up.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

var actorName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidActorName accepts 1–64 letters, digits, dots, dashes and underscores.
func ValidActorName(name string) error {
	if !actorName.MatchString(name) {
		return errors.New("actor name must be 1-64 letters, digits, '.', '-' or '_'")
	}
	return nil
}
