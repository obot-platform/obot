package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hostedagentmodels"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAPIKeyAuthenticatorCarriesAuditAttribution(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "alice",
		ProviderUserID:   "alice",
		Email:            "alice@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "CLI token", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	response, ok, err := NewAPIKeyAuthenticator(client, nil).AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatal("expected API key to authenticate")
	}
	attribution, ok := principal.APIKeyAttributionFromUser(response.User)
	if !ok || attribution.ID != created.ID || attribution.Name != "CLI token" {
		t.Fatalf("attribution = %#v, %v; want key %d named CLI token", attribution, ok, created.ID)
	}
}

func TestAPIKeyAuthenticatorCarriesMaskedAttributionForUnnamedKey(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "alice-unnamed",
		ProviderUserID:   "alice-unnamed",
		Email:            "alice-unnamed@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	response, ok, err := NewAPIKeyAuthenticator(client, nil).AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatal("expected API key to authenticate")
	}
	attribution, ok := principal.APIKeyAttributionFromUser(response.User)
	wantName := fmt.Sprintf("ok1-%d-%d-*****", keyOwner.ID, created.ID)
	if !ok || attribution.ID != created.ID || attribution.Name != wantName {
		t.Fatalf("attribution = %#v, %v; want key %d named %q", attribution, ok, created.ID, wantName)
	}
}

// A hosted agent must never reach the Obot API. Live authorization decides
// which MCP servers it may use, but nothing narrows the API surface except the
// absence of this group, so an exfiltrated credential would otherwise be able
// to enumerate and modify Obot resources.
//
// GroupSkills is withheld for the same reason and is worth stating separately,
// because a sandbox plainly does use skills -- it just does not use the skills
// API to get them. They arrive as files, and the group would instead let the
// credential list, preview and download every skill in the installation, since
// nothing narrows a skill request to the ones an agent was configured with.
func TestHostedAgentPrincipalCarriesOnlyWhatItReaches(t *testing.T) {
	groups := hostedAgentGroups(true)

	for _, forbidden := range []string{types2.GroupAPI, types2.GroupSkills} {
		if slices.Contains(groups, forbidden) {
			t.Fatalf("a hosted agent principal must not carry %q: %v", forbidden, groups)
		}
	}
	for _, want := range []string{types2.GroupAuthenticated, types2.GroupLLM, types2.GroupMCP} {
		if !slices.Contains(groups, want) {
			t.Errorf("expected %q in %v", want, groups)
		}
	}
}

// An agent with no MCP servers should not present as an MCP client at all.
func TestHostedAgentWithoutServersOmitsMCPGroup(t *testing.T) {
	groups := hostedAgentGroups(false)

	if slices.Contains(groups, types2.GroupMCP) {
		t.Fatalf("expected no %q group when the agent has no servers: %v", types2.GroupMCP, groups)
	}
}

// An agent's model access is what its instance was configured with, not what
// its owner may currently reach.
func TestModelAllowedForAgent(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configured []string
		modelID    string
		want       bool
	}{
		{"exact match", []string{"m1-abc", "m1-def"}, "m1-abc", true},
		{"not configured", []string{"m1-abc"}, "m1-def", false},
		{"wildcard", []string{"*"}, "m1-anything", true},
		{"nothing configured denies", nil, "m1-abc", false},
		// Documented gap: alias references are not resolved yet, and denying is
		// the safe direction while that is true.
		{"alias reference is not expanded", []string{"obot://llm"}, "m1-abc", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelAllowedForAgent(tt.configured, tt.modelID); got != tt.want {
				t.Errorf("modelAllowedForAgent(%v, %q) = %v, want %v", tt.configured, tt.modelID, got, tt.want)
			}
		})
	}
}

func llmModel(name, target, provider string) *v1.Model {
	return &v1.Model{
		Name: name, Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			Name: name, TargetModel: target, ModelProvider: provider,
			Active: true, Usage: types2.ModelUsageLLM,
		}},
	}
}

// grant is what a sandbox credential ends up authorized for: the same
// resolution the controller uses to write the sandbox's model endpoints, so the
// two cannot disagree.
func grant(t *testing.T, client kclient.Client, selections, providers []string) []string {
	t.Helper()
	models, err := hostedagentmodels.Resolve(context.Background(), client, system.DefaultNamespace, selections, providers)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

// A template names models by alias, because an alias is the only reference that
// works on any installation. The proxy compares literally, so an unexpanded
// alias would deny an agent every model it was granted.
func TestAgentModelAliasesAreExpandedForAuthorization(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		llmModel("m1sonnet", "claude-sonnet-4-5", "anthropic-model-provider"),
		&v1.DefaultModelAlias{
			Name: "llm", Namespace: system.DefaultNamespace,
			Spec: v1.DefaultModelAliasSpec{Manifest: types2.DefaultModelAliasManifest{Alias: "llm", Model: "m1sonnet"}},
		},
	).Build()

	got := grant(t, client, []string{types2.DefaultModelAliasRefPrefix + "llm"}, nil)

	if !slices.Contains(got, "m1sonnet") {
		t.Errorf("the alias did not expand to its model: %v", got)
	}
	if slices.Contains(got, types2.DefaultModelAliasRefPrefix+"llm") {
		t.Errorf("the unexpanded alias survived and would match nothing: %v", got)
	}
	// The end-to-end property: the proxy's own check now admits the model.
	if !modelAllowedForAgent(got, "m1sonnet") {
		t.Error("the agent is still denied the model its alias points at")
	}
}

// An alias bound to nothing grants nothing, and must not be carried through as
// a literal that could never match anyway.
func TestUnboundAliasGrantsNothing(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.DefaultModelAlias{
			Name: "llm", Namespace: system.DefaultNamespace,
			Spec: v1.DefaultModelAliasSpec{Manifest: types2.DefaultModelAliasManifest{Alias: "llm"}},
		},
	).Build()

	if got := grant(t, client, []string{types2.DefaultModelAliasRefPrefix + "llm"}, nil); len(got) != 0 {
		t.Fatalf("an unbound alias should grant nothing, got %v", got)
	}
}

// The grant must be concrete. A literal "*" reaching the principal would be
// read by the proxy as "every model", which is not what a template restricted
// to one provider offers -- and the credential can call the proxy directly,
// so the configuration omitting a model is no restriction at all.
func TestWildcardGrantIsConcreteAndProviderRestricted(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		llmModel("m1sonnet", "claude-sonnet-4-5", "anthropic-model-provider"),
		llmModel("m1gpt", "gpt-5", "openai-model-provider"),
	).Build()

	got := grant(t, client, []string{"*"}, []string{"anthropic-model-provider"})

	if slices.Contains(got, "*") {
		t.Fatalf("the wildcard reached the grant verbatim, authorizing every model: %v", got)
	}
	if !slices.Contains(got, "m1sonnet") {
		t.Errorf("the allowed provider's model is missing: %v", got)
	}
	if slices.Contains(got, "m1gpt") {
		t.Errorf("a model from an excluded provider was granted: %v", got)
	}
	if modelAllowedForAgent(got, "m1gpt") {
		t.Error("the proxy would admit a model the template's providers exclude")
	}
}

// An explicitly named model from an excluded provider is excluded too --
// naming it is a selection, not an override of the provider restriction.
func TestExplicitModelFromExcludedProviderIsNotGranted(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		llmModel("m1sonnet", "claude-sonnet-4-5", "anthropic-model-provider"),
		llmModel("m1gpt", "gpt-5", "openai-model-provider"),
	).Build()

	got := grant(t, client, []string{"m1sonnet", "m1gpt"}, []string{"anthropic-model-provider"})

	if slices.Contains(got, "m1gpt") {
		t.Errorf("an explicitly selected model from an excluded provider was granted: %v", got)
	}
	if !slices.Contains(got, "m1sonnet") {
		t.Errorf("the allowed model is missing: %v", got)
	}
}
