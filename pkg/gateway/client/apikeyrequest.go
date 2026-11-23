package client

import (
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/auth"
)

const (
	// HeaderObotAPIKey carries an Obot API key alongside another credential, on LLM proxy requests. It is one of the
	// credential headers that exempt a request from the cross-origin check (auth.CredentialHeaders).
	HeaderObotAPIKey = auth.HeaderObotAPIKey

	// llmProxyPathPrefix begins the paths of the LLM proxy, the only routes that read HeaderObotAPIKey.
	llmProxyPathPrefix = "/api/llm-proxy/"
)

// APIKeyHeaders returns the values of the headers a request can present an API key in, in the order they are read:
// X-Obot-Api-Key, on the LLM proxy only, then the Authorization header's bearer value, then X-API-Key. An empty value
// is a header that is not set.
func APIKeyHeaders(req *http.Request) []string {
	// X-Obot-Api-Key lets a client whose Authorization header carries another credential,
	// such as Claude Code signed in with a Claude subscription, send its Obot API key too. It
	// is only for the LLM proxy, which strips it before forwarding; elsewhere, such as on MCP
	// connections, it would travel on to an upstream server.
	var obotAPIKey string
	if strings.HasPrefix(req.URL.Path, llmProxyPathPrefix) {
		obotAPIKey = req.Header.Get(HeaderObotAPIKey)
	}
	return []string{
		obotAPIKey,
		strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "),
		req.Header.Get("X-API-Key"),
	}
}

// RequestAPIKey returns the credential the API key authenticator reads from a request: the first of APIKeyHeaders that
// is set. It need not be an API key. Code that must know that a request was authenticated with an API key, rather than
// with another credential of the same principal, parses it (ParseAPIKey).
func RequestAPIKey(req *http.Request) string {
	for _, value := range APIKeyHeaders(req) {
		if value != "" {
			return value
		}
	}
	return ""
}
