package types

// GitCredential represents an admin-managed credential for HTTPS Git repositories.
type GitCredential struct {
	Metadata
	DisplayName     string            `json:"displayName,omitempty"`
	Host            string            `json:"host,omitempty"`
	TokenConfigured bool              `json:"tokenConfigured,omitempty"`
	Uses            GitCredentialUses `json:"uses"`
}

type GitCredentialUses struct {
	SkillRepositories []GitCredentialUse `json:"skillRepositories"`
	MCPCatalogs       []GitCredentialUse `json:"mcpCatalogs"`
	SystemMCPCatalogs []GitCredentialUse `json:"systemMcpCatalogs"`
}

type GitCredentialUse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}

// GitCredentialManifest is accepted when creating or updating a Git credential.
type GitCredentialManifest struct {
	DisplayName string `json:"displayName,omitempty"`
	Host        string `json:"host,omitempty"`
	Token       string `json:"token,omitempty"`
}

type GitCredentialList List[GitCredential]
