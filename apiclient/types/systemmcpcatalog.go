package types

type SystemMCPCatalogManifest struct {
	DisplayName          string            `json:"displayName"`
	SourceURLs           []string          `json:"sourceURLs"`
	SourceURLCredentials map[string]string `json:"sourceURLCredentials,omitempty"`
}

type SystemMCPServerType string

const (
	SystemMCPServerTypeFilter SystemMCPServerType = "filter"
)

type SystemMCPCatalog struct {
	Metadata
	SystemMCPCatalogManifest
	LastSynced Time              `json:"lastSynced,omitzero"`
	SyncErrors map[string]string `json:"syncErrors,omitempty"`
	IsSyncing  bool              `json:"isSyncing,omitempty"`
}

type SystemMCPCatalogList List[SystemMCPCatalog]

type SystemMCPServerCatalogEntry struct {
	Metadata
	Manifest                  SystemMCPServerCatalogEntryManifest `json:"manifest"`
	Editable                  bool                                `json:"editable,omitempty"`
	CatalogName               string                              `json:"catalogName,omitempty"`
	SourceURL                 string                              `json:"sourceURL,omitempty"`
	LastUpdated               *Time                               `json:"lastUpdated,omitempty"`
	ToolPreviewsLastGenerated *Time                               `json:"toolPreviewsLastGenerated,omitempty"`
	NeedsUpdate               bool                                `json:"needsUpdate,omitempty"`
	OAuthCredentialConfigured bool                                `json:"oauthCredentialConfigured,omitempty"`
}

type SystemMCPServerCatalogEntryManifest struct {
	Metadata         map[string]string `json:"metadata,omitempty"`
	Name             string            `json:"name"`
	ShortDescription string            `json:"shortDescription"`
	Description      string            `json:"description"`
	Icon             string            `json:"icon"`
	RepoURL          string            `json:"repoURL,omitempty"`
	ToolPreview      []MCPServerTool   `json:"toolPreview,omitempty"`

	SystemMCPServerType SystemMCPServerType `json:"systemMCPServerType,omitempty"`

	FilterConfig *FilterConfig `json:"filterConfig,omitempty"`

	Runtime Runtime `json:"runtime"`

	UVXConfig           *UVXRuntimeConfig           `json:"uvxConfig,omitempty"`
	NPXConfig           *NPXRuntimeConfig           `json:"npxConfig,omitempty"`
	ContainerizedConfig *ContainerizedRuntimeConfig `json:"containerizedConfig,omitempty"`
	RemoteConfig        *RemoteCatalogConfig        `json:"remoteConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`
}

type FilterConfig struct {
	ToolName string `json:"toolName"`
}

type SystemMCPServerCatalogEntryList List[SystemMCPServerCatalogEntry]
