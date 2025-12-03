package types

// RegistryServerList represents the paginated list response from /v0.1/servers
type RegistryServerList struct {
	Servers  []RegistryServerResponse    `json:"servers"`
	Metadata *RegistryServerListMetadata `json:"metadata,omitempty"`
}

// RegistryServerListMetadata contains pagination metadata
type RegistryServerListMetadata struct {
	NextCursor string `json:"nextCursor,omitempty"`
	Count      int    `json:"count,omitempty"`
}

// RegistryServerResponse wraps a server with registry metadata
type RegistryServerResponse struct {
	Server RegistryServerDetail `json:"server"`
	Meta   RegistryMeta         `json:"_meta,omitzero"`

	// CreatedAtUnix is used to help sort during pagination. It is not returned in the actual response.
	CreatedAtUnix int64 `json:"-"`
}

// RegistryServerDetail matches the Registry API RegistryServerDetail schema
// For Obot, configured servers always use Remotes (never Packages)
type RegistryServerDetail struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Title       string                    `json:"title,omitempty"`
	Version     string                    `json:"version"`
	WebsiteURL  string                    `json:"websiteUrl,omitempty"`
	Icons       []RegistryServerIcon      `json:"icons,omitempty"`
	Remotes     []RegistryServerRemote    `json:"remotes,omitempty"`
	Repository  *RegistryServerRepository `json:"repository,omitempty"`
	Schema      string                    `json:"$schema,omitempty"`
	Meta        RegistryServerMeta        `json:"_meta"`
}

// RegistryServerIcon represents an icon for display
type RegistryServerIcon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

// RegistryServerRemote represents a remote server configuration
// All Obot servers are exposed as streamable-http remotes via mcp-connect
type RegistryServerRemote struct {
	Type string `json:"type"` // Always "streamable-http" for configured Obot servers
	URL  string `json:"url"`  // The mcp-connect URL
}

// RegistryServerRepository represents repository metadata
type RegistryServerRepository struct {
	URL       string `json:"url"`
	Source    string `json:"source"`
	ID        string `json:"id,omitempty"`
	Subfolder string `json:"subfolder,omitempty"`
}

// RegistryMeta contains registry-managed metadata
type RegistryMeta struct {
	Obot     *RegistryObotMeta    `json:"ai.obot/server,omitempty"`
	Official RegistryOfficialMeta `json:"io.modelcontextprotocol.registry/official"`
}

type RegistryOfficialMeta struct {
	IsLatest  bool   `json:"isLatest"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// RegistryObotMeta contains Obot-specific metadata
type RegistryObotMeta struct {
	ConfigurationRequired bool   `json:"configurationRequired,omitempty"`
	ConfigurationMessage  string `json:"configurationMessage,omitempty"`
}

type RegistryServerMeta struct {
	PublisherProvided *RegistryPublisherProvidedMeta `json:"io.modelcontextprotocol.registry/publisher-provided,omitempty"`
}

type RegistryPublisherProvidedMeta struct {
	GitHub *RegistryGitHubMeta `json:"github,omitempty"`
}

// RegistryGitHubMeta allows us to supply a readme that will be displayed in the registry UI in VSCode.
type RegistryGitHubMeta struct {
	Readme string `json:"readme,omitempty"`
}
