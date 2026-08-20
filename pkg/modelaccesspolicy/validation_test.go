package modelaccesspolicy

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateModelResource(t *testing.T) {
	reader := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			modelForValidationTest("m1-llm", types.ModelUsageLLM),
			modelForValidationTest("m1-llm-mini", types.ModelUsage("llm-mini")),
			modelForValidationTest("m1-embedding", types.ModelUsageEmbedding),
		).
		Build())

	tests := []struct {
		name        string
		resource    types.ModelResource
		wantInvalid bool
		wantErr     string
	}{
		{
			name:     "allows llm model",
			resource: types.ModelResource{ID: "m1-llm"},
		},
		{
			name:        "rejects llm-mini model usage",
			resource:    types.ModelResource{ID: "m1-llm-mini"},
			wantInvalid: true,
			wantErr:     `model "m1-llm-mini" must have a usage type of "llm"`,
		},
		{
			name:        "rejects embedding model",
			resource:    types.ModelResource{ID: "m1-embedding"},
			wantInvalid: true,
			wantErr:     `model "m1-embedding" must have a usage type of "llm"`,
		},
		{
			name:     "allows llm alias",
			resource: types.ModelResource{ID: "obot://llm"},
		},
		{
			name:     "allows llm-mini alias",
			resource: types.ModelResource{ID: "obot://llm-mini"},
		},
		{
			name:        "rejects embedding alias",
			resource:    types.ModelResource{ID: "obot://text-embedding"},
			wantInvalid: true,
			wantErr:     `model "obot://text-embedding" must reference default model alias "llm" or "llm-mini"`,
		},
		{
			name:     "allows wildcard",
			resource: types.ModelResource{ID: "*"},
		},
		{
			name:     "allows wildcard suffix",
			resource: types.ModelResource{ID: "claude-*"},
		},
		{
			name:        "rejects malformed model ID",
			resource:    types.ModelResource{ID: "not-a-model-id"},
			wantInvalid: true,
			wantErr:     `model "not-a-model-id" must reference a valid model ID`,
		},
		{
			name:     "returns missing model error",
			resource: types.ModelResource{ID: "m1-missing"},
			wantErr:  `failed to get model "m1-missing"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModelResource(
				t.Context(),
				reader,
				system.DefaultNamespace,
				tt.resource,
			)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantErr)
			require.Equal(t, tt.wantInvalid, IsInvalidModelResource(err))
		})
	}
}

func modelForValidationTest(name string, usage types.ModelUsage) *v1.Model {
	return &v1.Model{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{
			Manifest: types.ModelManifest{
				Usage: usage,
			},
		},
	}
}
