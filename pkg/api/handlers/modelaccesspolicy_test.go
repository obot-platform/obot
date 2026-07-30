package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadAndValidateModelAccessPolicyManifest(t *testing.T) {
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			modelForAccessPolicyHandlerTest("m1-llm", types.ModelUsageLLM),
			modelForAccessPolicyHandlerTest("m1-embedding", types.ModelUsageEmbedding),
		).
		Build())

	tests := []struct {
		name     string
		modelIDs []string
		wantErr  string
	}{
		{
			name:     "allows llm model and aliases",
			modelIDs: []string{"m1-llm", "obot://llm", "obot://llm-mini"},
		},
		{
			name:     "rejects non-llm model",
			modelIDs: []string{"m1-embedding"},
			wantErr:  `model "m1-embedding" must have a usage type of "llm"`,
		},
		{
			name:     "rejects non-llm alias",
			modelIDs: []string{"obot://vision"},
			wantErr:  `model "obot://vision" must reference default model alias "llm" or "llm-mini"`,
		},
		{
			name:     "allows selectors",
			modelIDs: []string{"model-prefix*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := modelAccessPolicyManifestForHandlerTest(tt.modelIDs...)
			body, err := json.Marshal(manifest)
			require.NoError(t, err)

			req := api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/model-access-policies", bytes.NewReader(body)),
				Storage:        storageClient,
			}
			got, err := readAndValidateModelAccessPolicyManifest(req)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, manifest, got)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestModelAccessPolicyHandlerUpdateRejectsPersistedNonLLMModel(t *testing.T) {
	const policyID = "map1-policy"

	initialManifest := modelAccessPolicyManifestForHandlerTest("m1-embedding")
	initialManifest.DisplayName = "Existing Policy"
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			modelForAccessPolicyHandlerTest("m1-embedding", types.ModelUsageEmbedding),
			&v1.ModelAccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyID,
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.ModelAccessPolicySpec{
					Manifest: initialManifest,
				},
			},
		).
		Build())

	updatedManifest := initialManifest
	updatedManifest.DisplayName = "Updated Policy"
	body, err := json.Marshal(updatedManifest)
	require.NoError(t, err)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/model-access-policies/"+policyID,
		bytes.NewReader(body),
	)
	request.SetPathValue("id", policyID)
	err = new(ModelAccessPolicyHandler).Update(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storageClient,
	})
	require.ErrorContains(t, err, `model "m1-embedding" must have a usage type of "llm"`)

	var policy v1.ModelAccessPolicy
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      policyID,
	}, &policy))
	require.Equal(t, initialManifest, policy.Spec.Manifest)
}

func modelForAccessPolicyHandlerTest(name string, usage types.ModelUsage) *v1.Model {
	return &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ModelSpec{
			Manifest: types.ModelManifest{
				Usage: usage,
			},
		},
	}
}

func modelAccessPolicyManifestForHandlerTest(modelIDs ...string) types.ModelAccessPolicyManifest {
	models := make([]types.ModelResource, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, types.ModelResource{ID: modelID})
	}
	return types.ModelAccessPolicyManifest{
		Subjects: []types.Subject{{
			Type: types.SubjectTypeUser,
			ID:   "user",
		}},
		Models: models,
	}
}
