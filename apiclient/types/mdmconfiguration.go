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
	// AllowAllBuiltinAgentMCP allows any call to a built-in agent MCP server (i.e.
	// Claude Code's workspace or claude-in-chrome)
	AllowAllBuiltinAgentMCP bool `json:"allowAllBuiltinAgentMcpServers,omitempty"`

	Servers []AllowlistServer `json:"servers,omitempty"`
}

// AllowlistServer allows MCP tool calls to one server. Exactly one of URL,
// Package, Hostname, or Connector identifies the server.
type AllowlistServer struct {
	URL      string                  `json:"url,omitempty"`
	Package  *AllowlistServerPackage `json:"package,omitempty"`
	Hostname string                  `json:"hostname,omitempty"`
	// Connector allowlists a hosted account connector by display name, for
	// agent-account connectors that expose no local URL or command. The device
	// attests which connector a call targeted; this decides whether it is
	// permitted. Matched case-insensitively.
	Connector string `json:"connector,omitempty"`
	// Tools limits the entry to these tool names; empty allows every tool on the server.
	Tools []string `json:"tools,omitempty"`
}

type AllowlistServerPackageSource string

const (
	AllowlistServerPackageSourceNPM  AllowlistServerPackageSource = "npm"
	AllowlistServerPackageSourcePyPI AllowlistServerPackageSource = "pypi"
)

type AllowlistServerPackage struct {
	// Source is the registry the package is published to: npm | pypi.
	Source AllowlistServerPackageSource `json:"source"`
	// Name is the package name as published to Source.
	Name string `json:"name"`
	// Version pins an exact version; empty accepts any version.
	Version string `json:"version,omitempty"`
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
