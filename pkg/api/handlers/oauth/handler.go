package oauth

import (
	"crypto/ecdsa"
	"strings"

	"github.com/obot-platform/obot/pkg/api/server"
)

type handler struct {
	baseURL, issuer string
	key             *ecdsa.PrivateKey
}

func SetupHandlers(baseURL string, key *ecdsa.PrivateKey, mux *server.Server) {
	h := &handler{
		baseURL: baseURL,
		issuer:  strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://"),
		key:     key,
	}

	mux.HandleFunc("POST /oauth/register", h.register)
	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)
	mux.HandleFunc("GET /oauth/authorize", h.authorize)
	mux.HandleFunc("POST /oauth/token", h.token)
}
