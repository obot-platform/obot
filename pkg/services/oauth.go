package services

// OAuthAuthorizationServerConfig represents the response from /.well-known/oauth-authorization-server
// as defined in RFC 8414 (OAuth 2.0 Authorization Server Metadata)
type OAuthAuthorizationServerConfig struct {
	// Issuer is the authorization server's issuer identifier, which is a URL that uses the "https" scheme
	// and has no query or fragment components. REQUIRED.
	Issuer string `json:"issuer"`
	// AuthorizationEndpoint is the URL of the authorization server's authorization endpoint.
	// REQUIRED unless no grant types are supported that use the authorization endpoint.
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// TokenEndpoint is the URL of the authorization server's token endpoint.
	// REQUIRED unless only the implicit grant type is supported.
	TokenEndpoint string `json:"token_endpoint"`
	// JWKSURI is the URL of the authorization server's JWK Set document.
	// OPTIONAL.
	JWKSURI string `json:"jwks_uri,omitempty"`
	// RegistrationEndpoint is the URL of the authorization server's OAuth 2.0 Dynamic Client Registration endpoint.
	// OPTIONAL.
	RegistrationEndpoint string `json:"registration_endpoint,omitempty"`
	// ScopesSupported is a JSON array containing a list of the OAuth 2.0 scope values that this authorization server supports.
	// RECOMMENDED.
	ScopesSupported []string `json:"scopes_supported,omitempty"`
	// ResponseTypesSupported is a JSON array containing a list of the OAuth 2.0 response_type values that this authorization server supports.
	// REQUIRED.
	ResponseTypesSupported []string `json:"response_types_supported"`
	// ResponseModesSupported is a JSON array containing a list of the OAuth 2.0 response_mode values that this authorization server supports.
	// OPTIONAL. Default is ["query", "fragment"].
	ResponseModesSupported []string `json:"response_modes_supported,omitempty"`
	// GrantTypesSupported is a JSON array containing a list of the OAuth 2.0 grant type values that this authorization server supports.
	// OPTIONAL. Default is ["authorization_code", "implicit"].
	GrantTypesSupported []string `json:"grant_types_supported,omitempty"`
	// TokenEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this token endpoint.
	// OPTIONAL. Default is "client_secret_basic".
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	// TokenEndpointAuthSigningAlgValuesSupported is a JSON array containing a list of the JWS signing algorithms supported by the token endpoint.
	// OPTIONAL. Required if "private_key_jwt" or "client_secret_jwt" authentication methods are specified.
	TokenEndpointAuthSigningAlgValuesSupported []string `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`
	// ServiceDocumentation is the URL of a page containing human-readable information that developers might want or need to know.
	// OPTIONAL.
	ServiceDocumentation string `json:"service_documentation,omitempty"`
	// UILocalesSupported is a JSON array of language tag values from BCP 47 for the user interface.
	// OPTIONAL.
	UILocalesSupported []string `json:"ui_locales_supported,omitempty"`
	// OPPolicyURI is the URL that the authorization server provides to read about the authorization server's requirements.
	// OPTIONAL.
	OPPolicyURI string `json:"op_policy_uri,omitempty"`
	// OPTosURI is the URL that the authorization server provides to read about the authorization server's terms of service.
	// OPTIONAL.
	OPTosURI string `json:"op_tos_uri,omitempty"`
	// RevocationEndpoint is the URL of the authorization server's OAuth 2.0 revocation endpoint.
	// OPTIONAL.
	RevocationEndpoint string `json:"revocation_endpoint,omitempty"`
	// RevocationEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this revocation endpoint.
	// OPTIONAL. Default is "client_secret_basic".
	RevocationEndpointAuthMethodsSupported []string `json:"revocation_endpoint_auth_methods_supported,omitempty"`
	// IntrospectionEndpoint is the URL of the authorization server's OAuth 2.0 introspection endpoint.
	// OPTIONAL.
	IntrospectionEndpoint string `json:"introspection_endpoint,omitempty"`
	// IntrospectionEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this introspection endpoint.
	// OPTIONAL.
	IntrospectionEndpointAuthMethodsSupported []string `json:"introspection_endpoint_auth_methods_supported,omitempty"`
	// IntrospectionEndpointAuthSigningAlgValuesSupported is a JSON array containing a list of the JWS signing algorithms supported by the introspection endpoint.
	// OPTIONAL. Required if "private_key_jwt" or "client_secret_jwt" authentication methods are specified.
	IntrospectionEndpointAuthSigningAlgValuesSupported []string `json:"introspection_endpoint_auth_signing_alg_values_supported,omitempty"`
	// CodeChallengeMethodsSupported is a JSON array containing a list of PKCE code challenge methods supported by this authorization server.
	// OPTIONAL. If omitted, the authorization server does not support PKCE.
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`
}
