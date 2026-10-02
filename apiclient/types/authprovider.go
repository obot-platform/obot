package types

type AuthProvider struct {
	Metadata
	AuthProviderManifest
	AuthProviderStatus
}

type AuthProviderManifest struct {
	CommonProviderMetadata `json:",inline" yaml:",inline"`
	PostgresTablePrefix    string `json:"postgresTablePrefix,omitempty"`
	GroupIDPrefix          string `json:"groupIDPrefix,omitempty" yaml:"groupIDPrefix,omitempty"`
}

type AuthProviderStatus struct {
	CommonProviderStatus
	Namespace string `json:"namespace,omitempty"`
	// Staged means this provider's settings are saved as a replacement while another provider
	// still serves logins.
	Staged bool `json:"staged,omitempty"`
	// VerifiedEmail is the address that signed in through this provider to prove the staged
	// settings work, and that will hold Owner once the switch completes. It is set only while the
	// provider is staged.
	VerifiedEmail string `json:"verifiedEmail,omitempty"`
	// RequiresActivation means a provisioned initial owner has not yet opened their setup link and
	// set a password, so nobody can sign in through this provider yet.
	RequiresActivation bool `json:"requiresActivation,omitempty"`
	// SCIMState is the state of the provider's SCIM connection: "connected", "enforced", or empty when
	// the provider has none. Only administrators see it.
	SCIMState string `json:"scimState,omitempty"`
	// SCIMTokenExpiresAt is when the bearer token of the provider's SCIM connection stops being accepted, and is
	// unset while the connection has no token. Only administrators see it.
	SCIMTokenExpiresAt *Time `json:"scimTokenExpiresAt,omitempty"`
	// SCIM describes how the provider supports SCIM. It is set only for a provider that supports SCIM, and only
	// administrators see it.
	SCIM *AuthProviderSCIM `json:"scim,omitempty"`
}

// AuthProviderSCIM describes how an auth provider supports SCIM provisioning.
type AuthProviderSCIM struct {
	// DirectoryParameters name the configuration parameters that only login-time directory synchronization uses.
	// While the provider is configured or staged without a SCIM connection, providing them sets up directory
	// synchronization, and omitting them sets up SCIM.
	DirectoryParameters []string `json:"directoryParameters"`
	// IssuerParameter names the configuration parameter that holds the identity provider's issuer URL.
	IssuerParameter string `json:"issuerParameter"`
	// ConnectionIssuer is the issuer URL recorded when the provider's SCIM connection was created, and empty while
	// it has none. SCIM bindings belong to the identity provider organization they were created in.
	ConnectionIssuer string `json:"connectionIssuer,omitempty"`
}

type AuthProviderList List[AuthProvider]
