package hostedagent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAgentConfigShape renders the file a sandbox is actually handed.
//
// It doubles as the documented example of the contract: run it with -v to see
// the full config. Every endpoint in it is absolute and carries its own
// credential, which is what lets an image boot from this file alone.
func TestAgentConfigShape(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: "m1sonnet", Namespace: "obot"},
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					Name:          "claude-sonnet",
					TargetModel:   "claude-sonnet-4-5",
					ModelProvider: system.AnthropicModelProvider,
					Active:        true,
					Usage:         types.ModelUsageLLM,
				}},
			},
			&v1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: "m1gpt", Namespace: "obot"},
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					Name:          "gpt",
					TargetModel:   "gpt-5",
					ModelProvider: system.OpenAIModelProvider,
					Active:        true,
					Usage:         types.ModelUsageLLM,
				}},
			},
			&v1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: "m1mini", Namespace: "obot"},
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					Name:          "gpt-mini",
					TargetModel:   "gpt-5-mini",
					ModelProvider: system.OpenAIModelProvider,
					Active:        true,
					Usage:         types.ModelUsageLLM,
				}},
			},
			&v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "obot"},
				Spec:       v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: "m1sonnet"}},
			},
			&v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{Name: "llm-mini", Namespace: "obot"},
				Spec:       v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm-mini", Model: "m1mini"}},
			},
		).
		Build()

	instance := &v1.HostedAgentInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "hai1g26fc", Namespace: "obot", UID: "d47613e8-aede-42f6-a599-9a641f99d8b7"},
		Spec: v1.HostedAgentInstanceSpec{
			UserID: "3",
			Manifest: types.HostedAgentInstanceManifest{
				Name:    "my-coding-agent",
				GitRepo: "https://github.com/example/service.git",
				GitRef:  "main",
				Answers: map[string]string{"instruction": "Follow the house style.", "token": "must-not-leak"},
			},
		},
	}
	agent := &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{
		Name:             "Claude Code",
		HarnessID:        "hrn1abc",
		AllowUserGitRepo: true,
		MCPServers:       []string{"ms1github", "ms1slack"},
		Models:           []string{"*"},
		Terminal:         true,
		Questions: []types.HostedAgentQuestion{
			{Key: "instruction"},
			{Key: "token", Sensitive: true},
		},
	}}}
	harness := &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{
		Image: "ghcr.io/obot-platform/agent-claude-code:latest", Interactive: true,
	}}}

	builder := defaultDesiredBuilder{ServerURL: "https://obot.example.com"}
	desired, err := builder.Build(context.Background(), BuildInput{
		Client:     client,
		Namespace:  "obot",
		Instance:   instance,
		Agent:      agent,
		Harness:    harness,
		Credential: "ok1-3-7-EXAMPLECREDENTIAL",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var raw, secretsRaw string
	for _, file := range desired.Files {
		if file.Path == agentConfigPath {
			raw = string(file.Content)
			if file.Mode != 0o444 {
				t.Errorf("config mode = %o, want 0444: it carries no secret", file.Mode)
			}
		}
	}
	for _, ref := range desired.Secrets {
		if ref.FilePath == agentSecretsPath {
			secretsRaw = ref.Value
		}
	}
	if raw == "" {
		t.Fatal("no config was delivered")
	}
	if secretsRaw == "" {
		t.Fatal("no secrets file was delivered")
	}

	// The whole point of the split: nothing in the readable config is a secret.
	if strings.Contains(raw, "EXAMPLECREDENTIAL") {
		t.Fatalf("a credential leaked into the world-readable config:\n%s", raw)
	}

	var pretty2 bytes.Buffer
	if err := json.Indent(&pretty2, []byte(secretsRaw), "", "  "); err != nil {
		t.Fatalf("secrets file is not valid JSON: %v", err)
	}
	t.Logf("%s\n%s", agentSecretsPath, pretty2.String())

	var secrets agentSecrets
	if err := json.Unmarshal([]byte(secretsRaw), &secrets); err != nil {
		t.Fatalf("unmarshal secrets: %v", err)
	}
	if secrets.MCPServers["ms1github"].Headers["Authorization"] != "Bearer ok1-3-7-EXAMPLECREDENTIAL" {
		t.Errorf("mcp credential missing: %+v", secrets.MCPServers)
	}
	if secrets.ModelProviders[system.AnthropicModelProvider].APIKey != "ok1-3-7-EXAMPLECREDENTIAL" {
		t.Errorf("model provider credential missing: %+v", secrets.ModelProviders)
	}
	// One entry per provider, not per model. Every model from a provider shares
	// an endpoint, so a per-model key stored the same string once per model --
	// 118 copies on this installation's catalogue.
	if len(secrets.ModelProviders) != 2 {
		t.Errorf("model provider entries = %d, want 2 (anthropic and openai): %+v",
			len(secrets.ModelProviders), secrets.ModelProviders)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(raw), "", "  "); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	t.Logf("%s\n%s", agentConfigPath, pretty.String())
	if dir := os.Getenv("WRITE_CONFIG_SAMPLE_DIR"); dir != "" {
		if err := os.WriteFile(dir+"/agent.json", pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/secrets.json", []byte(secretsRaw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var config agentConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Endpoints are complete: an image joins nothing together.
	if len(config.MCPServers) != 2 {
		t.Fatalf("mcpServers = %d, want 2", len(config.MCPServers))
	}
	for _, server := range config.MCPServers {
		if !strings.HasPrefix(server.URL, "https://obot.example.com/mcp-connect/") {
			t.Errorf("%s: url %q is not absolute", server.ID, server.URL)
		}
	}

	// Models are resolved to a wire protocol and a base URL, since an
	// Anthropic-native client and an OpenAI-compatible one are different
	// clients and nothing in the URL says which.
	// A "*" grant lists the whole catalogue, so the agent can choose rather
	// than being held to whatever the template author happened to name.
	if len(config.Models) != 3 {
		t.Fatalf("models = %d, want 3 (every model the agent was granted)", len(config.Models))
	}
	defaults := 0
	for _, model := range config.Models {
		if model.Default {
			defaults++
		}
	}
	// Exactly one: an image asking for "the default" must get an answer, and
	// must not have to choose between two.
	if defaults != 1 {
		t.Errorf("models marked default = %d, want 1: %+v", defaults, config.Models)
	}
	for _, model := range config.Models {
		if !strings.HasPrefix(model.BaseURL, "https://obot.example.com/api/llm-proxy/") {
			t.Errorf("%s: incomplete endpoint %+v", model.Model, model)
		}
	}

	if config.Source == nil || config.Source.Ref != "main" {
		t.Errorf("source = %+v", config.Source)
	}
	if config.Workspace != workspacePath {
		t.Errorf("workspace = %q", config.Workspace)
	}
	// A sensitive answer is collected from the user but never written into the
	// sandbox alongside everything else.
	if _, ok := config.Answers["token"]; ok {
		t.Error("a sensitive answer reached the sandbox config")
	}
	if config.Answers["instruction"] == "" {
		t.Error("a non-sensitive answer was dropped")
	}
}

// The config holds live credentials, so it must be redacted from the desired
// revision -- which is stored on the instance and visible wherever status is.
func TestCredentialsNeverReachTheRevision(t *testing.T) {
	builder := defaultDesiredBuilder{ServerURL: "https://obot.example.com"}
	desired, err := builder.Build(context.Background(), BuildInput{
		Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1", UID: "uid-1"}},
		Agent: &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{
			MCPServers: []string{"ms1github"},
		}}},
		Harness:    &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
		Credential: "ok1-3-7-EXAMPLECREDENTIAL",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	redacted, err := json.Marshal(desired.Redacted())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(redacted), "EXAMPLECREDENTIAL") {
		t.Fatalf("the credential survived redaction: %s", redacted)
	}
}

// A changed config has to restart the sandbox, or an agent granted a new MCP
// server keeps running without it.
func TestConfigChangeChangesTheRevision(t *testing.T) {
	build := func(servers ...string) string {
		t.Helper()
		desired, err := defaultDesiredBuilder{ServerURL: "https://obot.example.com"}.Build(context.Background(), BuildInput{
			Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1", UID: "uid-1"}},
			Agent:    &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{MCPServers: servers}}},
			Harness:  &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return desired.Revision
	}

	if build("ms1github") == build("ms1github", "ms1slack") {
		t.Fatal("adding an MCP server left the revision unchanged, so the sandbox would not restart")
	}
}

// An agent behind a path prefix has to know it. Obot strips the prefix before
// forwarding, so a server told nothing builds links and API calls against its
// own root -- which is not where anyone reaches it. A single-page UI ends up
// calling Obot instead of itself and reports that it cannot find its agent.
func TestPublishedAgentIsToldWhereItIsPublished(t *testing.T) {
	build := func(port int) agentConfig {
		t.Helper()
		desired, err := (defaultDesiredBuilder{ServerURL: "https://obot.example.com"}).Build(
			context.Background(), BuildInput{
				Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1qhtrt", UID: "uid-1"}},
				Agent:    &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{Port: port}}},
				Harness:  &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
			})
		if err != nil {
			t.Fatal(err)
		}
		var config agentConfig
		for _, file := range desired.Files {
			if file.Path == agentConfigPath {
				if err := json.Unmarshal(file.Content, &config); err != nil {
					t.Fatal(err)
				}
			}
		}
		return config
	}

	served := build(8000)
	// The resource ID, not the backend identifier: that is what the route is
	// keyed on.
	if served.PublicPath != "/agent-connect/hai1qhtrt" {
		t.Errorf("publicPath = %q", served.PublicPath)
	}
	if served.PublicURL != "https://obot.example.com/agent-connect/hai1qhtrt" {
		t.Errorf("publicURL = %q", served.PublicURL)
	}

	// An agent that serves nothing is published nowhere, and saying otherwise
	// would offer an address that 400s.
	quiet := build(0)
	if quiet.PublicPath != "" || quiet.PublicURL != "" {
		t.Errorf("a port-less agent should be published nowhere: %q %q", quiet.PublicPath, quiet.PublicURL)
	}
}

// The browser and the sandbox do not reach Obot at the same address, and the
// config carries both. Writing the public one into the sandbox's endpoints is
// what leaves an agent unable to reach its own models: the chart's egress
// policy excludes private ranges, so a public hostname resolving to an internal
// load balancer or a node is refused even though it resolves.
func TestSandboxEndpointsUseTheInternalAddress(t *testing.T) {
	const (
		public   = "https://obot.example.com"
		internal = "http://obot.obot.svc.cluster.local"
	)

	desired, err := (defaultDesiredBuilder{ServerURL: public, InternalURL: internal}).Build(
		context.Background(), BuildInput{
			Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1qhtrt", UID: "uid-1"}},
			Agent: &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{
				Port:       8000,
				MCPServers: []string{"ms1github"},
			}}},
			Harness: &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var config agentConfig
	for _, file := range desired.Files {
		if file.Path == agentConfigPath {
			if err := json.Unmarshal(file.Content, &config); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Called by the sandbox, so reached at the internal address.
	if config.ObotURL != internal {
		t.Errorf("obotURL = %q, want the internal address", config.ObotURL)
	}
	for _, server := range config.MCPServers {
		if !strings.HasPrefix(server.URL, internal) {
			t.Errorf("mcp server URL = %q, want the internal address", server.URL)
		}
	}

	// Followed by a browser, so it has to stay the public address -- the
	// internal one names a Service no browser can resolve.
	if config.PublicURL != public+"/agent-connect/hai1qhtrt" {
		t.Errorf("publicURL = %q, want the public address", config.PublicURL)
	}
}

// A caller that knows of only one address still has to produce a usable config,
// rather than one whose endpoints are rooted at the empty string.
func TestInternalAddressFallsBackToPublic(t *testing.T) {
	desired, err := (defaultDesiredBuilder{ServerURL: "https://obot.example.com"}).Build(
		context.Background(), BuildInput{
			Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1qhtrt", UID: "uid-1"}},
			Agent:    &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: types.HostedAgentManifest{MCPServers: []string{"ms1github"}}}},
			Harness:  &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
		})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var config agentConfig
	for _, file := range desired.Files {
		if file.Path == agentConfigPath {
			if err := json.Unmarshal(file.Content, &config); err != nil {
				t.Fatal(err)
			}
		}
	}

	if config.ObotURL != "https://obot.example.com" {
		t.Errorf("obotURL = %q, want the public address", config.ObotURL)
	}
	for _, server := range config.MCPServers {
		if !strings.HasPrefix(server.URL, "https://obot.example.com") {
			t.Errorf("mcp server URL = %q, want the public address", server.URL)
		}
	}
}
