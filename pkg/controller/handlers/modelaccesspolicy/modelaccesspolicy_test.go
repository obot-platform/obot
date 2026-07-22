package modelaccesspolicy

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPruneModels(t *testing.T) {
	existingModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "m1-existing",
			Namespace: "default",
		},
	}

	tests := []struct {
		name       string
		modelIDs   []string
		wantIDs    []string
		wantUpdate bool
	}{
		{
			name:       "keeps wildcard suffix pattern matching no models",
			modelIDs:   []string{"claude-haiku-4-5*"},
			wantIDs:    []string{"claude-haiku-4-5*"},
			wantUpdate: false,
		},
		{
			name:       "keeps pattern and existing model, prunes missing model",
			modelIDs:   []string{"claude-haiku-4-5*", "m1-existing", "m1-missing"},
			wantIDs:    []string{"claude-haiku-4-5*", "m1-existing"},
			wantUpdate: true,
		},
		{
			name:       "prunes invalid IDs including misplaced wildcards",
			modelIDs:   []string{"not-a-model", "a*b", "*haiku", "m1-existing"},
			wantIDs:    []string{"m1-existing"},
			wantUpdate: true,
		},
		{
			name:       "prunes duplicate patterns",
			modelIDs:   []string{"claude-haiku-4-5*", "claude-haiku-4-5*"},
			wantIDs:    []string{"claude-haiku-4-5*"},
			wantUpdate: true,
		},
		{
			name:       "wildcard collapses patterns and explicit references",
			modelIDs:   []string{"claude-haiku-4-5*", "*", "m1-existing"},
			wantIDs:    []string{"*"},
			wantUpdate: true,
		},
		{
			name:       "no update when nothing is pruned",
			modelIDs:   []string{"m1-existing", "obot://llm", "gpt-4o*"},
			wantIDs:    []string{"m1-existing", "obot://llm", "gpt-4o*"},
			wantUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models := make([]types.ModelResource, 0, len(tt.modelIDs))
			for _, id := range tt.modelIDs {
				models = append(models, types.ModelResource{ID: id})
			}

			client := fake.NewClientBuilder().
				WithScheme(storagescheme.Scheme).
				WithObjects(existingModel, &v1.ModelAccessPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "default",
					},
					Spec: v1.ModelAccessPolicySpec{
						Manifest: types.ModelAccessPolicyManifest{
							Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
							Models:   models,
						},
					},
				}).
				Build()

			// Fetch the stored policy so the handler updates it with a valid resource version
			var policy v1.ModelAccessPolicy
			key := kclient.ObjectKey{Namespace: "default", Name: "test-policy"}
			require.NoError(t, client.Get(t.Context(), key, &policy))

			initialVersion := policy.ResourceVersion
			err := PruneModels(router.Request{
				Client:    client,
				Ctx:       t.Context(),
				Object:    &policy,
				Namespace: policy.Namespace,
				Name:      policy.Name,
			}, nil)
			require.NoError(t, err)

			var updated v1.ModelAccessPolicy
			require.NoError(t, client.Get(t.Context(), key, &updated))

			gotIDs := make([]string, 0, len(updated.Spec.Manifest.Models))
			for _, m := range updated.Spec.Manifest.Models {
				gotIDs = append(gotIDs, m.ID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)

			if tt.wantUpdate {
				assert.NotEqual(t, initialVersion, updated.ResourceVersion, "expected policy to be updated")
			} else {
				assert.Equal(t, initialVersion, updated.ResourceVersion, "expected no update")
			}
		})
	}
}
