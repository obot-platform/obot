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
}

type AuthProviderList List[AuthProvider]
