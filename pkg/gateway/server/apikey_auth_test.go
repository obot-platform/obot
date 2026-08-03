package server

import (
	"context"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// A hosted agent must never reach the Obot API. Live authorization decides
// which MCP servers it may use, but nothing narrows the API surface except the
// absence of this group, so an exfiltrated credential would otherwise be able
// to enumerate and modify Obot resources.
func TestHostedAgentPrincipalNeverCarriesAPIGroup(t *testing.T) {
	groups := hostedAgentGroups(true)

	if slices.Contains(groups, types2.GroupAPI) {
		t.Fatalf("a hosted agent principal must not carry %q: %v", types2.GroupAPI, groups)
	}
	if !slices.Contains(groups, types2.GroupHostedAgent) {
		t.Errorf("expected %q, got %v", types2.GroupHostedAgent, groups)
	}
	for _, want := range []string{types2.GroupAuthenticated, types2.GroupLLM, types2.GroupSkills, types2.GroupMCP} {
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

// An agent configured with an alias must reach the model that alias points at.
//
// authorized_model_ids is compared literally against the model being requested,
// so an unexpanded "obot://llm" matches nothing: an agent granted a model
// through an alias -- which is how a template works on any installation, since
// no specific model ID exists everywhere -- would be denied every request.
func TestAgentModelAliasesAreExpandedForAuthorization(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "m1sonnet", Namespace: system.DefaultNamespace},
			Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
				Name: "m1sonnet", TargetModel: "claude-sonnet-4-5", Active: true, Usage: types2.ModelUsageLLM,
			}},
		},
		&v1.DefaultModelAlias{
			ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: system.DefaultNamespace},
			Spec:       v1.DefaultModelAliasSpec{Manifest: types2.DefaultModelAliasManifest{Alias: "llm", Model: "m1sonnet"}},
		},
	).Build()

	authenticator := &APIKeyAuthenticator{storage: client}
	got := authenticator.resolveAgentModelIDs(context.Background(),
		[]string{types2.DefaultModelAliasRefPrefix + "llm", "m1explicit", "*"})

	if !slices.Contains(got, "m1sonnet") {
		t.Errorf("the alias did not expand to its model: %v", got)
	}
	if slices.Contains(got, types2.DefaultModelAliasRefPrefix+"llm") {
		t.Errorf("the unexpanded alias survived and would match nothing: %v", got)
	}
	// Everything else passes through: "*" is understood by the proxy directly,
	// and a concrete ID needs no resolution.
	for _, want := range []string{"m1explicit", "*"} {
		if !slices.Contains(got, want) {
			t.Errorf("expected %q to pass through: %v", want, got)
		}
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
			ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: system.DefaultNamespace},
			Spec:       v1.DefaultModelAliasSpec{Manifest: types2.DefaultModelAliasManifest{Alias: "llm"}},
		},
	).Build()

	authenticator := &APIKeyAuthenticator{storage: client}
	got := authenticator.resolveAgentModelIDs(context.Background(), []string{types2.DefaultModelAliasRefPrefix + "llm"})
	if len(got) != 0 {
		t.Fatalf("an unbound alias should grant nothing, got %v", got)
	}
}
