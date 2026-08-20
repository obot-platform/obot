package model

import (
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoveApplyUpdateAnnotation(t *testing.T) {
	model := &v1.Model{
		Name:      "model",
		Namespace: "default",
		Annotations: map[string]string{
			apply.AnnotationUpdate: "false",
			"keep":                 "value",
		}}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(model).Build()

	if err := new(Handler).RemoveApplyUpdateAnnotation(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: model,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var updated v1.Model
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(model), &updated); err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.Annotations[apply.AnnotationUpdate]; ok {
		t.Fatal("apply update annotation was not removed")
	}
	if updated.Annotations["keep"] != "value" {
		t.Fatalf("unrelated annotation = %q, want value", updated.Annotations["keep"])
	}
}

func TestEnsureModelInfoUsesExactProviderAndTargetModel(t *testing.T) {
	model := &v1.Model{
		Name: "gpt-4o", Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: system.AzureEntraModelProvider,
			TargetModel:   "gpt-4o",
		}},
	}
	modelInfo := &v1.ModelInfo{
		Name:      v1.ModelInfoName(system.AzureEntraModelProvider, "gpt-4o"),
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelInfoSpec{
			Provider: system.AzureEntraModelProvider,
			Model:    "gpt-4o",
			Cost: types.ModelCost{TokenUsageCost: types.TokenUsageCost{
				Input: 2.5, Output: 10, CacheRead: 1.25,
			}},
		},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(modelInfo).Build()

	err := new(Handler).EnsureModelInfo(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: model,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if model.Status.Cost.Input != 2.5 || model.Status.Cost.Output != 10 || model.Status.Cost.CacheRead != 1.25 {
		t.Fatalf("model cost = %#v, want Azure Entra ModelInfo cost", model.Status.Cost)
	}
}
