package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func availabilityFor(t *testing.T, objs ...kclient.Object) *agentAvailability {
	t.Helper()
	client := fakeclient.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objs...).Build()
	availability, err := newAgentAvailability(context.Background(), client, "obot")
	if err != nil {
		t.Fatalf("newAgentAvailability: %v", err)
	}
	return availability
}

func harnessObj(name string) *v1.Harness {
	return &v1.Harness{
		Name: name, Namespace: "obot",
		Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Name: name, Image: "img"}},
	}
}

func modelObj(name, provider string) *v1.Model {
	return &v1.Model{
		Name: name, Namespace: "obot",
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name: name, TargetModel: name, ModelProvider: provider,
			Active: true, Usage: types.ModelUsageLLM,
		}},
	}
}

// The case this exists for: Claude Code is written against Anthropic, so on an
// installation with only OpenAI providers it can never work. Offering it
// produces a sandbox that fails at start-up with an error the user cannot act
// on. The template names the provider, and the reason names it back.
func TestAgentNeedingAnAbsentProviderIsUnavailable(t *testing.T) {
	availability := availabilityFor(t,
		harnessObj("hrn1claude"),
		modelObj("m1gpt", system.OpenAIModelProvider),
	)

	reasons := availability.reasons(context.Background(), types.HostedAgentManifest{
		Name: "Claude Code", HarnessID: "hrn1claude",
		ModelProviders: []string{system.AnthropicModelProvider}, Models: []string{"*"},
	})
	if len(reasons) != 1 || !strings.Contains(reasons[0], system.AnthropicModelProvider) {
		t.Fatalf("reasons = %v, want one naming the missing provider", reasons)
	}
}

// The same agent is fine as soon as the provider exists, without the template
// changing -- which is why this is computed rather than stored.
func TestAgentIsAvailableOnceItsProviderExists(t *testing.T) {
	availability := availabilityFor(t,
		harnessObj("hrn1claude"),
		modelObj("m1sonnet", system.AnthropicModelProvider),
	)

	if reasons := availability.reasons(context.Background(), types.HostedAgentManifest{
		HarnessID:      "hrn1claude",
		ModelProviders: []string{system.AnthropicModelProvider}, Models: []string{"*"},
	}); len(reasons) != 0 {
		t.Fatalf("reasons = %v, want none", reasons)
	}
}

// An agent naming no provider takes whatever is configured, and is restricted
// by nothing.
func TestAgentWithoutDeclaredProvidersAcceptsAnyModel(t *testing.T) {
	availability := availabilityFor(t,
		harnessObj("hrn1any"),
		modelObj("m1gpt", system.OpenAIModelProvider),
	)

	if reasons := availability.reasons(context.Background(), types.HostedAgentManifest{
		HarnessID: "hrn1any", Models: []string{"*"},
	}); len(reasons) != 0 {
		t.Fatalf("reasons = %v, want none", reasons)
	}
}

// An agent naming an MCP server that was never installed cannot work either.
func TestMissingMCPServerMakesAnAgentUnavailable(t *testing.T) {
	availability := availabilityFor(t,
		harnessObj("hrn1any"),
		modelObj("m1gpt", system.OpenAIModelProvider),
		&v1.MCPServer{Name: "ms1here", Namespace: "obot"},
	)

	reasons := availability.reasons(context.Background(), types.HostedAgentManifest{
		HarnessID: "hrn1any", Models: []string{"*"}, MCPServers: []string{"ms1here", "ms1gone"},
	})
	if len(reasons) != 1 || !strings.Contains(reasons[0], "ms1gone") {
		t.Fatalf("reasons = %v, want one naming the missing server", reasons)
	}
	if strings.Contains(reasons[0], "ms1here") {
		t.Errorf("a server that is installed should not be reported missing: %v", reasons)
	}
}

// Every reason at once, so an administrator fixes the set rather than
// discovering the next one after each fix.
func TestAllReasonsAreReportedTogether(t *testing.T) {
	availability := availabilityFor(t,
		harnessObj("hrn1claude"),
		modelObj("m1gpt", system.OpenAIModelProvider),
	)

	reasons := availability.reasons(context.Background(), types.HostedAgentManifest{
		HarnessID: "hrn1claude", Models: []string{"*"},
		ModelProviders: []string{system.AnthropicModelProvider}, MCPServers: []string{"ms1gone"},
	})
	if len(reasons) != 2 {
		t.Fatalf("reasons = %v, want both the model and the MCP server", reasons)
	}
}

// An agent whose harness was removed cannot run, and saying so is more useful
// than reporting a missing model on a harness that is not there.
func TestUnregisteredHarnessIsItsOwnReason(t *testing.T) {
	availability := availabilityFor(t, modelObj("m1gpt", system.OpenAIModelProvider))

	reasons := availability.reasons(context.Background(), types.HostedAgentManifest{HarnessID: "hrn1gone"})
	if len(reasons) != 1 || !strings.Contains(reasons[0], "harness") {
		t.Fatalf("reasons = %v", reasons)
	}
}
