package pkce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

type PKCEInfo struct {
	CodeVerifier, CodeChallenge, Method string
}

const (
	pkceLength          = 128
	pkceMethod          = "S256"
	codeVerifierCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// generateCodeVerifier generates a random code verifier of the specified length
func generateCodeVerifier(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	for i := range b {
		b[i] = codeVerifierCharset[b[i]%byte(len(codeVerifierCharset))]
	}
	return string(b), nil
}

// generateCodeChallengeS256 generates a S256 code challenge from the code verifier
func generateCodeChallengeS256(codeVerifier string) string {
	h := sha256.New()
	h.Write([]byte(codeVerifier))
	hash := h.Sum(nil)
	return base64URLEncode(hash)
}

// base64URLEncode encodes the input bytes to a URL-safe, base64-encoded string
func base64URLEncode(input []byte) string {
	encoded := base64.RawURLEncoding.EncodeToString(input)
	encoded = strings.TrimRight(encoded, "=")
	return encoded
}

func GetPKCE() (PKCEInfo, error) {
	codeVerifier, err := generateCodeVerifier(pkceLength)
	if err != nil {
		return PKCEInfo{}, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	codeChallenge := generateCodeChallengeS256(codeVerifier)
	return PKCEInfo{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		Method:        pkceMethod,
	}, nil
}
