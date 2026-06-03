package types

type AuthProvider struct {
	Metadata
	AuthProviderManifest
	AuthProviderStatus
}

type AuthProviderManifest struct {
	CommonProviderMetadata `json:",inline" yaml:",inline"`
	PostgresTablePrefix    string `json:"postgresTablePrefix,omitempty"`
}

type AuthProviderStatus struct {
	CommonProviderStatus
	Namespace string `json:"namespace,omitempty"`
}

type AuthProviderList List[AuthProvider]
