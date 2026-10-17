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

type modelOpt func(*v1.Model)

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

			got, err := h.resolveTargetModel(tt.provider, tt.targetModel)
			if tt.wantNotFound {
				require.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
		})
	}
}

func TestResolveUpstreamModelName(t *testing.T) {
	const provider = "llm-gateway-model-provider"
	older := withCreated(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	newer := withCreated(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	models := []*v1.Model{
		newModel("m-openai-gpt-4o-mini", provider, "openai/gpt-4o-mini", true, older),
		// Imported after the OpenAI provider, so it is the newest model with this ID.
		newModel("m-azure-gpt-4o-mini", provider, "azure/gpt-4o-mini", true, newer),
		newModel("m-anthropic-sonnet-4-5", provider, "anthropic/claude-sonnet-4-5-20250929", true, older),
		newModel("m-anthropic-opus-4", provider, "anthropic/claude-opus-4-20250514", true, older),
		newModel("m-anthropic-opus-4-1", provider, "anthropic/claude-opus-4-1-20250805", true, older),
		newModel("m-anthropic-haiku-old", provider, "anthropic/claude-haiku-4-5-20250101", true, older),
		newModel("m-anthropic-haiku", provider, "anthropic/claude-haiku-4-5-20251001", true, older),
		newModel("m-azure-sonnet-4-5", provider, "azure/claude-sonnet-4-5", true, newer),
		newModel("m-anthropic-opus-4-5", provider, "anthropic/claude-opus-4-5", true, older),
		newModel("m-anthropic-opus-4-5-dated", provider, "anthropic/claude-opus-4-5-20251101", true, newer),
		newModel("m-custom-llama", provider, "custom/llama-3", true, older),
		newModel("m-other-llama", provider, "other/llama-3", true, newer),
		newModel("m-anthropic-inactive", provider, "anthropic/claude-inactive-20250101", false, older),
	}
	h := newModelHelper(t, models)

	tests := []struct {
		name         string
		reference    string
		usable       func(*v1.Model) bool
		wantName     string
		wantNotFound bool
	}{
		{
			name:      "bare ID prefers the provider that made the model",
			reference: "gpt-4o-mini",
			wantName:  "m-openai-gpt-4o-mini",
		},
		{
			name:      "bare ID prefers a model the caller may use",
			reference: "gpt-4o-mini",
			usable:    func(m *v1.Model) bool { return m.Name == "m-azure-gpt-4o-mini" },
			wantName:  "m-azure-gpt-4o-mini",
		},
		{
			name:      "bare ID of hosted models picks the most recently created",
			reference: "llama-3",
			wantName:  "m-other-llama",
		},
		{
			name:      "undated alias resolves to the latest dated version",
			reference: "claude-haiku-4-5",
			wantName:  "m-anthropic-haiku",
		},
		{
			name:      "undated alias with its provider",
			reference: "anthropic/claude-haiku-4-5",
			wantName:  "m-anthropic-haiku",
		},
		{
			name:      "the maker's dated version wins over a host's exact ID",
			reference: "claude-sonnet-4-5",
			wantName:  "m-anthropic-sonnet-4-5",
		},
		{
			name:      "a host's exact ID when the caller may only use it",
			reference: "claude-sonnet-4-5",
			usable:    func(m *v1.Model) bool { return m.Name == "m-azure-sonnet-4-5" },
			wantName:  "m-azure-sonnet-4-5",
		},
		{
			name:      "an exact ID's provider must match a qualified name",
			reference: "anthropic/claude-sonnet-4-5",
			wantName:  "m-anthropic-sonnet-4-5",
		},
		{
			name:      "an exact ID wins over a dated version",
			reference: "claude-opus-4-5",
			wantName:  "m-anthropic-opus-4-5",
		},
		{
			name:      "an alias doesn't match a longer model name",
			reference: "claude-opus-4",
			wantName:  "m-anthropic-opus-4",
		},
		{
			name:         "a qualified name only matches its provider",
			reference:    "openai/claude-haiku-4-5",
			wantNotFound: true,
		},
		{
			name:         "a prefix that isn't followed by a date doesn't match",
			reference:    "claude",
			wantNotFound: true,
		},
		{
			name:         "inactive models don't match",
			reference:    "claude-inactive",
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.ResolveUpstreamModelName(provider, tt.reference, tt.usable)
			if tt.wantNotFound {
				require.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v (%v)", err, got)
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
			Name: "p-user", Namespace: "default",
			Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
				Subjects: []types2.Subject{{Type: types2.SubjectTypeUser, ID: userID}},
				Models:   models,
			}},
		}
	}

	// wildcardPolicy grants every eligible model to every user.
	wildcardPolicy := &v1.ModelAccessPolicy{
		Name: "p-wildcard", Namespace: "default",
		Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
			Subjects: []types2.Subject{{Type: types2.SubjectTypeSelector, ID: "*"}},
			Models:   []types2.ModelResource{{ID: "*"}},
		}},
	}

	tests := []struct {
		name         string
		models       []*v1.Model
		policies     []*v1.ModelAccessPolicy
		dialect      string
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
			name: "wildcard policy enumerates only eligible model usages",
			models: []*v1.Model{
				newModel("m1-gpt-4o", provider, "gpt-4o", true),
				newModel("m1-gpt-4o-mini", provider, "gpt-4o-mini", true),
				newModel(
					"m1-text-embedding",
					provider,
					"text-embedding-3-small",
					true,
					withUsage(types2.ModelUsageEmbedding),
				),
			},
			policies: []*v1.ModelAccessPolicy{wildcardPolicy},
			want: map[string]bool{
				"gpt-4o":      true,
				"gpt-4o-mini": true,
			},
		},
		{
			name: "filters allowed models by dialect",
			models: []*v1.Model{
				newModel("m1-openai", provider, "openai.gpt-5.5", true, withDialect("OpenAIResponses")),
				newModel("m1-anthropic", provider, "anthropic.claude-haiku-4-5", true, withDialect("AnthropicMessages")),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("m1-openai", "m1-anthropic")},
			dialect:  "AnthropicMessages",
			want:     map[string]bool{"anthropic.claude-haiku-4-5": true},
		},
		{
			name: "wildcard policy enumerates models when filtering by dialect",
			models: []*v1.Model{
				newModel("m1-openai", provider, "openai.gpt-5.5", true, withDialect("OpenAIResponses")),
				newModel("m1-anthropic", provider, "anthropic.claude-haiku-4-5", true, withDialect("AnthropicMessages")),
			},
			policies: []*v1.ModelAccessPolicy{wildcardPolicy},
			dialect:  "OpenAIResponses",
			want:     map[string]bool{"openai.gpt-5.5": true},
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
				newModel(
					"m1-gpt-4o-embedding",
					provider,
					"gpt-4o-embedding",
					true,
					withUsage(types2.ModelUsageEmbedding),
				),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("gpt-4o*")},
			want:     map[string]bool{"gpt-4o": true, "gpt-4o-mini": true},
		},
		{
			name: "legacy explicit non-llm model grants no access",
			models: []*v1.Model{
				newModel(
					"m1-text-embedding",
					provider,
					"text-embedding-3-small",
					true,
					withUsage(types2.ModelUsageEmbedding),
				),
			},
			policies: []*v1.ModelAccessPolicy{userPolicy("m1-text-embedding")},
			want:     map[string]bool{},
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
			policies: []*v1.ModelAccessPolicy{userPolicy("claude-haiku-4-5*")},
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

			got, allowAll, err := h.GetUserAllowedTargetModels(&kuser.DefaultInfo{Name: userID, UID: userID}, provider, tt.dialect)
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
		Name: "p-pattern", Namespace: "default",
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

func TestObotGroupSubjectGrantsModelAccess(t *testing.T) {
	const modelID = "m1-openai-gpt-4o"
	h := newModelHelper(t,
		[]*v1.Model{newModel(modelID, "openai-model-provider", "gpt-4o", true)},
		&v1.ModelAccessPolicy{
			Name:      "admin",
			Namespace: "default",
			Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
				Subjects: []types2.Subject{{Type: types2.SubjectTypeObotGroup, ID: "admin"}},
				Models:   []types2.ModelResource{{ID: modelID}},
			}},
		},
	)
	admin := &kuser.DefaultInfo{UID: "user1", Extra: map[string][]string{"obot_groups": {"admin"}}}
	got, err := h.UserHasAccessToModel(admin, modelID)
	require.NoError(t, err)
	assert.True(t, got)

	idp := &kuser.DefaultInfo{UID: "user1", Extra: map[string][]string{"auth_provider_groups": {"admin"}}}
	got, err = h.UserHasAccessToModel(idp, modelID)
	require.NoError(t, err)
	assert.False(t, got)
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
		mapUserIndex:      mapSubjectIndexFunc(types2.SubjectTypeUser),
		mapGroupIndex:     mapSubjectIndexFunc(types2.SubjectTypeGroup),
		mapObotGroupIndex: mapSubjectIndexFunc(types2.SubjectTypeObotGroup),
		mapSelectorIndex:  mapSubjectIndexFunc(types2.SubjectTypeSelector),
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

func withCreated(ts time.Time) modelOpt {
	return func(m *v1.Model) {
		m.CreationTimestamp = metav1.NewTime(ts)
	}
}

func withDialect(dialect string) modelOpt {
	return func(m *v1.Model) {
		m.Spec.Manifest.Dialect = dialect
	}
}

func withUsage(usage types2.ModelUsage) modelOpt {
	return func(m *v1.Model) {
		m.Spec.Manifest.Usage = usage
	}
}

func newModel(name, provider, targetModel string, active bool, opts ...modelOpt) *v1.Model {
	m := &v1.Model{
		Name:      name,
		Namespace: "default",
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			TargetModel:   targetModel,
			ModelProvider: provider,
			Active:        active,
			Usage:         types2.ModelUsageLLM,
		}},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
