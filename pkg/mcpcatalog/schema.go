package mcpcatalog

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/obot-platform/obot/apiclient/types"
)

const (
	// CatalogItemSchemaURI is the MCP resource URI of the catalog item schema.
	CatalogItemSchemaURI = "obot://schemas/catalog-item.json"
)

var (
	catalogItemSchema = sync.OnceValues(buildCatalogItemSchema)
)

// CatalogVMCPItem is the catalog source form of a vMCP. Field order is the
// order in generated YAML.
type CatalogVMCPItem struct {
	Type        string                 `json:"type" jsonschema:"Must be vmcp."`
	EntryKey    string                 `json:"entryKey" jsonschema:"Stable key that identifies this vMCP within its catalog source. Changing it creates a different vMCP."`
	DisplayName string                 `json:"displayName" jsonschema:"Name shown to users. Required."`
	Description string                 `json:"description,omitempty"`
	Icon        string                 `json:"icon,omitempty" jsonschema:"Icon URL."`
	ToolSearch  bool                   `json:"toolSearch,omitempty" jsonschema:"Expose a search tool instead of listing every tool."`
	Components  []CatalogVMCPComponent `json:"components" jsonschema:"The catalog entries this vMCP combines."`
	// Profiles is never omitted: sync replaces a missing list with an admin-only default.
	Profiles []types.VMCPProfile `json:"profiles" jsonschema:"Who can use this vMCP and which tools they get. Profiles are additive and there is no deny. If omitted, only admins can use the vMCP, so always write it; an empty list grants no one."`
}

type CatalogVMCPComponent struct {
	// ID is always set so sync keeps the configuration and credentials scoped to it.
	ID                       string                          `json:"id" jsonschema:"Author-chosen stable id, unique within the vMCP, without periods. Profiles refer to it. Must not change once merged."`
	Name                     string                          `json:"name" jsonschema:"Component name, unique within the vMCP. Defaults to the entry name."`
	MCPServerCatalogEntryKey string                          `json:"mcpServerCatalogEntryKey" jsonschema:"The referenced entry: its entryKey when it is in the same catalog source, or sourceID::entryKey for another source of the same catalog, where sourceID is the source URL without its scheme or trailing slash."`
	Configuration            []types.VMCPConfigurationPolicy `json:"configuration,omitempty" jsonschema:"Policy for each configuration key of the entry. Required keys must be fixed, with a value or a secretBinding. Never put a sensitive value in the file; use a secretBinding."`
	ToolPrefix               string                          `json:"toolPrefix,omitempty" jsonschema:"Prefix added to this component's tool names."`
	ToolOverrides            []types.ToolOverride            `json:"toolOverrides,omitempty" jsonschema:"Renames, descriptions and enablement for individual upstream tools. When set, only enabled tools can be granted."`
	ForceSingleUser          bool                            `json:"forceSingleUser,omitempty" jsonschema:"Run a separate server for each user even if the entry supports multiple users."`
}

// catalogEntryItem is the catalog source form of an MCP server catalog entry.
type catalogEntryItem struct {
	Type string `json:"type,omitempty" jsonschema:"entry, or omitted."`
	types.MCPServerCatalogEntryManifest
}

// CatalogItemSchema returns the JSON Schema for a catalog source file: a
// vMCP or catalog entry item, or a list of them.
func CatalogItemSchema() ([]byte, error) {
	return catalogItemSchema()
}

func buildCatalogItemSchema() ([]byte, error) {
	vmcp, err := jsonschema.For[CatalogVMCPItem](nil)
	if err != nil {
		return nil, err
	}
	if err := describeVMCP(vmcp); err != nil {
		return nil, err
	}

	entry, err := jsonschema.For[catalogEntryItem](&jsonschema.ForOptions{IgnoreInvalidTypes: true})
	if err != nil {
		return nil, err
	}
	entry.Description = "A catalog entry: an MCP server definition that vMCP components reference by entryKey."
	// Entries may carry fields for other Obot versions, which sync skips.
	entry.AdditionalProperties = nil
	entry.Properties["type"].Enum = []any{"entry"}

	vmcpType := any("vmcp")
	item := &jsonschema.Schema{
		If: &jsonschema.Schema{
			Properties: map[string]*jsonschema.Schema{"type": {Const: &vmcpType}},
			Required:   []string{"type"},
		},
		Then: &jsonschema.Schema{Ref: "#/$defs/vmcp"},
		Else: &jsonschema.Schema{Ref: "#/$defs/entry"},
	}
	root := &jsonschema.Schema{
		Schema:      "https://json-schema.org/draft/2020-12/schema",
		Title:       "Obot catalog source file",
		Description: "A catalog source file holds one item or a list of items. An item with type: vmcp is a vMCP; any other item is a catalog entry.",
		OneOf: []*jsonschema.Schema{
			{Ref: "#/$defs/item"},
			{Type: "array", Items: &jsonschema.Schema{Ref: "#/$defs/item"}},
		},
		Defs: map[string]*jsonschema.Schema{
			"item":  item,
			"vmcp":  vmcp,
			"entry": entry,
		},
	}
	return json.MarshalIndent(root, "", "  ")
}

func describeVMCP(s *jsonschema.Schema) error {
	vmcpType := any("vmcp")
	s.Description = "A vMCP: one MCP endpoint that combines tools from catalog entries and grants them to users through profiles."
	s.Properties["type"].Const = &vmcpType
	// Sync accepts a vMCP without entryKey or profiles, and the validator only warns, so editors mustn't
	// call them errors either. Their descriptions say to write them.
	s.Required = slices.DeleteFunc(s.Required, func(name string) bool { return name == "entryKey" || name == "profiles" })
	// Components may omit name; sync uses the entry name.
	s.Properties["components"].Items.Required = slices.DeleteFunc(s.Properties["components"].Items.Required, func(name string) bool { return name == "name" })

	profile, err := schemaAt(s, "profiles", "[]")
	if err != nil {
		return err
	}
	profile.Properties["name"].Description = "Profile name, unique within the vMCP."
	profile.Properties["subjects"].Description = "Who the profile applies to. At least one."
	profile.Properties["subjects"].MinItems = new(1)

	subject := profile.Properties["subjects"].Items
	subject.Properties["type"].Enum = []any{
		string(types.SubjectTypeGroup),
		string(types.SubjectTypeObotGroup),
		string(types.SubjectTypeUser),
		string(types.SubjectTypeSelector),
	}
	subject.Properties["type"].Description = "group: an auth provider group id, the same on every Obot instance using that provider. obotGroup: an Obot role. user: a user id, which is specific to one Obot instance, so prefer groups. selector: everyone, with id *."
	subject.Properties["id"].Description = "The subject id. Look up group and user ids with the search_groups and find_user tools. For obotGroup, use power-user or power-user-plus only when asked for them by name; they may be deprecated."
	subject.AllOf = []*jsonschema.Schema{
		subjectIDs(types.SubjectTypeObotGroup, types.GroupOwner, types.GroupAdmin, types.GroupAuditor, types.GroupPowerUserPlus, types.GroupPowerUser, types.GroupBasic, types.GroupAuthenticated),
		subjectIDs(types.SubjectTypeSelector, "*"),
	}

	permissions := profile.Properties["vmcpPermissions"]
	permissions.Description = "The tools the profile grants. The key is vmcpPermissions, not permissions."
	permissions.Properties["allowAllComponents"].Description = "Grant every enabled tool of every component."
	allowed := permissions.Properties["allowedComponents"]
	allowed.Description = "Tools granted per component, keyed by component id (not name)."
	allowed.PropertyNames = &jsonschema.Schema{Type: "string", Pattern: `^[^.]+$`}
	componentSet := allowed.AdditionalProperties
	if componentSet == nil {
		return fmt.Errorf("allowedComponents has no value schema")
	}
	componentSet.Required = nil
	componentSet.Properties["allowedTools"].Description = "Upstream tool names, before toolPrefix and toolOverrides rename them. Omitted grants all tools; an empty list grants none."

	policy, err := schemaAt(s, "components", "[]", "configuration", "[]")
	if err != nil {
		return err
	}
	policy.Properties["policy"].Enum = []any{
		string(types.VMCPConfigurationPolicyProhibited),
		string(types.VMCPConfigurationPolicyFixed),
		string(types.VMCPConfigurationPolicyUserAllowed),
	}
	policy.Properties["policy"].Description = "prohibited (the default): users can't set it. fixed: set here by value or secretBinding. userAllowed: each user sets it."
	policy.Properties["value"].Description = "Fixed value. Only with policy fixed. Never use for sensitive values; use secretBinding."
	return nil
}

// subjectIDs restricts the ids of subjects of subjectType to ids.
func subjectIDs(subjectType types.SubjectType, ids ...string) *jsonschema.Schema {
	typeValue := any(string(subjectType))
	enum := make([]any, 0, len(ids))
	for _, id := range ids {
		enum = append(enum, id)
	}
	return &jsonschema.Schema{
		If: &jsonschema.Schema{
			Properties: map[string]*jsonschema.Schema{"type": {Const: &typeValue}},
			Required:   []string{"type"},
		},
		Then: &jsonschema.Schema{
			Properties: map[string]*jsonschema.Schema{"id": {Enum: enum}},
		},
	}
}

// schemaAt returns the schema at path, where "[]" steps into array items.
func schemaAt(s *jsonschema.Schema, path ...string) (*jsonschema.Schema, error) {
	for _, step := range path {
		if step == "[]" {
			s = s.Items
		} else {
			s = s.Properties[step]
		}
		if s == nil {
			return nil, fmt.Errorf("catalog schema has no %v", path)
		}
	}
	return s, nil
}
