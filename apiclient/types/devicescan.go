package types

// DeviceScan is the wire shape submitted by `obot scan`.
type DeviceScan struct {
	// ID is the server-assigned primary key. Zero on submission.
	ID uint `json:"id,omitempty"`
	// ReceivedAt is the server's receipt timestamp. Zero on submission.
	ReceivedAt Time `json:"received_at"`
	// SubmittedBy is the user ID of the caller. Set by the server.
	SubmittedBy string `json:"submitted_by,omitempty"`

	// ScannerVersion is the obot version that produced the scan.
	ScannerVersion string `json:"scanner_version"`
	// ScannedAt is when the scanner finished collecting on the device.
	ScannedAt Time `json:"scanned_at"`
	// DeviceID is the persisted per-device identifier so re-scans collate.
	DeviceID string `json:"device_id"`
	// Hostname is the device hostname at scan time.
	Hostname string `json:"hostname"`
	// OS is GOOS (darwin, linux, windows).
	OS string `json:"os"`
	// Arch is GOARCH (amd64, arm64).
	Arch string `json:"arch"`
	// Username is the OS user that ran the scan.
	Username string `json:"username,omitempty"`

	// Files are the config / manifest files captured during the scan,
	// deduped by absolute path.
	Files []DeviceScanFile `json:"files"`
	// MCPServers are the MCP server observations.
	MCPServers []DeviceScanMCPServer `json:"mcp_servers"`
	// Skills are the skill observations (SKILL.md hits).
	Skills []DeviceScanSkill `json:"skills"`
	// Plugins are the plugin observations.
	Plugins []DeviceScanPlugin `json:"plugins"`
	// Clients are the per-client presence + roll-up rows.
	Clients []DeviceScanClient `json:"clients"`
}

// DeviceScanList is the response shape for GET /api/devices/scans.
type DeviceScanList struct {
	Items  []DeviceScan `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// DeviceScanFile is one captured config or manifest file.
type DeviceScanFile struct {
	// Path is the absolute path on the device.
	Path string `json:"path"`
	// SizeBytes is the file size in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// Oversized is true when the file exceeded the per-file content cap.
	Oversized bool `json:"oversized"`
	// Content is the raw file bytes; omitted when Oversized.
	Content string `json:"content,omitempty"`
}

// DeviceScanMCPServer is one MCP server observation.
type DeviceScanMCPServer struct {
	// Client is the canonical client name (e.g. "cursor"); empty for orphans.
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope observations; empty for global.
	ProjectPath string `json:"project_path,omitempty"`
	// File is the absolute path of the defining config file.
	File string `json:"file,omitempty"`
	// ConfigHash is the content-addressed identity for fleet-wide aggregation.
	// Computed over Name, Transport, Command, Args, URL only — env / header
	// keys are excluded so a server with a rotated secret stays one entity.
	ConfigHash string `json:"config_hash,omitempty"`
	// EnvKeys are the env var names referenced by the server config (values redacted).
	EnvKeys []string `json:"env_keys"`
	// HeaderKeys are the HTTP header names referenced by the server config (values redacted).
	HeaderKeys []string `json:"header_keys"`
	// Name is the server's configured name.
	Name string `json:"name"`
	// Transport is "stdio", "http", "sse", etc.
	Transport string `json:"transport"`
	// Command is the stdio command, if any.
	Command string `json:"command,omitempty"`
	// Args are the stdio command arguments.
	Args []string `json:"args,omitempty"`
	// URL is the remote endpoint for HTTP / SSE transports.
	URL string `json:"url,omitempty"`
}

// DeviceScanSkill is one skill (SKILL.md) observation.
type DeviceScanSkill struct {
	// Client is the canonical client name; empty for free-floating SKILL.md files.
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope skills.
	ProjectPath string `json:"project_path,omitempty"`
	// File is the absolute path of the SKILL.md file.
	File string `json:"file,omitempty"`
	// Name is the skill name (typically the SKILL.md frontmatter name).
	Name string `json:"name"`
	// Description is the skill description from frontmatter.
	Description string `json:"description,omitempty"`
	// Files lists SKILL.md plus any supporting artifacts in the skill directory.
	Files []string `json:"files"`
	// HasScripts indicates the skill ships at least one executable script.
	HasScripts bool `json:"has_scripts"`
	// GitRemoteURL is the git remote of the skill's enclosing repo, if any.
	GitRemoteURL string `json:"git_remote_url,omitempty"`
}

// DeviceScanClient is a per-device record for an AI client. Carries
// presence facts plus roll-ups summarising what the device has
// configured for this client.
type DeviceScanClient struct {
	// Name is the canonical client name (e.g. "claudecode", "cursor").
	Name string `json:"name"`
	// Version is the client version, when one was discoverable.
	Version string `json:"version,omitempty"`
	// BinaryPath is the resolved $PATH location of the client binary.
	BinaryPath string `json:"binary_path,omitempty"`
	// InstallPath is the install location (e.g. an /Applications bundle).
	InstallPath string `json:"install_path,omitempty"`
	// ConfigPath is the client's primary config directory under $HOME.
	ConfigPath string `json:"config_path,omitempty"`
	// HasMCPServers is true when at least one MCPServers row references this client.
	HasMCPServers bool `json:"has_mcp_servers"`
	// HasSkills is true when at least one Skills row references this client.
	HasSkills bool `json:"has_skills"`
	// HasPlugins is true when at least one Plugins row references this client.
	HasPlugins bool `json:"has_plugins"`
}

// DeviceScanPlugin is one plugin observation.
type DeviceScanPlugin struct {
	// Client is the canonical client name that owns the plugin host.
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope plugins.
	ProjectPath string `json:"project_path,omitempty"`
	// ConfigPath is the absolute path of the plugin's defining manifest.
	ConfigPath string `json:"config_path,omitempty"`
	// Name is the plugin name.
	Name string `json:"name"`
	// PluginType identifies the plugin shape (extension, marketplace package, etc.).
	PluginType string `json:"plugin_type"`
	// Version is the plugin version.
	Version string `json:"version,omitempty"`
	// Description is the plugin description from its manifest.
	Description string `json:"description,omitempty"`
	// Author is the plugin author from its manifest.
	Author string `json:"author,omitempty"`
	// Marketplace is the source marketplace, if applicable.
	Marketplace string `json:"marketplace,omitempty"`
	// Files lists every file collected from the plugin directory.
	Files []string `json:"files"`
	// Enabled is true when the plugin is enabled per the host's config.
	Enabled bool `json:"enabled"`
	// HasMCPServers is true when the plugin defines MCP servers.
	HasMCPServers bool `json:"has_mcp_servers"`
	// HasSkills is true when the plugin defines skills.
	HasSkills bool `json:"has_skills"`
	// HasRules is true when the plugin defines rules.
	HasRules bool `json:"has_rules"`
	// HasCommands is true when the plugin defines commands.
	HasCommands bool `json:"has_commands"`
	// HasHooks is true when the plugin defines hooks.
	HasHooks bool `json:"has_hooks"`
}

// DeviceMCPServerStat is one row of the fleet-wide MCP aggregation,
// keyed by ConfigHash.
type DeviceMCPServerStat struct {
	// ConfigHash is the aggregation key.
	ConfigHash string   `json:"config_hash"`
	Name       string   `json:"name"`
	Transport  string   `json:"transport"`
	Command    string   `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	URL        string   `json:"url,omitempty"`
	// DeviceCount is the number of distinct devices observing this hash.
	DeviceCount int64 `json:"device_count"`
	// UserCount is the number of distinct submitters observing this hash.
	UserCount int64 `json:"user_count"`
	// ClientCount is the number of distinct client names observing this hash.
	ClientCount int64 `json:"client_count"`
	// ScopeCount is the number of distinct scopes (global / project) observing this hash.
	ScopeCount int64 `json:"scope_count"`
	// ObservationCount is the total number of rows with this hash.
	ObservationCount int64 `json:"observation_count"`
}

// DeviceMCPServerDetail is the response shape for
// GET /api/devices/mcp-servers/{config_hash}.
type DeviceMCPServerDetail struct {
	DeviceMCPServerStat
	EnvKeys    []string `json:"env_keys"`
	HeaderKeys []string `json:"header_keys"`
}

// DeviceClientStat is one row of the dashboard's per-client rollup.
type DeviceClientStat struct {
	// Name is the canonical client name.
	Name string `json:"name"`
	// DeviceCount is the number of distinct devices with this client.
	DeviceCount int64 `json:"device_count"`
	// UserCount is the number of distinct submitters with this client.
	UserCount int64 `json:"user_count"`
	// ObservationCount is the total number of client rows for this name.
	ObservationCount int64 `json:"observation_count"`
}

// DeviceSkillStat is one row of the dashboard's per-skill rollup.
type DeviceSkillStat struct {
	// Name is the skill name (the aggregation key).
	Name string `json:"name"`
	// DeviceCount is the number of distinct devices with this skill.
	DeviceCount int64 `json:"device_count"`
	// UserCount is the number of distinct submitters with this skill.
	UserCount int64 `json:"user_count"`
	// ObservationCount is the total number of skill rows for this name.
	ObservationCount int64 `json:"observation_count"`
}

// DeviceScanStats is the response shape for GET /api/devices/scan-stats.
type DeviceScanStats struct {
	// TimeStart is the inclusive lower bound of the rollup window.
	TimeStart Time `json:"time_start"`
	// TimeEnd is the exclusive upper bound of the rollup window.
	TimeEnd Time `json:"time_end"`
	// DeviceCount is the number of distinct devices in the window.
	DeviceCount int64 `json:"device_count"`
	// UserCount is the number of distinct submitters in the window.
	UserCount int64 `json:"user_count"`
	// Clients is the full ranked per-client breakdown.
	Clients []DeviceClientStat `json:"clients"`
	// MCPServers is the full ranked per-ConfigHash breakdown.
	MCPServers []DeviceMCPServerStat `json:"mcp_servers"`
	// Skills is the full ranked per-skill breakdown.
	Skills []DeviceSkillStat `json:"skills"`
	// ScanTimestamps is every scan submission's scanned_at inside the
	// window, sorted ascending. The dashboard chart buckets these
	// client-side in the user's local timezone. Counts every submission,
	// not just the latest-per-device subset that drives the other
	// rollups.
	ScanTimestamps []Time `json:"scan_timestamps"`
}

// DeviceMCPServerOccurrence is one device's latest-scan row for a
// specific ConfigHash.
type DeviceMCPServerOccurrence struct {
	// DeviceScanID is the parent scan's primary key.
	DeviceScanID uint `json:"device_scan_id"`
	// DeviceID is the device that submitted the parent scan.
	DeviceID string `json:"device_id"`
	// Client is the canonical client name on this row.
	Client string `json:"client"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ScannedAt is when the parent scan was collected on the device.
	ScannedAt Time `json:"scanned_at"`
	// Index is the position of this row inside the parent scan's MCPServers slice.
	Index int `json:"index"`
}

// DeviceMCPServerOccurrenceList is the response shape for
// GET /api/devices/mcp-servers/{config_hash}/occurrences.
type DeviceMCPServerOccurrenceList struct {
	Items  []DeviceMCPServerOccurrence `json:"items"`
	Total  int64                       `json:"total"`
	Limit  int                         `json:"limit"`
	Offset int                         `json:"offset"`
}

// DeviceSkillStatList is the response shape for GET /api/devices/skills.
type DeviceSkillStatList struct {
	Items  []DeviceSkillStat `json:"items"`
	Total  int64             `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// DeviceSkillDetail is the response shape for GET /api/devices/skills/{name}.
type DeviceSkillDetail struct {
	DeviceSkillStat
	Description  string   `json:"description,omitempty"`
	HasScripts   bool     `json:"has_scripts"`
	GitRemoteURL string   `json:"git_remote_url,omitempty"`
	Files        []string `json:"files,omitempty"`
}

// DeviceSkillOccurrence is one device's latest-scan row for a specific
// skill name.
type DeviceSkillOccurrence struct {
	// DeviceScanID is the parent scan's primary key.
	DeviceScanID uint `json:"device_scan_id"`
	// DeviceID is the device that submitted the parent scan.
	DeviceID string `json:"device_id"`
	// Client is the canonical client name on this row.
	Client string `json:"client"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ProjectPath is the project root for project-scope rows.
	ProjectPath string `json:"project_path,omitempty"`
	// ScannedAt is when the parent scan was collected on the device.
	ScannedAt Time `json:"scanned_at"`
	// Index is the position of this row inside the parent scan's Skills slice.
	Index int `json:"index"`
}

// DeviceSkillOccurrenceList is the response shape for
// GET /api/devices/skills/{name}/occurrences.
type DeviceSkillOccurrenceList struct {
	Items  []DeviceSkillOccurrence `json:"items"`
	Total  int64                   `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}
