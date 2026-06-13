// Package testutil provides helpers for pkg/oidcjwt tests.
package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type TestIssuer struct {
	*httptest.Server
	mu   sync.Mutex
	keys map[string]*rsa.PrivateKey
}

func NewTestIssuer(t *testing.T, priv *rsa.PrivateKey, kid string) (*TestIssuer, func()) {
	t.Helper()

	ti := &TestIssuer{
		keys: map[string]*rsa.PrivateKey{kid: priv},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                base,
			"jwks_uri":                              base + "/.well-known/jwks.json",
			"authorization_endpoint":                base + "/auth",
			"token_endpoint":                        base + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		ti.mu.Lock()
		defer ti.mu.Unlock()

		jwks := struct {
			Keys []map[string]string `json:"keys"`
		}{}
		for kid, p := range ti.keys {
			pub := p.PublicKey
			jwks.Keys = append(jwks.Keys, map[string]string{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_ = json.NewEncoder(w).Encode(jwks)
	})

	srv := httptest.NewServer(mux)
	ti.Server = srv
	return ti, srv.Close
}

func (ti *TestIssuer) AddKey(t *testing.T, priv *rsa.PrivateKey, kid string) {
	t.Helper()
	ti.mu.Lock()
	defer ti.mu.Unlock()
	ti.keys[kid] = priv
}

func MintTestJWT(t *testing.T, priv *rsa.PrivateKey, kid, iss, aud, sub string, ttl time.Duration, extra map[string]any) string {
	t.Helper()

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": iss,
		"aud": aud,
		"sub": sub,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

func MustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}
