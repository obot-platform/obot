package types

// DeviceScanManifest is what `obot scan` submits. Server-assigned
// fields (id, receivedAt, submittedBy) live on DeviceScan instead.
// Child observations share the same wire type for submission and
// response — the ID field is server-set and decoded into a zero value
// on submission, which DeviceScanFromManifest deliberately does not
// copy. Submitters cannot trample existing row PKs.
type DeviceScanManifest struct {
	// ScannerVersion is the obot version that produced the scan.
	ScannerVersion string `json:"scannerVersion"`
	// ScannedAt is when the scanner finished collecting on the device.
	ScannedAt Time `json:"scannedAt"`
	// DeviceID is the persisted per-device identifier so re-scans collate.
	DeviceID string `json:"deviceID"`
	// Hostname is the device hostname at scan time.
	Hostname string `json:"hostname"`
	// OS is the operating system (darwin, linux, windows).
	OS string `json:"os"`
	// Arch is the CPU architecture (amd64, arm64).
	Arch string `json:"arch"`
	// Username is the OS user that ran the scan.
	Username string `json:"username,omitempty"`
	// Files are the config / manifest files captured during the scan,
	// deduped by absolute path.
	Files []DeviceScanFile `json:"files"`
	// MCPServers are the MCP server observations.
	MCPServers []DeviceScanMCPServer `json:"mcpServers"`
	// Skills are the skill observations (SKILL.md hits).
	Skills []DeviceScanSkill `json:"skills"`
	// Plugins are the plugin observations.
	Plugins []DeviceScanPlugin `json:"plugins"`
	// Clients are the per-client presence + roll-up rows.
	Clients []DeviceScanClient `json:"clients"`
}

// DeviceScan is a persisted scan: the submitted manifest plus
// server-assigned fields.
type DeviceScan struct {
	DeviceScanManifest `json:",inline"`
	// ID is the server-assigned primary key.
	ID uint `json:"id"`
	// ReceivedAt is the server's receipt timestamp.
	ReceivedAt Time `json:"receivedAt"`
	// SubmittedBy is the user ID of the caller that posted the scan.
	SubmittedBy string `json:"submittedBy"`
}

type DeviceScanList List[DeviceScan]

// DeviceScanResponse is returned by GET /api/devices/scans.
type DeviceScanResponse struct {
	DeviceScanList `json:",inline"`
	Total          int64 `json:"total"`
	Limit          int   `json:"limit"`
	Offset         int   `json:"offset"`
}

// DeviceScanFile is one captured config or manifest file.
type DeviceScanFile struct {
	// Path is the absolute path on the device.
	Path string `json:"path"`
	// SizeBytes is the file size in bytes.
	SizeBytes int64 `json:"sizeBytes"`
	// Oversized is true when the file exceeded the per-file content cap.
	Oversized bool `json:"oversized"`
	// Content is the raw file bytes; omitted when Oversized.
	Content string `json:"content,omitempty"`
}

// DeviceScanMCPServer is one MCP server observation. ID is
// server-assigned on insert and stable across responses.
type DeviceScanMCPServer struct {
	// ID is the row's primary key. Server-set; ignored on submission.
	ID uint `json:"id,omitempty"`
	// Client is the canonical client name (e.g. "cursor"); empty for orphans.
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope observations; empty for global.
	ProjectPath string `json:"projectPath,omitempty"`
	// File is the absolute path of the defining config file.
	File string `json:"file,omitempty"`
	// ConfigHash is the content-addressed identity for fleet-wide aggregation.
	// Computed over Name, Transport, Command, Args, URL only — env / header
	// keys are excluded so a server with a rotated secret stays one entity.
	ConfigHash string `json:"configHash,omitempty"`
	// EnvKeys are the env var names referenced by the server config (values redacted).
	EnvKeys []string `json:"envKeys"`
	// HeaderKeys are the HTTP header names referenced by the server config (values redacted).
	HeaderKeys []string `json:"headerKeys"`
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

// DeviceScanSkill is one skill (SKILL.md) observation. ID is
// server-assigned on insert and stable across responses.
type DeviceScanSkill struct {
	// ID is the row's primary key. Server-set; ignored on submission.
	ID uint `json:"id,omitempty"`
	// Client is the canonical client name; "multi" for free-floating
	// SKILL.md files with no canonical owning client (e.g.
	// .agents/skills, .agent/skills, project skills outside a known
	// client tree).
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope skills.
	ProjectPath string `json:"projectPath,omitempty"`
	// File is the absolute path of the SKILL.md file.
	File string `json:"file,omitempty"`
	// Name is the skill name (typically the SKILL.md frontmatter name).
	Name string `json:"name"`
	// Description is the skill description from frontmatter.
	Description string `json:"description,omitempty"`
	// Files lists SKILL.md plus any supporting artifacts in the skill directory.
	Files []string `json:"files"`
	// HasScripts indicates the skill ships at least one executable script.
	HasScripts bool `json:"hasScripts"`
	// GitRemoteURL is the git remote of the skill's enclosing repo, if any.
	GitRemoteURL string `json:"gitRemoteURL,omitempty"`
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
	BinaryPath string `json:"binaryPath,omitempty"`
	// InstallPath is the install location (e.g. an /Applications bundle).
	InstallPath string `json:"installPath,omitempty"`
	// ConfigPath is the client's primary config directory under $HOME.
	ConfigPath string `json:"configPath,omitempty"`
	// HasMCPServers is true when at least one MCPServers row references this client.
	HasMCPServers bool `json:"hasMCPServers"`
	// HasSkills is true when at least one Skills row references this client.
	HasSkills bool `json:"hasSkills"`
	// HasPlugins is true when at least one Plugins row references this client.
	HasPlugins bool `json:"hasPlugins"`
}

// DeviceScanPlugin is one plugin observation. ID is server-assigned
// on insert and stable across responses.
type DeviceScanPlugin struct {
	// ID is the row's primary key. Server-set; ignored on submission.
	ID uint `json:"id,omitempty"`
	// Client is the canonical client name that owns the plugin host.
	Client string `json:"client"`
	// ProjectPath is the project root for project-scope plugins.
	ProjectPath string `json:"projectPath,omitempty"`
	// ConfigPath is the absolute path of the plugin's defining manifest.
	ConfigPath string `json:"configPath,omitempty"`
	// Name is the plugin name.
	Name string `json:"name"`
	// PluginType identifies the plugin kind (extension, marketplace package, etc.).
	PluginType string `json:"pluginType"`
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
	HasMCPServers bool `json:"hasMCPServers"`
	// HasSkills is true when the plugin defines skills.
	HasSkills bool `json:"hasSkills"`
	// HasRules is true when the plugin defines rules.
	HasRules bool `json:"hasRules"`
	// HasCommands is true when the plugin defines commands.
	HasCommands bool `json:"hasCommands"`
	// HasHooks is true when the plugin defines hooks.
	HasHooks bool `json:"hasHooks"`
}

// DeviceMCPServerStat is one row of the fleet-wide MCP aggregation,
// keyed by ConfigHash. Identity fields (Name, Transport, Command,
// Args, URL) are stable within a hash group by construction — they
// are inputs to the hash itself.
type DeviceMCPServerStat struct {
	// ConfigHash is the aggregation key.
	ConfigHash string   `json:"configHash"`
	Name       string   `json:"name"`
	Transport  string   `json:"transport"`
	Command    string   `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	URL        string   `json:"url,omitempty"`
	// DeviceCount is the number of distinct devices observing this hash.
	DeviceCount int64 `json:"deviceCount"`
	// UserCount is the number of distinct submitters observing this hash.
	UserCount int64 `json:"userCount"`
	// ClientCount is the number of distinct client names observing this hash.
	ClientCount int64 `json:"clientCount"`
	// ObservationCount is the total number of rows with this hash.
	ObservationCount int64 `json:"observationCount"`
}

// DeviceMCPServerDetail is the GET /api/devices/mcp-servers/{config_hash}
// response. EnvKeys and HeaderKeys are not in the hash and may vary
// per observation; they are unioned across all observations.
type DeviceMCPServerDetail struct {
	DeviceMCPServerStat
	// EnvKeys is the set union of env var names referenced across
	// observations of this hash.
	EnvKeys []string `json:"envKeys"`
	// HeaderKeys is the set union of HTTP header names referenced
	// across observations of this hash.
	HeaderKeys []string `json:"headerKeys"`
}

// DeviceClientStat is one row of the per-client rollup.
type DeviceClientStat struct {
	// Name is the canonical client name.
	Name string `json:"name"`
	// DeviceCount is the number of distinct devices with this client.
	DeviceCount int64 `json:"deviceCount"`
	// UserCount is the number of distinct submitters with this client.
	UserCount int64 `json:"userCount"`
	// ObservationCount is the total number of client rows for this name.
	ObservationCount int64 `json:"observationCount"`
}

// DeviceSkillStat is one row of the per-skill rollup.
type DeviceSkillStat struct {
	// Name is the skill name (the aggregation key).
	Name string `json:"name"`
	// DeviceCount is the number of distinct devices with this skill.
	DeviceCount int64 `json:"deviceCount"`
	// UserCount is the number of distinct submitters with this skill.
	UserCount int64 `json:"userCount"`
	// ObservationCount is the total number of skill rows for this name.
	ObservationCount int64 `json:"observationCount"`
}

type DeviceSkillStatList List[DeviceSkillStat]

// DeviceSkillStatResponse is returned by GET /api/devices/skills.
type DeviceSkillStatResponse struct {
	DeviceSkillStatList `json:",inline"`
	Total               int64 `json:"total"`
	Limit               int   `json:"limit"`
	Offset              int   `json:"offset"`
}

// DeviceSkillDetail is the GET /api/devices/skills/{name} response.
// The metadata fields come from one canonical observation and are not
// guaranteed to be stable across observations sharing the same name.
type DeviceSkillDetail struct {
	DeviceSkillStat
	// Description is the skill's short summary.
	Description string `json:"description,omitempty"`
	// HasScripts is true when the skill ships executable scripts.
	HasScripts bool `json:"hasScripts"`
	// GitRemoteURL is the upstream repo, if any.
	GitRemoteURL string `json:"gitRemoteURL,omitempty"`
	// Files lists every file collected from the skill directory.
	Files []string `json:"files,omitempty"`
}

// DeviceScanStats is returned by GET /api/devices/scan-stats.
type DeviceScanStats struct {
	// TimeStart is the inclusive lower bound of the rollup window.
	TimeStart Time `json:"timeStart"`
	// TimeEnd is the exclusive upper bound of the rollup window.
	TimeEnd Time `json:"timeEnd"`
	// DeviceCount is the number of distinct devices in the window.
	DeviceCount int64 `json:"deviceCount"`
	// UserCount is the number of distinct submitters in the window.
	UserCount int64 `json:"userCount"`
	// Clients is the full ranked per-client breakdown.
	Clients []DeviceClientStat `json:"clients"`
	// MCPServers is the full ranked per-ConfigHash breakdown.
	MCPServers []DeviceMCPServerStat `json:"mcpServers"`
	// Skills is the full ranked per-skill breakdown.
	Skills []DeviceSkillStat `json:"skills"`
	// ScanTimestamps is every scan submission's scanned_at inside the
	// window, sorted ascending. The dashboard chart buckets these
	// client-side in the user's local timezone. Counts every submission,
	// not just the latest-per-device subset that drives the other
	// rollups.
	ScanTimestamps []Time `json:"scanTimestamps"`
}

// DeviceMCPServerOccurrence is one device's latest-scan row for a
// specific ConfigHash.
type DeviceMCPServerOccurrence struct {
	// DeviceScanID is the parent scan's primary key.
	DeviceScanID uint `json:"deviceScanID"`
	// DeviceID is the device that submitted the parent scan.
	DeviceID string `json:"deviceID"`
	// Client is the canonical client name on this row.
	Client string `json:"client"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ScannedAt is when the parent scan was collected on the device.
	ScannedAt Time `json:"scannedAt"`
	// ID is the observation's stable identifier.
	ID uint `json:"id"`
}

type DeviceMCPServerOccurrenceList List[DeviceMCPServerOccurrence]

// DeviceMCPServerOccurrenceResponse is returned by
// GET /api/devices/mcp-servers/{config_hash}/occurrences.
type DeviceMCPServerOccurrenceResponse struct {
	DeviceMCPServerOccurrenceList `json:",inline"`
	Total                         int64 `json:"total"`
	Limit                         int   `json:"limit"`
	Offset                        int   `json:"offset"`
}

// DeviceSkillOccurrence is one device's latest-scan row for a specific
// skill name.
type DeviceSkillOccurrence struct {
	// DeviceScanID is the parent scan's primary key.
	DeviceScanID uint `json:"deviceScanID"`
	// DeviceID is the device that submitted the parent scan.
	DeviceID string `json:"deviceID"`
	// Client is the canonical client name on this row.
	Client string `json:"client"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ProjectPath is the project root for project-scope rows.
	ProjectPath string `json:"projectPath,omitempty"`
	// ScannedAt is when the parent scan was collected on the device.
	ScannedAt Time `json:"scannedAt"`
	// ID is the observation's stable identifier.
	ID uint `json:"id"`
}

type DeviceSkillOccurrenceList List[DeviceSkillOccurrence]

// DeviceSkillOccurrenceResponse is returned by
// GET /api/devices/skills/{name}/occurrences.
type DeviceSkillOccurrenceResponse struct {
	DeviceSkillOccurrenceList `json:",inline"`
	Total                     int64 `json:"total"`
	Limit                     int   `json:"limit"`
	Offset                    int   `json:"offset"`
}

// DeviceClientFleetSkill is one skill row on a device client fleet summary
// (client match, not "multi"; canonical row is earliest observation id per
// client + skill name).
type DeviceClientFleetSkill struct {
	// Name is the skill name (typically from SKILL.md frontmatter).
	Name string `json:"name"`
	// Description is the short summary from frontmatter when present.
	Description string `json:"description,omitempty"`
	// HasScripts is true when the skill directory includes executable scripts.
	HasScripts bool `json:"hasScripts"`
	// Files is the number of file paths recorded for that skill observation.
	Files int `json:"files"`
}

// DeviceClientFleetSummary rolls up latest-scan-per-device data for one
// canonical client name (from device_scan_clients).
type DeviceClientFleetSummary struct {
	// Name is the canonical client identifier (e.g. "cursor", "claude-code").
	Name string `json:"name"`
	// Users are distinct scan submitters whose latest scan lists this client.
	Users []string `json:"users"`
	// Skills lists one entry per distinct skill name with metadata on each
	// device's latest scan (client match; excludes "multi").
	Skills []DeviceClientFleetSkill `json:"skills"`
	// MCPServers are distinct MCP servers (by ConfigHash) observed with
	// Client == Name in those latest scans; rows with client "multi" are excluded.
	MCPServers []DeviceMCPServerStat `json:"mcpServers"`
}

type DeviceClientFleetSummaryList List[DeviceClientFleetSummary]

// DeviceClientFleetSummaryResponse is returned by GET /api/devices/clients.
type DeviceClientFleetSummaryResponse struct {
	DeviceClientFleetSummaryList `json:",inline"`
	Total                        int64 `json:"total"`
	Limit                        int    `json:"limit"`
	Offset                       int    `json:"offset"`
}
