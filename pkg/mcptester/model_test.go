package mcptester

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeModelResolver struct {
	allowed       bool
	accessModelID string
}

func (f *fakeModelResolver) UserHasAccessToModel(_ kuser.Info, modelID string) (bool, error) {
	f.accessModelID = modelID
	return f.allowed, nil
}

func TestResolveDefaultModelUsesOnlyLLMAlias(t *testing.T) {
	resolver := &fakeModelResolver{allowed: true}
	model := testModel(true, llmtypes.DialectOpenAIResponses)
	model.Spec.Manifest.TargetModel = "llm"
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(model).Build()

	_, err := ResolveDefaultModel(t.Context(), client, resolver, &kuser.DefaultInfo{UID: "user-1"})
	if err == nil {
		t.Fatal("ResolveDefaultModel() error = nil, want missing alias error")
	}
	if resolver.accessModelID != "" {
		t.Fatalf("access checked model %q without a configured alias", resolver.accessModelID)
	}
}

func TestResolveDefaultModelRequiresUserAccess(t *testing.T) {
	resolver := &fakeModelResolver{
		allowed: false,
	}
	model := testModel(true, llmtypes.DialectOpenAIResponses)
	client := testModelClient(model)

	_, err := ResolveDefaultModel(t.Context(), client, resolver, &kuser.DefaultInfo{UID: "user-1"})
	if err == nil {
		t.Fatal("ResolveDefaultModel() error = nil, want access error")
	}
	if resolver.accessModelID != model.Name {
		t.Fatalf("access checked model %q, want %q", resolver.accessModelID, model.Name)
	}
}

func TestResolveDefaultModelBuildsExistingProxyRoute(t *testing.T) {
	resolver := &fakeModelResolver{
		allowed: true,
	}
	model := testModel(true, llmtypes.DialectAnthropicMessages)
	model.Spec.Manifest.ModelProvider = system.AnthropicModelProvider
	client := testModelClient(model)

	got, err := ResolveDefaultModel(t.Context(), client, resolver, &kuser.DefaultInfo{UID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProxyPath != "anthropic" || got.APIPath != "/v1/messages" {
		t.Fatalf("proxy route = %q%q, want anthropic/v1/messages", got.ProxyPath, got.APIPath)
	}
}

func testModelClient(model *v1.Model) kclient.Client {
	return fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		model,
		&v1.DefaultModelAlias{
			Name:      string(types.DefaultModelAliasTypeLLM),
			Namespace: system.DefaultNamespace,
			Spec: v1.DefaultModelAliasSpec{
				Manifest: types.DefaultModelAliasManifest{
					Alias: string(types.DefaultModelAliasTypeLLM),
					Model: model.Name,
				},
			},
		},
	).Build()
}

func testModel(active bool, dialect llmtypes.Dialect) *v1.Model {
	return &v1.Model{
		Name:      "m1default",
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			TargetModel:   "model-target",
			ModelProvider: system.OpenAIModelProvider,
			Active:        active,
			Usage:         types.ModelUsageLLM,
			Dialect:       string(dialect),
		}},
	}
}
