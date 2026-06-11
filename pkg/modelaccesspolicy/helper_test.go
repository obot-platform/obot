package modelaccesspolicy

import (
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

func TestResolveTargetModel(t *testing.T) {
	const provider = "openai-model-provider"

	tests := []struct {
		name         string
		models       []*v1.Model
		provider     string
		targetModel  string
		wantName     string
		wantNotFound bool
	}{
		{
			name:        "finds active model",
			models:      []*v1.Model{newModel("m1-openai-gpt-4o", provider, "gpt-4o", true)},
			provider:    provider,
			targetModel: "gpt-4o",
			wantName:    "m1-openai-gpt-4o",
		},
		{
			name:         "ignores model from a different provider",
			models:       []*v1.Model{newModel("m1-other-gpt-4o", "some-other-provider", "gpt-4o", true)},
			provider:     provider,
			targetModel:  "gpt-4o",
			wantNotFound: true,
		},
		{
			name:         "ignores inactive model",
			models:       []*v1.Model{newModel("m1-openai-gpt-4o-inactive", provider, "gpt-4o", false)},
			provider:     provider,
			targetModel:  "gpt-4o",
			wantNotFound: true,
		},
		{
			name: "most recently created wins on duplicate",
			models: []*v1.Model{
				newModel("m1-openai-gpt-4o-older", provider, "gpt-4o", true, withCreated(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))),
				newModel("m1-openai-gpt-4o-newer", provider, "gpt-4o", true, withCreated(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))),
			},
			provider:    provider,
			targetModel: "gpt-4o",
			wantName:    "m1-openai-gpt-4o-newer",
		},
		{
			name:         "no models configured",
			provider:     provider,
			targetModel:  "gpt-4o",
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newModelHelper(t, tt.models)

			got, err := h.ResolveTargetModel(tt.provider, tt.targetModel)
			if tt.wantNotFound {
				require.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
		})
	}
}

func TestGetUserAllowedTargetModels(t *testing.T) {
	const (
		provider = "openai-model-provider"
		userID   = "u1"
	)

	// userPolicy grants the given model IDs to userID.
	userPolicy := func(modelIDs ...string) *v1.ModelAccessPolicy {
		models := make([]types2.ModelResource, 0, len(modelIDs))
		for _, id := range modelIDs {
			models = append(models, types2.ModelResource{ID: id})
		}
		return &v1.ModelAccessPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "p-user", Namespace: "default"},
			Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
				Subjects: []types2.Subject{{Type: types2.SubjectTypeUser, ID: userID}},
				Models:   models,
			}},
		}
	}

	// wildcardPolicy grants every model to every user.
	wildcardPolicy := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p-wildcard", Namespace: "default"},
		Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
			Subjects: []types2.Subject{{Type: types2.SubjectTypeSelector, ID: "*"}},
			Models:   []types2.ModelResource{{ID: "*"}},
		}},
	}

	tests := []struct {
		name         string
		models       []*v1.Model
		policies     []*v1.ModelAccessPolicy
		want         map[string]bool
		wantAllowAll bool
	}{
		{
			name: "returns target models the user is allowed",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-mini", provider, "gpt-4o-mini", true),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("m1-gpt-4o")},
			want:     map[string]bool{"gpt-4o": true},
		},
		{
			name: "excludes inactive and other providers",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-inactive", provider, "gpt-4o-inactive", false),
				newModel("m1-other", "some-other-provider", "claude-sonnet-4-5", true),
			},
			// Grant every model name; only the active, same-provider one survives
			// because the provider index drops the others.
			policies: []*v1.ModelAccessPolicy{userPolicy("m1-gpt-4o", "m1-gpt-4o-inactive", "m1-other")},
			want:     map[string]bool{"gpt-4o": true},
		},
		{
			name: "wildcard policy reports allowAll without enumerating",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-mini", provider, "gpt-4o-mini", true),
			},
			policies:     []*v1.ModelAccessPolicy{wildcardPolicy},
			want:         nil,
			wantAllowAll: true,
		},
		{
			name: "empty when user is allowed nothing",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
			},
			want: map[string]bool{},
		},
		{
			name: "wildcard suffix pattern matches target models by prefix",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-mini", provider, "gpt-4o-mini", true),
				newModel("m1-gpt-5", provider, "gpt-5", true),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("gpt-4o*")},
			want:     map[string]bool{"gpt-4o": true, "gpt-4o-mini": true},
		},
		{
			name: "wildcard suffix pattern is case-sensitive",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("GPT-4o*")},
			want:     map[string]bool{},
		},
		{
			name: "wildcard suffix pattern matching nothing allows nothing",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("claude-haiku-4.5*")},
			want:     map[string]bool{},
		},
		{
			name: "wildcard suffix pattern combines with explicit model ID",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-5", provider, "gpt-5", true),
				newModel("m1-o3", provider, "o3", true),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("gpt-4o*", "m1-o3")},
			want:     map[string]bool{"gpt-4o": true, "o3": true},
		},
		{
			name: "wildcard suffix pattern matches inactive model but provider index drops it",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-mini", provider, "gpt-4o-mini", false),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("gpt-4o*")},
			want:     map[string]bool{"gpt-4o": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newModelHelper(t, tt.models, tt.policies...)

			got, allowAll, err := h.GetUserAllowedTargetModels(&kuser.DefaultInfo{Name: userID, UID: userID}, provider)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAllowAll, allowAll)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUserHasAccessToModelWithWildcardSuffix(t *testing.T) {
	const userID = "u1"

	models := []*v1.Model{
		newModel("m1-anthropic-claude-haiku", "anthropic-model-provider", "claude-haiku-4-5-20251001", true),
		newModel("m1-bedrock-claude-haiku", "bedrock-model-provider", "claude-haiku-4-5-20251001", true),
		newModel("m1-anthropic-claude-opus", "anthropic-model-provider", "claude-opus-4-8", true),
	}

	policy := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p-pattern", Namespace: "default"},
		Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
			Subjects: []types2.Subject{{Type: types2.SubjectTypeUser, ID: userID}},
			Models:   []types2.ModelResource{{ID: "claude-haiku-4-5*"}},
		}},
	}

	h := newModelHelper(t, models, policy)
	user := &kuser.DefaultInfo{Name: userID, UID: userID}

	for modelID, want := range map[string]bool{
		// The pattern grants matching models from every provider
		"m1-anthropic-claude-haiku": true,
		"m1-bedrock-claude-haiku":   true,
		"m1-anthropic-claude-opus":  false,
	} {
		got, err := h.UserHasAccessToModel(user, modelID)
		require.NoError(t, err)
		assert.Equal(t, want, got, "access for %s", modelID)
	}
}

// newModelHelper returns a Helper whose model and policy indexers are populated,
// mirroring the production indexes built in NewHelper.
func newModelHelper(t *testing.T, models []*v1.Model, policies ...*v1.ModelAccessPolicy) *Helper {
	t.Helper()

	modelIndexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		modelProviderIndex: modelProviderIndexFunc,
	})
	for _, m := range models {
		require.NoError(t, modelIndexer.Add(m))
	}

	mapIndexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		mapUserIndex:     mapSubjectIndexFunc(types2.SubjectTypeUser),
		mapGroupIndex:    mapSubjectIndexFunc(types2.SubjectTypeGroup),
		mapSelectorIndex: mapSubjectIndexFunc(types2.SubjectTypeSelector),
	})
	for _, p := range policies {
		require.NoError(t, mapIndexer.Add(p))
	}

	return &Helper{
		mapIndexer:   mapIndexer,
		dmaIndexer:   gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{dmaModelIndex: dmaModelIndexFunc}),
		modelIndexer: modelIndexer,
	}
}

type modelOpt func(*v1.Model)

func withCreated(ts time.Time) modelOpt {
	return func(m *v1.Model) {
		m.CreationTimestamp = metav1.NewTime(ts)
	}
}

func newModel(name, provider, targetModel string, active bool, opts ...modelOpt) *v1.Model {
	m := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			TargetModel:   targetModel,
			ModelProvider: provider,
			Active:        active,
		}},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
