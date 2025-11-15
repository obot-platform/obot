package types

// RegistryServerList represents the paginated list response from /v0/servers
type RegistryServerList struct {
	Servers  []RegistryServerResponse `json:"servers"`
	Metadata *RegistryMetadata        `json:"metadata,omitempty"`
}

// RegistryMetadata contains pagination metadata
type RegistryMetadata struct {
	NextCursor string `json:"nextCursor,omitempty"`
	Count      int    `json:"count,omitempty"`
}

// RegistryServerResponse wraps a server with registry metadata
type RegistryServerResponse struct {
	Server ServerDetail `json:"server"`
	Meta   RegistryMeta `json:"_meta,omitzero"`
}

// ServerDetail matches the Registry API ServerDetail schema
// For Obot, configured servers always use Remotes (never Packages)
type ServerDetail struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Title       string              `json:"title,omitempty"`
	Version     string              `json:"version"`
	WebsiteURL  string              `json:"websiteUrl,omitempty"`
	Icons       []RegistryIcon      `json:"icons,omitempty"`
	Remotes     []RegistryRemote    `json:"remotes,omitempty"`
	Repository  *RegistryRepository `json:"repository,omitempty"`
}

// RegistryIcon represents an icon for display
type RegistryIcon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

// RegistryRemote represents a remote server configuration
// All Obot servers are exposed as streamable-http remotes via mcp-connect
type RegistryRemote struct {
	Type string `json:"type"` // Always "streamable-http" for configured Obot servers
	URL  string `json:"url"`  // The mcp-connect URL
}

// RegistryRepository represents repository metadata
type RegistryRepository struct {
	URL       string `json:"url"`
	Source    string `json:"source"`
	ID        string `json:"id,omitempty"`
	Subfolder string `json:"subfolder,omitempty"`
}

// RegistryMeta contains registry-managed metadata
type RegistryMeta struct {
	Obot *RegistryObotMeta `json:"ai.obot/server,omitempty"`
}

// RegistryObotMeta contains Obot-specific metadata
type RegistryObotMeta struct {
	ConfigurationRequired bool   `json:"configurationRequired,omitempty"`
	ConfigurationMessage  string `json:"configurationMessage,omitempty"`
}
