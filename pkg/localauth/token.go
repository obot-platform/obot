package localauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// tokenSigner mints the short-lived access tokens the gateway hands back to the provider when it
// asks for user info. They are signed rather than looked up so that user info can be served
// without touching the database. The key is per-process: tokens don't outlive a restart, and the
// gateway gets a fresh one with every session state response.
type tokenSigner struct {
	key []byte
}

func newTokenSigner() (*tokenSigner, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate token signing key: %w", err)
	}

	return &tokenSigner{key: key}, nil
}

func (t *tokenSigner) sign(email string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(email))
	return payload + "." + base64.RawURLEncoding.EncodeToString(t.mac(payload))
}

func (t *tokenSigner) verify(token string) (string, error) {
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return "", errors.New("malformed token")
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return "", errors.New("malformed token signature")
	}

	if !hmac.Equal(decodedSignature, t.mac(payload)) {
		return "", errors.New("invalid token signature")
	}

	email, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("malformed token payload")
	}

	return string(email), nil
}

func (t *tokenSigner) mac(payload string) []byte {
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
