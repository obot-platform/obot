package types

type OAuthClientManifest struct {
	// RedirectURI is a single redirection URI string
	// Maintained for backward compatibility
	//
	// Deprecated: use RedirectURIs instead
	RedirectURI string `json:"redirect_uri,omitempty"`

	// RedirectURIs is an array of redirection URI strings for use in redirect-based flows
	// such as the authorization code and implicit flows. As required by Section 2 of OAuth 2.0 [RFC6749],
	// clients using flows with redirection MUST register their redirection URI values.
	// Required for redirect-based flows.
	RedirectURIs []string `json:"redirect_uris,omitempty"`

	// TokenEndpointAuthMethod is a string indicator of the requested authentication method for the token endpoint.
	// Values defined include: "none", "client_secret_post", "client_secret_basic".
	// If unspecified or omitted, the default is "client_secret_basic".
	// Optional.
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`

	// GrantTypes is an array of OAuth 2.0 grant type strings that the client can use at the token endpoint.
	// If omitted, the default behavior is that the client will use only the "authorization_code" Grant Type.
	// Optional.
	GrantTypes []string `json:"grant_types,omitempty"`

	// ResponseTypes is an array of the OAuth 2.0 response type strings that the client can use at the authorization endpoint.
	// If omitted, the default is that the client will use only the "code" response type.
	// Optional.
	ResponseTypes []string `json:"response_types,omitempty"`

	// ClientName is a human-readable string name of the client to be presented to the end-user during authorization.
	// If omitted, the authorization server MAY display the raw "client_id" value to the end-user instead.
	// It is RECOMMENDED that clients always send this field.
	// Optional.
	ClientName string `json:"client_name,omitempty"`

	// ClientURI is a URL string of a web page providing information about the client.
	// If present, the server SHOULD display this URL to the end-user in a clickable fashion.
	// It is RECOMMENDED that clients always send this field.
	// Optional.
	ClientURI string `json:"client_uri,omitempty"`

	// LogoURI is a URL string that references a logo for the client.
	// If present, the server SHOULD display this image to the end-user during approval.
	// Optional.
	LogoURI string `json:"logo_uri,omitempty"`

	// Scope is a string containing a space-separated list of scope values that the client can use when requesting access tokens.
	// If omitted, an authorization server MAY register a client with a default set of scopes.
	// Optional.
	Scope string `json:"scope,omitempty"`

	// Contacts is an array of strings representing ways to contact people responsible for this client, typically email addresses.
	// Optional.
	Contacts []string `json:"contacts,omitempty"`

	// TOSURI is a URL string that points to a human-readable terms of service document for the client.
	// Optional.
	TOSURI string `json:"tos_uri,omitempty"`

	// PolicyURI is a URL string that points to a human-readable privacy policy document.
	// Optional.
	PolicyURI string `json:"policy_uri,omitempty"`

	// JWKSURI is a URL string referencing the client's JSON Web Key (JWK) Set document, which contains the client's public keys.
	// The "jwks_uri" and "jwks" parameters MUST NOT both be present in the same request or response.
	// Optional.
	JWKSURI string `json:"jwks_uri,omitempty"`

	// JWKS is the client's JSON Web Key Set document value, which contains the client's public keys.
	// This parameter is intended to be used by clients that cannot use the "jwks_uri" parameter.
	// The "jwks_uri" and "jwks" parameters MUST NOT both be present in the same request or response.
	// Optional.
	JWKS string `json:"jwks,omitempty"`

	// SoftwareID is a unique identifier string assigned by the client developer or software publisher
	// used by registration endpoints to identify the client software to be dynamically registered.
	// Optional.
	SoftwareID string `json:"software_id,omitempty"`

	// SoftwareVersion is a version identifier string for the client software identified by "software_id".
	// Optional.
	SoftwareVersion string `json:"software_version,omitempty"`
}

type OAuthClient struct {
	Metadata
	OAuthClientManifest
	RegistrationAccessToken    string `json:"registration_access_token,omitempty"`
	RegistrationTokenIssuedAt  int64  `json:"registration_token_issued_at"`
	RegistrationTokenExpiresAt int64  `json:"registration_token_expires_at"`
	RegistrationClientURI      string `json:"registration_client_uri"`
	ClientID                   string `json:"client_id"`
	ClientSecret               string `json:"client_secret,omitempty"`
	ClientSecretIssuedAt       int64  `json:"client_secret_issued_at"`
	ClientSecretExpiresAt      int64  `json:"client_secret_expires_at"`
}

type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}
