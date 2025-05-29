package oauth

import (
	"crypto/ecdsa"

	"github.com/obot-platform/obot/pkg/api/server"
)

type handler struct {
	baseURL string
	key     *ecdsa.PrivateKey
}

func SetupHandlers(baseURL string, key *ecdsa.PrivateKey, mux *server.Server) {
	h := &handler{
		baseURL: baseURL,
		key:     key,
	}

	mux.HandleFunc("POST /oauth/register", h.register)
	mux.HandleFunc("GET /oauth/register/{namespace}/{name}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{namespace}/{name}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{namespace}/{name}", h.deleteClient)
	mux.HandleFunc("POST /oauth/authorize", nil)
	mux.HandleFunc("POST /oauth/token", nil)
	mux.HandleFunc("POST /oauth/revoke", nil)
	mux.HandleFunc("POST /oauth/introspect", nil)
}
