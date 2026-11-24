package adminmcp

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/localagents/assets"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"sigs.k8s.io/yaml"
)

const (
	// skillName is the obot setup bootstrap skill that teaches agents to use this server.
	skillName = "obot-author-vmcps"
)

// readSkill returns the vMCP authoring skill as obot setup installs it.
func readSkill(t *testing.T) string {
	t.Helper()
	data := assets.SharedAgentsTemplateData()
	data.Admin = true
	rendered, err := assets.RenderAgentSkills(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range rendered {
		if asset.SkillName == skillName {
			return string(asset.Content)
		}
	}
	t.Fatalf("obot setup doesn't install the %s skill", skillName)
	return ""
}

// TestSkillExampleIsValid keeps the skill's example in step with the schema and the validator.
func TestSkillExampleIsValid(t *testing.T) {
	match := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindStringSubmatch(readSkill(t))
	if match == nil {
		t.Fatal("the skill has no YAML example")
	}
	example := match[1]

	data, err := mcpcatalog.CatalogItemSchema()
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	instanceJSON, err := yaml.YAMLToJSON([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(instanceJSON, &instance); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("the skill's example doesn't match the schema: %v", err)
	}

	tools := func(names ...string) []types.MCPServerTool {
		result := make([]types.MCPServerTool, 0, len(names))
		for _, name := range names {
			result = append(result, types.MCPServerTool{Name: name})
		}
		return result
	}
	sourceID := mcp.SourceIDForURL(testSourceURL)
	validator := &catalogValidator{
		catalogName: "default",
		sourceURL:   testSourceURL,
		mcpBackend:  mcp.RuntimeBackendKubernetes,
		entries: map[string]types.MCPServerCatalogEntryManifest{
			mcpcatalog.SourceRef(sourceID, "obot-outlook"): {
				Name:        "Outlook",
				EntryKey:    "obot-outlook",
				Runtime:     types.RuntimeRemote,
				ToolPreview: tools("list_emails", "send_draft", "delete_email"),
			},
			mcpcatalog.SourceRef(sourceID, "obot-calendar"): {
				Name:        "Calendar",
				EntryKey:    "obot-calendar",
				Runtime:     types.RuntimeRemote,
				ToolPreview: tools("list_events", "create_event", "delete_event"),
			},
		},
		existingGroups: func(context.Context, []string) (map[string]bool, error) { return nil, nil },
		existingUsers:  func(context.Context, []string) (map[string]bool, error) { return nil, nil },
	}
	diagnostics, err := validator.validate(t.Context(), []CatalogFile{{Path: "o365.yaml", Content: example}})
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severityError {
			t.Errorf("the skill's example fails validation: %s", diagnostic.Message)
		}
		// The skill tells agents to list allowedTools, so its example must too.
		if strings.Contains(diagnostic.Message, "grants every tool on component") {
			t.Errorf("the skill's example doesn't follow the skill: %s", diagnostic.Message)
		}
	}
}

// TestSkillCallShapesMatchTools keeps the skill's call shapes in step with the tools' arguments.
func TestSkillCallShapesMatchTools(t *testing.T) {
	skill := readSkill(t)
	session := connect(t, newTestServer(t, vmcpUser(types.GroupAdmin)).url, nil)
	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range result.Tools {
		shape := regexp.MustCompile("`" + tool.Name + `\(\{([^}]*(?:\}[^)]*)?)\}\)` + "`").FindStringSubmatch(skill)
		if shape == nil {
			t.Errorf("the skill has no call shape for %s", tool.Name)
			continue
		}
		var arguments []string
		for _, field := range regexp.MustCompile(`(\w+)(?::|,|$)`).FindAllStringSubmatch(strings.TrimSpace(shape[1]), -1) {
			arguments = append(arguments, field[1])
		}

		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		for _, required := range schema.Required {
			if !slices.Contains(arguments, required) {
				t.Errorf("the skill's call shape for %s lacks required argument %s", tool.Name, required)
			}
		}
	}
}
