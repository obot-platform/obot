package wellknown

import (
	"crypto/ecdsa"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/obot-platform/obot/pkg/api/server"
)

type handler struct {
	baseURL string
	keySet  jwk.Set
}

func SetupHandlers(baseURL string, key *ecdsa.PrivateKey, mux *server.Server) error {
	// Create a new empty JWKS
	jwks := jwk.NewSet()

	// Convert the ECDSA key to a JWK
	jwkKey, err := jwk.Import(key.PublicKey)
	if err != nil {
		return err
	}

	// Set the key ID and other properties
	if err = jwkKey.Set(jwk.KeyIDKey, "obot-key"); err != nil {
		return err
	}
	if err = jwkKey.Set(jwk.AlgorithmKey, "ES256"); err != nil {
		return err
	}
	if err = jwkKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return err
	}

	// Add the key to the JWKS
	if err = jwks.AddKey(jwkKey); err != nil {
		return err
	}

	h := &handler{
		baseURL: baseURL,
		keySet:  jwks,
	}

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.oauthAuthorization)
	mux.HandleFunc("GET /.well-known/jwks.json", h.jwks)

	return nil
}
