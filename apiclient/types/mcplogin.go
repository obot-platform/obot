package types

// MCPLocalLogin describes a pending UI OAuth attempt. All credentials and the
// PKCE verifier remain on the server; the CLI only relays browser callbacks.
type MCPLocalLogin struct {
	AuthorizationURL string `json:"authorizationURL"`
	RedirectURL      string `json:"redirectURL"`
	State            string `json:"state"`
}
