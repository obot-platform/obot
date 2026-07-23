package types

import "encoding/json"

type MDMConfigurationManifest struct {
	AssetDigest string          `json:"assetDigest,omitempty"`
	Values      json.RawMessage `json:"values,omitempty"`
}

// MDMConfiguration is a fleet grouping that devices enroll into. AssetDigest
// identifies the asset bundle whose fields Values conform to. Artifacts are
// rendered for every platform and OS in that bundle when Values are saved.
type MDMConfiguration struct {
	MDMConfigurationManifest `json:",inline"`

	ID        uint `json:"id"`
	IsDefault bool `json:"isDefault"`
	CreatedAt Time `json:"createdAt"`

	// ObotSentryVersion is copied from the source bundle's manifest when the
	// artifacts are rendered. It is server-owned and reports the version the
	// saved packages were generated with.
	ObotSentryVersion string `json:"obotSentryVersion,omitempty"`

	Artifacts []MDMConfigurationArtifact `json:"artifacts"`

	EnforcementEnabled   bool                 `json:"enforcementEnabled,omitempty"`
	EnforcementAllowlist EnforcementAllowlist `json:"enforcementAllowlist"`
}

type MDMConfigurationEnforcementRequest struct {
	EnforcementEnabled   bool                 `json:"enforcementEnabled,omitempty"`
	EnforcementAllowlist EnforcementAllowlist `json:"enforcementAllowlist"`
}

type EnforcementAllowlist struct {
	AllowEverything           bool `json:"allowEverything,omitempty"`
	AllowAllObotHostedMCP     bool `json:"allowAllObotHostedMcpServers,omitempty"`
	AllowAllBuiltinAgentTools bool `json:"allowAllBuiltinAgentTools,omitempty"`
	// AllowAllBuiltinAgentMCP allows any call to a built-in agent MCP server (i.e. Codex computer-use)
	AllowAllBuiltinAgentMCP bool `json:"allowAllBuiltinAgentMcpServers,omitempty"`

	Servers []AllowlistServer `json:"servers,omitempty"`
}

type AllowlistServer struct {
	URL      string                  `json:"url,omitempty"`
	Package  *AllowlistServerPackage `json:"package,omitempty"`
	Hostname string                  `json:"hostname,omitempty"`
	Tools    []string                `json:"tools,omitempty"` // empty = all tools on this server
}

type AllowlistServerPackageSource string

const (
	AllowlistServerPackageSourceNPM  AllowlistServerPackageSource = "npm"
	AllowlistServerPackageSourcePyPI AllowlistServerPackageSource = "pypi"
)

type AllowlistServerPackage struct {
	Source  AllowlistServerPackageSource `json:"source"` // npm | pypi
	Name    string                       `json:"name"`
	Version string                       `json:"version,omitempty"` // empty = accept any version
}

type MDMConfigurationList List[MDMConfiguration]

// MDMConfigurationArtifact is one rendered deployment option. Slug selects its
// download endpoint; ZIP content, content digest, and filename remain private.
type MDMConfigurationArtifact struct {
	Slug         string `json:"slug"`
	Platform     string `json:"platform"`
	OS           string `json:"os"`
	Instructions string `json:"instructions"`
}

type MDMEnrollmentKey struct {
	ID         uint   `json:"id"`
	Name       string `json:"name,omitempty"`
	CreatedAt  Time   `json:"createdAt"`
	LastUsedAt *Time  `json:"lastUsedAt,omitempty"`
	ExpiresAt  *Time  `json:"expiresAt,omitempty"`
}

type MDMEnrollmentKeyList List[MDMEnrollmentKey]

type MDMEnrollmentKeyCreateRequest struct {
	Name      string `json:"name,omitempty"`
	ExpiresAt *Time  `json:"expiresAt,omitempty"`
}

type MDMEnrollmentKeyCreateResponse struct {
	MDMEnrollmentKey
	EnrollmentCredential string `json:"enrollmentCredential"`
}
