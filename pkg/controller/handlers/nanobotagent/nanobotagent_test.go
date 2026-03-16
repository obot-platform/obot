package nanobotagent

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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

	if model != "gpt-5.4" {
		t.Fatalf("expected gpt-5.4, got %q", model)
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

	if model != "model-a" {
		t.Fatalf("expected model-a, got %q", model)
	}
}
