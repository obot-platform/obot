package tunnel

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	credentialVersion     = 1
	credentialTokenSize   = 64
	credentialPreviewSize = 16
)

type credentialRecord struct {
	Version int    `json:"version"`
	Digest  string `json:"digest"`
	Preview string `json:"preview"`
}

type credentialMatcher struct {
	digest       [sha256.Size]byte
	credentialID string
}

// NewCredential creates a 64-byte bearer token and the non-reversible value
// stored on an MCPTunnel.
func NewCredential() (token, credentialValue string, err error) {
	rawToken := make([]byte, credentialTokenSize)
	if _, err := rand.Read(rawToken); err != nil {
		return "", "", fmt.Errorf("failed to generate tunnel token: %w", err)
	}

	token = base64.RawURLEncoding.EncodeToString(rawToken)
	digest := sha256.Sum256([]byte(token))
	record := credentialRecord{
		Version: credentialVersion,
		Digest:  base64.RawURLEncoding.EncodeToString(digest[:]),
		Preview: base64.RawURLEncoding.EncodeToString(rawToken[:credentialPreviewSize]),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode tunnel credential: %w", err)
	}
	return token, string(encoded), nil
}

// CredentialPreview returns the 16-byte preview stored with a credential.
func CredentialPreview(credentialValue string) (string, error) {
	record, err := parseCredential(credentialValue)
	if err != nil {
		return "", err
	}
	return record.Preview + "...", nil
}

// CredentialID returns the non-secret identifier used to narrow credential
// lookups before the full digest is verified.
func CredentialID(credentialValue string) (string, error) {
	record, err := parseCredential(credentialValue)
	if err != nil {
		return "", err
	}
	preview, err := base64.RawURLEncoding.DecodeString(record.Preview)
	if err != nil {
		return "", errors.New("stored tunnel token is invalid")
	}
	return hex.EncodeToString(preview), nil
}

// CredentialMatches reports whether token matches a stored credential digest.
func CredentialMatches(credentialValue, token string) bool {
	matcher, ok := newCredentialMatcher(token)
	return ok && matcher.Matches(credentialValue)
}

func newCredentialMatcher(token string) (credentialMatcher, bool) {
	if len(token) != base64.RawURLEncoding.EncodedLen(credentialTokenSize) {
		return credentialMatcher{}, false
	}
	rawToken, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(rawToken) != credentialTokenSize {
		return credentialMatcher{}, false
	}
	return credentialMatcher{
		digest:       sha256.Sum256([]byte(token)),
		credentialID: hex.EncodeToString(rawToken[:credentialPreviewSize]),
	}, true
}

func (m credentialMatcher) Matches(credentialValue string) bool {
	record, err := parseCredential(credentialValue)
	if err != nil {
		return false
	}
	storedDigest, err := base64.RawURLEncoding.DecodeString(record.Digest)
	if err != nil || len(storedDigest) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(storedDigest, m.digest[:]) == 1
}

func parseCredential(credentialValue string) (credentialRecord, error) {
	var record credentialRecord
	if err := json.Unmarshal([]byte(credentialValue), &record); err != nil {
		return credentialRecord{}, errors.New("stored tunnel token is invalid")
	}
	if record.Version != credentialVersion {
		return credentialRecord{}, errors.New("stored tunnel token is invalid")
	}
	digest, digestErr := base64.RawURLEncoding.DecodeString(record.Digest)
	preview, previewErr := base64.RawURLEncoding.DecodeString(record.Preview)
	if digestErr != nil || len(digest) != sha256.Size || previewErr != nil || len(preview) != credentialPreviewSize {
		return credentialRecord{}, errors.New("stored tunnel token is invalid")
	}
	return record, nil
}
