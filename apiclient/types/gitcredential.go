package types

// GitCredential represents an admin-managed credential for HTTPS Git repositories.
type GitCredential struct {
	Metadata
	DisplayName     string `json:"displayName,omitempty"`
	Host            string `json:"host,omitempty"`
	TokenConfigured bool   `json:"tokenConfigured,omitempty"`
	InUse           bool   `json:"inUse"`
}

// GitCredentialManifest is accepted when creating or updating a Git credential.
type GitCredentialManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	Host        string `json:"host,omitempty"`
	Token       string `json:"token,omitempty"`
}

type GitCredentialList List[GitCredential]
