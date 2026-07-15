package types

// GitCredential represents an admin-managed credential for HTTPS Git repositories.
type GitCredential struct {
	Metadata
	GitCredentialManifest
	TokenConfigured bool `json:"tokenConfigured,omitempty"`
}

type GitCredentialManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	Host        string `json:"host,omitempty"`
	Token       string `json:"token,omitempty"`
}

type GitCredentialList List[GitCredential]
