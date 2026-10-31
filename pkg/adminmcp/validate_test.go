package adminmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/version"
	"github.com/stretchr/testify/assert"
)

const (
	testSourceURL = "https://github.com/acme/catalog"

	validVMCP = `
type: vmcp
entryKey: support
displayName: Support
components:
  - id: github
    mcpServerCatalogEntryKey: github
    configuration:
      - key: TOKEN
        policy: fixed
        secretBinding:
          name: github
          key: token
  - id: slack
    mcpServerCatalogEntryKey: github.com/acme/other::slack
profiles:
  - name: support
    subjects:
      - type: group
        id: provider/support
    vmcpPermissions:
      allowedComponents:
        github:
          allowedTools: [search_issues]
        slack:
          allowedTools: []
`
)

func newTestValidator() *catalogValidator {
	sourceID := mcp.SourceIDForURL(testSourceURL)
	return &catalogValidator{
		catalogName: "default",
		sourceURL:   testSourceURL,
		mcpBackend:  mcp.RuntimeBackendKubernetes,
		entries: map[string]types.MCPServerCatalogEntryManifest{
			mcpcatalog.SourceRef(sourceID, "github"): {
				Name:     "GitHub",
				EntryKey: "github",
				Runtime:  types.RuntimeRemote,
				Config: []types.MCPConfig{
					{Key: "TOKEN", Name: "Token", Usage: types.Header, Required: true, Sensitive: true},
					{Key: "ORG", Name: "Org", Usage: types.Header},
				},
				ToolPreview: []types.MCPServerTool{
					{Name: "search_issues"},
					{Name: "create_issue"},
				},
			},
			mcpcatalog.SourceRef("github.com/acme/other", "slack"): {
				Name:     "Slack",
				EntryKey: "slack",
				Runtime:  types.RuntimeRemote,
			},
		},
		secretTargets: map[string][]string{"github": {"token"}},
		existingGroups: func(_ context.Context, ids []string) (map[string]bool, error) {
			existing := map[string]bool{}
			for _, id := range ids {
				existing[id] = id == "provider/support"
			}
			return existing, nil
		},
		existingUsers: func(_ context.Context, ids []string) (map[string]bool, error) {
			existing := map[string]bool{}
			for _, id := range ids {
				existing[id] = id == "42"
			}
			return existing, nil
		},
	}
}

func TestValidateCatalogFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []CatalogFile
		// obotVersion is the running Obot version, for entries limited to some versions.
		obotVersion  string
		wantErrors   []string
		wantWarnings []string
	}{
		{
			name:  "valid vMCP",
			files: []CatalogFile{{Path: "support.yaml", Content: validVMCP}},
		},
		{
			name: "unknown entry reference",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.Replace(validVMCP, "mcpServerCatalogEntryKey: github\n", "mcpServerCatalogEntryKey: gitlab\n", 1),
			}},
			wantErrors: []string{`component "github" references "gitlab", but catalog "default" has no entry with entryKey "gitlab"`},
		},
		{
			name: "entry defined in the same change",
			files: []CatalogFile{
				{
					Path:    "support.yaml",
					Content: strings.Replace(validVMCP, "mcpServerCatalogEntryKey: github\n", "mcpServerCatalogEntryKey: gitlab\n", 1),
				},
				{
					Path: "gitlab.yaml",
					Content: `
name: GitLab
entryKey: gitlab
shortDescription: GitLab
description: GitLab
icon: ""
runtime: npx
npxConfig:
  package: "@acme/gitlab-mcp"
toolPreview:
  - name: search_issues
config:
  - key: TOKEN
    name: Token
    usage: env
    required: true
    sensitive: true
`,
				},
			},
		},
		{
			name: "unknown tool",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.Replace(validVMCP, "allowedTools: [search_issues]", "allowedTools: [delete_repo]", 1),
			}},
			wantErrors: []string{`profile "support" names tool "delete_repo", which component "github" doesn't have`},
		},
		{
			name: "sensitive value in the file",
			files: []CatalogFile{{
				Path: "support.yaml",
				Content: strings.Replace(validVMCP, `        secretBinding:
          name: github
          key: token`, "        value: ghp_secret", 1),
			}},
			wantErrors: []string{`component "github" configuration "TOKEN" is sensitive`},
		},
		{
			name: "required configuration not fixed",
			files: []CatalogFile{{
				Path: "support.yaml",
				Content: strings.Replace(validVMCP, `        policy: fixed
        secretBinding:
          name: github
          key: token`, "        policy: userAllowed", 1),
			}},
		},
		{
			name: "required configuration prohibited",
			files: []CatalogFile{{
				Path: "support.yaml",
				Content: strings.Replace(validVMCP, `    configuration:
      - key: TOKEN
        policy: fixed
        secretBinding:
          name: github
          key: token
`, "", 1),
			}},
			wantErrors: []string{`required configuration "TOKEN" cannot be prohibited`},
		},
		{
			name: "unknown secret",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.Replace(validVMCP, "name: github\n", "name: gh\n", 1),
			}},
			wantErrors: []string{`binds Secret "gh", which is not a bindable Secret`},
		},
		{
			name: "unknown group and user",
			files: []CatalogFile{{
				Path: "support.yaml",
				Content: strings.Replace(validVMCP, "        id: provider/support\n", `        id: provider/sales
      - type: user
        id: "7"
`, 1),
			}},
			wantErrors: []string{
				`profile "support" user "7" doesn't exist`,
				`profile "support" group "provider/sales" doesn't exist`,
			},
			wantWarnings: []string{`profile "support" grants user "7"`},
		},
		{
			name: "broad grants and missing profiles",
			files: []CatalogFile{
				{
					Path: "everyone.yaml",
					Content: strings.Replace(validVMCP, `      - type: group
        id: provider/support
`, `      - type: selector
        id: "*"
`, 1),
				},
				{
					Path: "admins.yaml",
					Content: `
type: vmcp
entryKey: admins
displayName: Admins
components: []
`,
				},
			},
			wantWarnings: []string{
				`profile "support" grants every user`,
				`vMCP "Admins" has no profiles`,
			},
		},
		{
			name: "duplicate vMCP entry keys",
			files: []CatalogFile{
				{Path: "a.yaml", Content: validVMCP},
				{Path: "b.yaml", Content: validVMCP},
			},
			wantErrors: []string{`duplicate vMCP entry key "support" also used by a.yaml`},
		},
		{
			name: "tools of an entry without a tool list",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.Replace(validVMCP, "allowedTools: []", "allowedTools: [post_message]", 1),
			}},
			wantWarnings: []string{`component "slack" tool names can't be checked`},
		},
		{
			name: "entry this change limits to other Obot versions",
			files: []CatalogFile{
				{Path: "support.yaml", Content: validVMCP},
				{
					Path: "github.yaml",
					Content: `
name: GitHub
entryKey: github
minObotVersion: v99.0.0
shortDescription: GitHub
description: GitHub
icon: ""
runtime: npx
npxConfig:
  package: "@acme/github-mcp"
`,
				},
			},
			obotVersion:  "v0.20.0",
			wantErrors:   []string{`component "github" references "github", which this change limits to other Obot versions`},
			wantWarnings: []string{`entry "GitHub" is limited to Obot v99.0.0 and later`},
		},
		{
			name: "profile grants every tool on a component",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.Replace(validVMCP, "        slack:\n          allowedTools: []\n", "        slack: {}\n", 1),
			}},
			wantWarnings: []string{`profile "support" grants every tool on component "slack", including tools enabled on it later`},
		},
		{
			// A file cut off after a component's key grants that component every tool.
			name: "truncated profile",
			files: []CatalogFile{{
				Path:    "support.yaml",
				Content: strings.TrimSuffix(validVMCP, "          allowedTools: []\n"),
			}},
			wantWarnings: []string{`profile "support" grants every tool on component "slack"`},
		},
		{
			name:       "invalid YAML",
			files:      []CatalogFile{{Path: "bad.yaml", Content: "type: [vmcp"}},
			wantErrors: []string{"invalid catalog file"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.obotVersion != "" {
				originalTag := version.Tag
				version.Tag = tt.obotVersion
				t.Cleanup(func() { version.Tag = originalTag })
			}
			diagnostics, err := newTestValidator().validate(t.Context(), tt.files)
			if err != nil {
				t.Fatalf("validate() error = %v", err)
			}
			var gotErrors, gotWarnings []string
			for _, diagnostic := range diagnostics {
				if diagnostic.Severity == severityError {
					gotErrors = append(gotErrors, diagnostic.Message)
				} else {
					gotWarnings = append(gotWarnings, diagnostic.Message)
				}
			}
			assertMessages(t, "errors", gotErrors, tt.wantErrors)
			assertMessages(t, "warnings", gotWarnings, tt.wantWarnings)
		})
	}
}

// assertMessages checks that each message starts with or contains the wanted text, in order.
func assertMessages(t *testing.T, kind string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d %s %q, want %d %q", len(got), kind, got, len(want), want)
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("%s[%d] = %q, want it to contain %q", kind, i, got[i], want[i])
		}
	}
}

func TestValidateCatalogFilesReportsAccess(t *testing.T) {
	content := strings.Replace(validVMCP, "  - id: slack\n", `    toolOverrides:
      - name: search_issues
        enabled: true
      - name: create_issue
  - id: slack
`, 1)
	content = strings.Replace(content, "profiles:\n", `profiles:
  - name: everything
    subjects:
      - type: obotGroup
        id: admin
    vmcpPermissions:
      allowAllComponents: true
`, 1)
	validator := newTestValidator()
	diagnostics, err := validator.validate(t.Context(), []CatalogFile{{Path: "support.yaml", Content: content}})
	if err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	if count := errorCount(diagnostics); count != 0 {
		t.Fatalf("validate() errors = %v", diagnostics)
	}

	want := []VMCPAccess{{
		File: "support.yaml",
		VMCP: "Support",
		Profiles: []ProfileAccess{
			{
				// create_issue isn't enabled on the component, and Slack's entry has no tool list.
				Profile: "everything",
				Components: []ComponentAccess{
					{Component: "github", Tools: []string{"search_issues"}, AllTools: true},
					{Component: "slack", AllTools: true, Unknown: true},
				},
			},
			{
				Profile: "support",
				Components: []ComponentAccess{
					{Component: "github", Tools: []string{"search_issues"}},
					{Component: "slack", Tools: []string{}},
				},
			},
		},
	}}
	assert.Equal(t, want, validator.access)
}
