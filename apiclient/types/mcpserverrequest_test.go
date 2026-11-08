package types

import (
	"strings"
	"testing"
)

func TestMCPServerRequestManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest MCPServerRequestManifest
		wantErr  bool
	}{
		{
			name:     "remote",
			manifest: MCPServerRequestManifest{Name: "Linear", Runtime: RuntimeRemote, RemoteURL: "https://mcp.linear.app/mcp"},
		},
		{
			name:     "remote without URL",
			manifest: MCPServerRequestManifest{Name: "Linear", Runtime: RuntimeRemote},
			wantErr:  true,
		},
		{
			name:     "remote with a non-HTTP URL",
			manifest: MCPServerRequestManifest{Name: "Linear", Runtime: RuntimeRemote, RemoteURL: "file:///etc/passwd"},
			wantErr:  true,
		},
		{
			name:     "npx",
			manifest: MCPServerRequestManifest{Name: "Filesystem", Runtime: RuntimeNPX, Package: "@modelcontextprotocol/server-filesystem"},
		},
		{
			name:     "uvx without package",
			manifest: MCPServerRequestManifest{Name: "Fetch", Runtime: RuntimeUVX},
			wantErr:  true,
		},
		{
			name:     "containerized without port",
			manifest: MCPServerRequestManifest{Name: "Thing", Runtime: RuntimeContainerized, Image: "ghcr.io/x/y:1", Path: "/mcp"},
			wantErr:  true,
		},
		{
			name:     "no name",
			manifest: MCPServerRequestManifest{Runtime: RuntimeNPX, Package: "x"},
			wantErr:  true,
		},
		{
			name:     "unsupported runtime",
			manifest: MCPServerRequestManifest{Name: "x", Runtime: RuntimeComposite},
			wantErr:  true,
		},
		{
			name:     "config with an unsupported usage",
			manifest: MCPServerRequestManifest{Name: "x", Runtime: RuntimeNPX, Package: "x", Config: []MCPServerRequestConfig{{Key: "K", Usage: File}}},
			wantErr:  true,
		},
		{
			name:     "existing catalog entry",
			manifest: MCPServerRequestManifest{Name: "Exa Search", ExistingCatalogEntryID: "default-exa-search-1"},
		},
		{
			name:     "config without key",
			manifest: MCPServerRequestManifest{Name: "x", Runtime: RuntimeNPX, Package: "x", Config: []MCPServerRequestConfig{{Description: "token"}}},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.manifest.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMCPServerRequestCatalogEntryManifest(t *testing.T) {
	m := MCPServerRequestManifest{
		Name:        "GitHub",
		Description: "GitHub's MCP server.\nMore detail.",
		SourceURL:   "https://github.com/github/github-mcp-server",
		Runtime:     RuntimeNPX,
		Package:     "@github/mcp",
		Args:        []string{"--read-only"},
		Config:      []MCPServerRequestConfig{{Key: "GITHUB_TOKEN", Description: "A personal access token", Sensitive: true}},
	}
	entry := m.CatalogEntryManifest()
	if entry.Runtime != RuntimeNPX || entry.NPXConfig == nil || entry.NPXConfig.Package != "@github/mcp" || len(entry.NPXConfig.Args) != 1 {
		t.Fatalf("unexpected runtime config: %+v", entry)
	}
	if entry.ShortDescription != "GitHub's MCP server." {
		t.Fatalf("ShortDescription = %q", entry.ShortDescription)
	}
	if entry.RepoURL != m.SourceURL {
		t.Fatalf("RepoURL = %q", entry.RepoURL)
	}
	if len(entry.Config) != 1 || entry.Config[0].Key != "GITHUB_TOKEN" || !entry.Config[0].Sensitive || !entry.Config[0].Required || entry.Config[0].Usage != Env {
		t.Fatalf("unexpected config: %+v", entry.Config)
	}
}

func TestMCPServerRequestApprovalValidate(t *testing.T) {
	tests := []struct {
		name     string
		approval MCPServerRequestApproval
		wantErr  bool
	}{
		{
			name:     "existing vMCP",
			approval: MCPServerRequestApproval{VMCPID: "vmcp1abc"},
		},
		{
			name:     "new vMCP",
			approval: MCPServerRequestApproval{NewVMCP: &MCPServerRequestNewVMCP{DisplayName: "Engineering"}},
		},
		{
			name:    "neither",
			wantErr: true,
		},
		{
			name:     "both",
			approval: MCPServerRequestApproval{VMCPID: "vmcp1abc", NewVMCP: &MCPServerRequestNewVMCP{DisplayName: "Engineering"}},
			wantErr:  true,
		},
		{
			name:     "new vMCP without a name",
			approval: MCPServerRequestApproval{NewVMCP: &MCPServerRequestNewVMCP{DisplayName: " "}},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.approval.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMCPServerRequestRemoteConfigUsesHeaders(t *testing.T) {
	entry := MCPServerRequestManifest{
		Name: "x", Runtime: RuntimeRemote, RemoteURL: "https://x.example/mcp",
		Config: []MCPServerRequestConfig{{Key: "Authorization"}},
	}.CatalogEntryManifest()
	if entry.Config[0].Usage != Header {
		t.Fatalf("Usage = %q, want header", entry.Config[0].Usage)
	}
	if err := entry.ValidateConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestFirstLineFitsShortDescription(t *testing.T) {
	long := strings.Repeat("Prisma manages Postgres databases, ", 10)
	got := firstLine(long + "\nsecond line")
	if n := len([]rune(got)); n > shortDescriptionLimit {
		t.Fatalf("short description has %d characters", n)
	}
	if !strings.HasSuffix(got, "…") || strings.Contains(got, "second") {
		t.Fatalf("unexpected short description %q", got)
	}
	if got := firstLine("  Short.  "); got != "Short." {
		t.Fatalf("got %q", got)
	}
	entry := MCPServerRequestManifest{Name: "x", Description: long, Runtime: RuntimeRemote, RemoteURL: "https://x.example/mcp"}.CatalogEntryManifest()
	if n := len([]rune(entry.ShortDescription)); n > shortDescriptionLimit {
		t.Fatalf("catalog short description has %d characters", n)
	}
}
