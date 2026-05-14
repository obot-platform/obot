//nolint:revive
package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/datatypes"
)

// DeviceScan is the parent envelope. Children (MCPServers, Skills,
// Plugins, Files) are GORM associations — db.Create(&scan) inserts
// everything atomically; db.Preload(...).First(...) loads them back.
//
// Composite indexes:
//   - idx_ds_user_time   (submitted_by, created_at) — list scans for a user
//   - idx_ds_device_time (device_id, created_at)    — list scans for a device
type DeviceScan struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"createdAt" gorm:"index:idx_ds_user_time,priority:2;index:idx_ds_device_time,priority:2"`
	SubmittedBy    string    `json:"submittedBy" gorm:"index:idx_ds_user_time,priority:1"`
	DeviceID       string    `json:"deviceID" gorm:"index:idx_ds_device_time,priority:1"`
	Hostname       string    `json:"hostname"`
	Username       string    `json:"username"`
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	ScannerVersion string    `json:"scannerVersion"`
	ScannedAt      time.Time `json:"scannedAt" gorm:"index"`

	MCPServers []DeviceScanMCPServer `json:"mcpServers,omitempty" gorm:"foreignKey:DeviceScanID;constraint:OnDelete:CASCADE"`
	Skills     []DeviceScanSkill     `json:"skills,omitempty"     gorm:"foreignKey:DeviceScanID;constraint:OnDelete:CASCADE"`
	Plugins    []DeviceScanPlugin    `json:"plugins,omitempty"    gorm:"foreignKey:DeviceScanID;constraint:OnDelete:CASCADE"`
	Files      []DeviceScanFile      `json:"files,omitempty"      gorm:"foreignKey:DeviceScanID;constraint:OnDelete:CASCADE"`
	Clients    []DeviceScanClient    `json:"clients,omitempty"    gorm:"foreignKey:DeviceScanID;constraint:OnDelete:CASCADE"`
}

// DeviceScanMCPServer is one MCP server observation. Scope is derived
// at insert time from ProjectPath ("" → "global", non-empty → "project")
// and persisted denormalized so list queries hit a single table.
type DeviceScanMCPServer struct {
	ID           uint                        `json:"id" gorm:"primaryKey"`
	DeviceScanID uint                        `json:"deviceScanID" gorm:"index;not null"`
	CreatedAt    time.Time                   `json:"createdAt" gorm:"index"`
	Client       string                      `json:"client" gorm:"index"`
	Scope        string                      `json:"scope" gorm:"index"`
	ProjectPath  string                      `json:"projectPath" gorm:"index"`
	File         string                      `json:"file"`
	Name         string                      `json:"name" gorm:"index"`
	Transport    string                      `json:"transport" gorm:"index"`
	Command      string                      `json:"command"`
	Args         datatypes.JSONSlice[string] `json:"args"`
	URL          string                      `json:"url"`
	EnvKeys      datatypes.JSONSlice[string] `json:"envKeys"`
	HeaderKeys   datatypes.JSONSlice[string] `json:"headerKeys"`
	ConfigHash   string                      `json:"configHash" gorm:"index"`
}

type DeviceScanSkill struct {
	ID           uint                        `json:"id" gorm:"primaryKey"`
	DeviceScanID uint                        `json:"deviceScanID" gorm:"index;not null"`
	CreatedAt    time.Time                   `json:"createdAt" gorm:"index"`
	Client       string                      `json:"client" gorm:"index"`
	Scope        string                      `json:"scope" gorm:"index"`
	ProjectPath  string                      `json:"projectPath" gorm:"index"`
	File         string                      `json:"file"`
	Name         string                      `json:"name" gorm:"index"`
	Description  string                      `json:"description"`
	HasScripts   bool                        `json:"hasScripts"`
	GitRemoteURL string                      `json:"gitRemoteURL" gorm:"index"`
	Files        datatypes.JSONSlice[string] `json:"files"`
}

type DeviceScanPlugin struct {
	ID            uint                        `json:"id" gorm:"primaryKey"`
	DeviceScanID  uint                        `json:"deviceScanID" gorm:"index;not null"`
	CreatedAt     time.Time                   `json:"createdAt" gorm:"index"`
	Client        string                      `json:"client" gorm:"index"`
	Scope         string                      `json:"scope" gorm:"index"`
	ProjectPath   string                      `json:"projectPath" gorm:"index"`
	ConfigPath    string                      `json:"configPath"`
	Name          string                      `json:"name" gorm:"index"`
	PluginType    string                      `json:"pluginType" gorm:"index"`
	Version       string                      `json:"version"`
	Description   string                      `json:"description"`
	Author        string                      `json:"author"`
	Enabled       bool                        `json:"enabled"`
	Marketplace   string                      `json:"marketplace"`
	Files         datatypes.JSONSlice[string] `json:"files"`
	HasMCPServers bool                        `json:"hasMCPServers"`
	HasSkills     bool                        `json:"hasSkills"`
	HasRules      bool                        `json:"hasRules"`
	HasCommands   bool                        `json:"hasCommands"`
	HasHooks      bool                        `json:"hasHooks"`
}

// DeviceScanClient is a per-scan record for an AI client observed on
// the device. Presence facts (BinaryPath, InstallPath, StateDir,
// Version) come from generic per-client detection. Has{MCPServers,
// Skills,Plugins} are roll-ups derived from observations attributed
// to this client name in the same scan.
type DeviceScanClient struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	DeviceScanID  uint      `json:"deviceScanID" gorm:"index;not null"`
	CreatedAt     time.Time `json:"createdAt" gorm:"index"`
	Name          string    `json:"name" gorm:"index"`
	Version       string    `json:"version"`
	BinaryPath    string    `json:"binaryPath"`
	InstallPath   string    `json:"installPath"`
	ConfigPath    string    `json:"configPath"`
	HasMCPServers bool      `json:"hasMCPServers"`
	HasSkills     bool      `json:"hasSkills"`
	HasPlugins    bool      `json:"hasPlugins"`
}

type DeviceScanFile struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	DeviceScanID uint      `json:"deviceScanID" gorm:"index;not null"`
	CreatedAt    time.Time `json:"createdAt" gorm:"index"`
	Path         string    `json:"path" gorm:"index"`
	SizeBytes    int64     `json:"sizeBytes"`
	Oversized    bool      `json:"oversized"`
	Content      string    `json:"content" gorm:"type:text"`
}

// ConvertDeviceScan converts internal DeviceScan to API type. Children
// must already be loaded (via Preload) for them to appear in the result.
func ConvertDeviceScan(s DeviceScan) types2.DeviceScan {
	out := types2.DeviceScan{
		ID:          s.ID,
		ReceivedAt:  *types2.NewTime(s.CreatedAt),
		SubmittedBy: s.SubmittedBy,
		DeviceScanManifest: types2.DeviceScanManifest{
			ScannerVersion: s.ScannerVersion,
			ScannedAt:      *types2.NewTime(s.ScannedAt),
			DeviceID:       s.DeviceID,
			Hostname:       s.Hostname,
			OS:             s.OS,
			Arch:           s.Arch,
			Username:       s.Username,
		},
	}
	if len(s.Files) > 0 {
		out.Files = make([]types2.DeviceScanFile, len(s.Files))
		for i, f := range s.Files {
			out.Files[i] = ConvertDeviceScanFile(f)
		}
	}
	if len(s.MCPServers) > 0 {
		out.MCPServers = make([]types2.DeviceScanMCPServer, len(s.MCPServers))
		for i, m := range s.MCPServers {
			out.MCPServers[i] = ConvertDeviceScanMCPServer(m)
		}
	}
	if len(s.Skills) > 0 {
		out.Skills = make([]types2.DeviceScanSkill, len(s.Skills))
		for i, sk := range s.Skills {
			out.Skills[i] = ConvertDeviceScanSkill(sk)
		}
	}
	if len(s.Plugins) > 0 {
		out.Plugins = make([]types2.DeviceScanPlugin, len(s.Plugins))
		for i, p := range s.Plugins {
			out.Plugins[i] = ConvertDeviceScanPlugin(p)
		}
	}
	if len(s.Clients) > 0 {
		out.Clients = make([]types2.DeviceScanClient, len(s.Clients))
		for i, c := range s.Clients {
			out.Clients[i] = ConvertDeviceScanClient(c)
		}
	}
	return out
}

func ConvertDeviceScanClient(c DeviceScanClient) types2.DeviceScanClient {
	return types2.DeviceScanClient{
		Name:          c.Name,
		Version:       c.Version,
		BinaryPath:    c.BinaryPath,
		InstallPath:   c.InstallPath,
		ConfigPath:    c.ConfigPath,
		HasMCPServers: c.HasMCPServers,
		HasSkills:     c.HasSkills,
		HasPlugins:    c.HasPlugins,
	}
}

// ConvertDeviceScanFile converts a stored file row to its wire form.
// Content is included only when the file wasn't flagged as oversized.
func ConvertDeviceScanFile(f DeviceScanFile) types2.DeviceScanFile {
	out := types2.DeviceScanFile{
		Path:      f.Path,
		SizeBytes: f.SizeBytes,
		Oversized: f.Oversized,
	}
	if !f.Oversized {
		out.Content = f.Content
	}
	return out
}

func ConvertDeviceScanMCPServer(m DeviceScanMCPServer) types2.DeviceScanMCPServer {
	return types2.DeviceScanMCPServer{
		ID:          m.ID,
		Client:      m.Client,
		ProjectPath: m.ProjectPath,
		File:        m.File,
		Name:        m.Name,
		Transport:   m.Transport,
		Command:     m.Command,
		Args:        []string(m.Args),
		URL:         m.URL,
		EnvKeys:     []string(m.EnvKeys),
		HeaderKeys:  []string(m.HeaderKeys),
		ConfigHash:  m.ConfigHash,
	}
}

func ConvertDeviceScanSkill(s DeviceScanSkill) types2.DeviceScanSkill {
	return types2.DeviceScanSkill{
		ID:           s.ID,
		Client:       s.Client,
		ProjectPath:  s.ProjectPath,
		File:         s.File,
		Name:         s.Name,
		Description:  s.Description,
		Files:        []string(s.Files),
		HasScripts:   s.HasScripts,
		GitRemoteURL: s.GitRemoteURL,
	}
}

func ConvertDeviceScanPlugin(p DeviceScanPlugin) types2.DeviceScanPlugin {
	return types2.DeviceScanPlugin{
		ID:            p.ID,
		Client:        p.Client,
		ProjectPath:   p.ProjectPath,
		ConfigPath:    p.ConfigPath,
		Name:          p.Name,
		PluginType:    p.PluginType,
		Version:       p.Version,
		Description:   p.Description,
		Author:        p.Author,
		Enabled:       p.Enabled,
		Marketplace:   p.Marketplace,
		Files:         []string(p.Files),
		HasMCPServers: p.HasMCPServers,
		HasSkills:     p.HasSkills,
		HasRules:      p.HasRules,
		HasCommands:   p.HasCommands,
		HasHooks:      p.HasHooks,
	}
}

// MCPServerStat is one row of the device-fleet MCP aggregation: every
// DeviceScanMCPServer with the same ConfigHash, observed in any
// device's latest scan within the requested time window, collapses
// into a single entity. Identity fields (Name, Transport, Command,
// URL, Args) are constant within a ConfigHash group by construction.
// Args is loaded post-hoc because JSONB has no MAX() in Postgres.
type MCPServerStat struct {
	ConfigHash       string                      `gorm:"column:config_hash"`
	Name             string                      `gorm:"column:name"`
	Transport        string                      `gorm:"column:transport"`
	Command          string                      `gorm:"column:command"`
	Args             datatypes.JSONSlice[string] `gorm:"-"`
	URL              string                      `gorm:"column:url"`
	DeviceCount      int64                       `gorm:"column:device_count"`
	UserCount        int64                       `gorm:"column:user_count"`
	ClientCount      int64                       `gorm:"column:client_count"`
	ObservationCount int64                       `gorm:"column:observation_count"`
}

// MCPServerDetail is the per-hash detail payload: an aggregated row
// plus the union of EnvKeys / HeaderKeys observed across every
// occurrence (those are deliberately excluded from the hash).
type MCPServerDetail struct {
	MCPServerStat
	EnvKeys    []string
	HeaderKeys []string
}

// ClientStat is one row of the per-client rollup.
type ClientStat struct {
	Name             string `gorm:"column:name"`
	DeviceCount      int64  `gorm:"column:device_count"`
	UserCount        int64  `gorm:"column:user_count"`
	ObservationCount int64  `gorm:"column:observation_count"`
}

// SkillStat is one row of the per-skill rollup.
type SkillStat struct {
	Name             string `gorm:"column:name"`
	DeviceCount      int64  `gorm:"column:device_count"`
	UserCount        int64  `gorm:"column:user_count"`
	ObservationCount int64  `gorm:"column:observation_count"`
}

// SkillDetail is the per-skill detail payload: an aggregated row plus
// representative metadata pulled from a single canonical row in the
// latest-scan-per-device subset. Description / HasScripts /
// GitRemoteURL / Files come from one observation and are not
// guaranteed to be stable across observations sharing the same name.
type SkillDetail struct {
	SkillStat
	Description  string
	HasScripts   bool
	GitRemoteURL string
	Files        []string
}

// MCPServerOccurrence is one device's latest-scan instance of a given
// ConfigHash.
type MCPServerOccurrence struct {
	DeviceScanID uint      `gorm:"column:device_scan_id"`
	DeviceID     string    `gorm:"column:device_id"`
	Client       string    `gorm:"column:client"`
	Scope        string    `gorm:"column:scope"`
	ScannedAt    time.Time `gorm:"column:scanned_at"`
	ID           uint      `gorm:"column:id"`
}

// SkillOccurrence is one device's latest-scan instance of a given
// skill name.
type SkillOccurrence struct {
	DeviceScanID uint      `gorm:"column:device_scan_id"`
	DeviceID     string    `gorm:"column:device_id"`
	Client       string    `gorm:"column:client"`
	Scope        string    `gorm:"column:scope"`
	ProjectPath  string    `gorm:"column:project_path"`
	ScannedAt    time.Time `gorm:"column:scanned_at"`
	ID           uint      `gorm:"column:id"`
}

// DeviceScanFromManifest builds a gateway DeviceScan + its children
// from a submission manifest. Caller is responsible for setting
// SubmittedBy on the returned struct before passing it to
// InsertDeviceScan.
func DeviceScanFromManifest(p types2.DeviceScanManifest) DeviceScan {
	s := DeviceScan{
		DeviceID:       p.DeviceID,
		Hostname:       p.Hostname,
		Username:       p.Username,
		OS:             p.OS,
		Arch:           p.Arch,
		ScannerVersion: p.ScannerVersion,
		ScannedAt:      p.ScannedAt.GetTime(),
	}
	if len(p.Files) > 0 {
		s.Files = make([]DeviceScanFile, len(p.Files))
		for i, f := range p.Files {
			s.Files[i] = DeviceScanFile{
				Path:      f.Path,
				SizeBytes: f.SizeBytes,
				Oversized: f.Oversized,
				Content:   f.Content,
			}
		}
	}
	if len(p.MCPServers) > 0 {
		s.MCPServers = make([]DeviceScanMCPServer, len(p.MCPServers))
		for i, m := range p.MCPServers {
			s.MCPServers[i] = DeviceScanMCPServer{
				Client:      m.Client,
				Scope:       deriveScope(m.ProjectPath),
				ProjectPath: m.ProjectPath,
				File:        m.File,
				Name:        m.Name,
				Transport:   m.Transport,
				Command:     m.Command,
				Args:        datatypes.JSONSlice[string](m.Args),
				URL:         m.URL,
				EnvKeys:     datatypes.JSONSlice[string](m.EnvKeys),
				HeaderKeys:  datatypes.JSONSlice[string](m.HeaderKeys),
				ConfigHash:  m.ConfigHash,
			}
		}
	}
	if len(p.Skills) > 0 {
		s.Skills = make([]DeviceScanSkill, len(p.Skills))
		for i, sk := range p.Skills {
			s.Skills[i] = DeviceScanSkill{
				Client:       sk.Client,
				Scope:        deriveScope(sk.ProjectPath),
				ProjectPath:  sk.ProjectPath,
				File:         sk.File,
				Name:         sk.Name,
				Description:  sk.Description,
				HasScripts:   sk.HasScripts,
				GitRemoteURL: sk.GitRemoteURL,
				Files:        datatypes.JSONSlice[string](sk.Files),
			}
		}
	}
	if len(p.Plugins) > 0 {
		s.Plugins = make([]DeviceScanPlugin, len(p.Plugins))
		for i, pl := range p.Plugins {
			s.Plugins[i] = DeviceScanPlugin{
				Client:        pl.Client,
				Scope:         deriveScope(pl.ProjectPath),
				ProjectPath:   pl.ProjectPath,
				ConfigPath:    pl.ConfigPath,
				Name:          pl.Name,
				PluginType:    pl.PluginType,
				Version:       pl.Version,
				Description:   pl.Description,
				Author:        pl.Author,
				Enabled:       pl.Enabled,
				Marketplace:   pl.Marketplace,
				HasMCPServers: pl.HasMCPServers,
				HasSkills:     pl.HasSkills,
				HasRules:      pl.HasRules,
				HasCommands:   pl.HasCommands,
				HasHooks:      pl.HasHooks,
				Files:         datatypes.JSONSlice[string](pl.Files),
			}
		}
	}
	if len(p.Clients) > 0 {
		s.Clients = make([]DeviceScanClient, len(p.Clients))
		for i, c := range p.Clients {
			s.Clients[i] = DeviceScanClient{
				Name:          c.Name,
				Version:       c.Version,
				BinaryPath:    c.BinaryPath,
				InstallPath:   c.InstallPath,
				ConfigPath:    c.ConfigPath,
				HasMCPServers: c.HasMCPServers,
				HasSkills:     c.HasSkills,
				HasPlugins:    c.HasPlugins,
			}
		}
	}
	return s
}

// deriveScope returns "global" when projectPath is empty, "project"
// otherwise. Persisted denormalized to keep list/aggregation queries
// off the projectPath column.
func deriveScope(projectPath string) string {
	if projectPath == "" {
		return "global"
	}
	return "project"
}
