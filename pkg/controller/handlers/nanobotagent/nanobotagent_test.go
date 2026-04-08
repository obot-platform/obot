package nanobotagent

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestChooseModelPrefersKnownNames(t *testing.T) {
	models := []v1.Model{
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "other",
					TargetModel: "some-other-model",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.TargetModel != "gpt-5.4" {
		t.Fatalf("expected gpt-5.4, got %q", model.TargetModel)
	}
}

func TestChooseModelFallsBackToFirstActiveModel(t *testing.T) {
	models := []v1.Model{
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "model-a",
					TargetModel: "model-a",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.TargetModel != "model-a" {
		t.Fatalf("expected model-a, got %q", model.TargetModel)
	}
}

func TestChooseModelPrefersSuggestedOrder(t *testing.T) {
	models := []v1.Model{
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "claude-sonnet-4-6",
					TargetModel: "claude-sonnet-4-6",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.TargetModel != "gpt-5.4" {
		t.Fatalf("expected gpt-5.4, got %q", model.TargetModel)
	}
}

func TestChooseModelMiniFallsBackToResolvedLLM(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.DefaultModelAlias{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "DefaultModelAlias",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "llm",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "llm",
						Model: "openai-gpt-5.4",
					},
				},
			},
			&v1.Model{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Model",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "openai-gpt-5.4",
				},
				Spec: v1.ModelSpec{
					Manifest: types.ModelManifest{
						Name:        "gpt-5.4",
						TargetModel: "gpt-5.4",
						Active:      true,
						Usage:       types.ModelUsageLLM,
					},
				},
			},
		).
		Build()

	models := []v1.Model{
		{
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), client, "", models, types.DefaultModelAliasTypeLLMMini)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.TargetModel != "gpt-5.4" {
		t.Fatalf("expected gpt-5.4, got %q", model.TargetModel)
	}
}
