package types

// MCPLocalLogin describes a pending UI OAuth attempt. All credentials and the
// PKCE verifier remain on the server; the CLI only relays browser callbacks.
type MCPLocalLogin struct {
	AuthorizationURL string `json:"authorizationURL"`
	RedirectURL      string `json:"redirectURL"`
	State            string `json:"state"`
	// ExpiresIn is the remaining lifetime in seconds, computed by the server.
	ExpiresIn int `json:"expiresIn"`
}
