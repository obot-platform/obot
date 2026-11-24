package mcpcatalog

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/obot-platform/obot/apiclient/types"
	"sigs.k8s.io/yaml"
)

func TestCatalogItemSchema(t *testing.T) {
	data, err := CatalogItemSchema()
	if err != nil {
		t.Fatalf("CatalogItemSchema() error = %v", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "vmcp with profiles",
			yaml: `
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
    name: Slack
    mcpServerCatalogEntryKey: catalogs.example.com/acme::slack
profiles:
  - name: support
    subjects:
      - type: group
        id: provider/support
      - type: obotGroup
        id: admin
    vmcpPermissions:
      allowedComponents:
        github:
          allowedTools: [search_issues]
        slack: {}
`,
		},
		{
			name: "vmcp with empty profiles",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components: []
profiles: []
`,
		},
		{
			name: "list of items",
			yaml: `
- type: vmcp
  entryKey: support
  displayName: Support
  components: []
  profiles: []
- name: GitHub
  entryKey: github
  runtime: remote
  shortDescription: GitHub
  description: GitHub
  icon: ""
  futureField: kept for other Obot versions
`,
		},
		{
			name: "permissions instead of vmcpPermissions",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components: []
profiles:
  - name: support
    subjects:
      - type: selector
        id: "*"
    permissions:
      allowAllComponents: true
`,
			wantErr: true,
		},
		{
			name: "unknown obotGroup",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components: []
profiles:
  - name: support
    subjects:
      - type: obotGroup
        id: admins
`,
			wantErr: true,
		},
		{
			name: "selector other than star",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components: []
profiles:
  - name: support
    subjects:
      - type: selector
        id: everyone
`,
			wantErr: true,
		},
		{
			name: "profile without subjects",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components: []
profiles:
  - name: support
    subjects: []
`,
			wantErr: true,
		},
		{
			// Sync accepts both missing; the validator only warns.
			name: "missing profiles and entryKey",
			yaml: `
type: vmcp
displayName: Support
components: []
`,
		},
		{
			name: "missing displayName",
			yaml: `
type: vmcp
entryKey: support
components: []
profiles: []
`,
			wantErr: true,
		},
		{
			name: "component id with period",
			yaml: `
type: vmcp
entryKey: support
displayName: Support
components:
  - id: git.hub
    mcpServerCatalogEntryKey: github
profiles:
  - name: support
    subjects:
      - type: selector
        id: "*"
    vmcpPermissions:
      allowedComponents:
        git.hub: {}
`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.YAMLToJSON([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("YAMLToJSON() error = %v", err)
			}
			var instance any
			if err := json.Unmarshal(data, &instance); err != nil {
				t.Fatalf("unmarshal instance: %v", err)
			}
			err = resolved.Validate(instance)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCatalogVMCPMirrorsSyncTypes keeps the schema's vMCP types in step with the types sync decodes
// a vMCP into: a field sync reads but the schema lacks would be flagged in every editor.
func TestCatalogVMCPMirrorsSyncTypes(t *testing.T) {
	tests := []struct {
		name   string
		sync   reflect.Type
		mirror reflect.Type
		// renamed maps sync's field names to the catalog file's.
		renamed map[string]string
		// serverOnly are fields Obot fills in, which catalog files don't set.
		serverOnly []string
	}{
		{
			name:   "vMCP",
			sync:   reflect.TypeFor[types.VMCPManifest](),
			mirror: reflect.TypeFor[CatalogVMCPItem](),
		},
		{
			name:       "component",
			sync:       reflect.TypeFor[types.VMCPComponent](),
			mirror:     reflect.TypeFor[CatalogVMCPComponent](),
			renamed:    map[string]string{"mcpServerCatalogEntryID": "mcpServerCatalogEntryKey"},
			serverOnly: []string{"mcpCatalogID", "catalogEntry", "sourceDigest", "oauthCredentialID"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mirrored := map[string]bool{}
			for _, name := range jsonFieldNames(tt.mirror) {
				mirrored[name] = true
			}
			for _, name := range jsonFieldNames(tt.sync) {
				if slices.Contains(tt.serverOnly, name) {
					continue
				}
				if renamed, ok := tt.renamed[name]; ok {
					name = renamed
				}
				if !mirrored[name] {
					t.Errorf("%s field %q is read by sync but missing from %s", tt.sync, name, tt.mirror)
				}
			}
		})
	}
}

func jsonFieldNames(typ reflect.Type) []string {
	var names []string
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}
