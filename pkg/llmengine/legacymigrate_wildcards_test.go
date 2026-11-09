package llmengine

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestMigrationNativeSlashWildcards(t *testing.T) {
	for _, tc := range []struct {
		name            string
		pattern         string
		native          string
		gatewayProvider string
		unverified      bool
		undiscovered    bool
		wantMigrated    bool
		wantQualified   bool
	}{
		{
			name:            "native Qwen names",
			pattern:         "Qwen/Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
		},
		{
			name:            "ambiguous existing gateway grant",
			pattern:         "anthropic/claude*",
			native:          "anthropic/claude-sonnet",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
		},
		{
			name:            "multiple native slashes",
			pattern:         "org/team/Qwen*",
			native:          "org/team/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
		},
		{
			name:            "gateway grant without matching native source",
			pattern:         "anthropic/claude*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
		},
		{
			name:            "unverified source",
			pattern:         "Qwen/Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			unverified:      true,
		},
		{
			name:            "undiscovered target",
			pattern:         "Qwen/Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			undiscovered:    true,
		},
		{
			name:            "unrelated target provider",
			pattern:         "Qwen/Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "other",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			ns := system.DefaultNamespace
			original := types.ModelResource{ID: tc.pattern}
			policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
				Models:   []types.ModelResource{original},
			}}}
			require.NoError(t, policy.Spec.Manifest.Validate())
			existing := &v1.Model{Namespace: ns, Name: "m1-existing", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
				ModelProvider: system.LLMGatewayModelProvider,
				TargetModel:   "anthropic/claude-sonnet",
			}}}
			objects := []kclient.Object{policy, existing}
			discovered := map[string]struct{}{}
			wantMappings := map[string]string{}
			// Two matching models must yield one provider form.
			for _, suffix := range []string{"", "-extra"} {
				legacy := &v1.Model{Namespace: ns, Name: "m1-old" + suffix, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.GenericResponsesModelProvider,
					TargetModel:   tc.native + suffix,
				}}}
				target := &v1.Model{Namespace: ns, Name: "m1-new" + suffix, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   tc.gatewayProvider + "/" + tc.native + suffix,
				}}}
				objects = append(objects, legacy, target)
				if !tc.undiscovered {
					discovered[target.Spec.Manifest.TargetModel] = struct{}{}
				}
				if tc.wantMigrated {
					wantMappings[legacy.Name] = target.Name
				}
			}
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
			sources := map[string]string{}
			if !tc.unverified {
				sources[system.GenericResponsesModelProvider] = "generic-responses"
			}
			saved := map[string]string{}
			save := func(mapping map[string]string) error {
				maps.Copy(saved, mapping)
				return nil
			}
			require.NoError(t, migrateLegacyModels(ctx, c, sources, discovered, save))
			require.Equal(t, wantMappings, saved)
			for _, suffix := range []string{"", "-extra"} {
				err := c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: "m1-old" + suffix}, &v1.Model{})
				if tc.wantMigrated {
					require.True(t, apierrors.IsNotFound(err), "verified source should be deleted")
				} else {
					require.NoError(t, err, "unmigrated source must survive")
				}
			}
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
			require.NoError(t, policy.Spec.Manifest.Validate())
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(existing), existing))
			grants := policy.Spec.Manifest.Models
			// modelaccesspolicy.addModel uses this matcher to enumerate allowed model IDs.
			for target, want := range map[string]bool{
				tc.gatewayProvider + "/" + tc.native:            tc.wantQualified,
				tc.gatewayProvider + "/" + tc.native + "-extra": tc.wantQualified,
				existing.Spec.Manifest.TargetModel:              original.MatchesTargetModel(existing.Spec.Manifest.TargetModel),
				"unrelated-provider/" + tc.native:               false,
				"generic-responses/unrelated/model":             false,
			} {
				got := slices.ContainsFunc(grants, func(m types.ModelResource) bool { return m.MatchesTargetModel(target) })
				require.Equal(t, want, got, "access to %s after source cleanup", target)
			}
			want := []types.ModelResource{original}
			if tc.wantQualified {
				want = []types.ModelResource{original, {ID: "generic-responses/" + tc.pattern}}
			}
			require.Equal(t, want, grants)
			require.NoError(t, migrateLegacyModels(ctx, c, sources, discovered, save))
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
			require.Equal(t, grants, policy.Spec.Manifest.Models, "reconciliation must be idempotent")
		})
	}
}

func TestMigrationNativeSlashWildcardPartialRecovery(t *testing.T) {
	ctx := t.Context()
	ns := system.DefaultNamespace
	policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
		Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
		Models:   []types.ModelResource{{ID: "Qwen/Qwen3*"}, {ID: "m1-untouched"}, {ID: "obot://llm"}},
	}}}
	bare := policy.DeepCopy()
	bare.Name = "bare"
	bare.Spec.Manifest.Models = []types.ModelResource{{ID: "*"}}
	require.NoError(t, policy.Spec.Manifest.Validate())
	require.NoError(t, bare.Spec.Manifest.Validate())
	objects := []kclient.Object{policy, bare}
	for _, provider := range []string{"first", "second"} {
		objects = append(objects,
			&v1.Model{Namespace: ns, Name: "m1-old-" + provider, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
				ModelProvider: provider,
				TargetModel:   "Qwen/Qwen3-32B",
			}}},
			&v1.Model{Namespace: ns, Name: "m1-new-" + provider, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
				ModelProvider: system.LLMGatewayModelProvider,
				TargetModel:   provider + "/Qwen/Qwen3-32B",
			}}})
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
	sources := map[string]string{}
	want := slices.Clone(policy.Spec.Manifest.Models)
	for _, provider := range []string{"first", "second"} {
		sources[provider] = provider
		require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error { return nil }))
		want = append(want, types.ModelResource{ID: provider + "/Qwen/Qwen3*"})
		require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
		require.NoError(t, policy.Spec.Manifest.Validate())
		require.ElementsMatch(t, want, policy.Spec.Manifest.Models)
		require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(bare), bare))
		require.Equal(t, []types.ModelResource{{ID: "*"}}, bare.Spec.Manifest.Models)
		require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: "m1-old-" + provider}, &v1.Model{})))
		if provider == "first" {
			require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: "m1-old-second"}, &v1.Model{}))
		}
	}
}

func TestMigrationNativeSlashWildcardPolicyFailureKeepsSource(t *testing.T) {
	ctx := t.Context()
	ns := system.DefaultNamespace
	legacy := &v1.Model{Namespace: ns, Name: "m1-old", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
		ModelProvider: system.GenericResponsesModelProvider,
		TargetModel:   "Qwen/Qwen3-32B",
	}}}
	target := &v1.Model{Namespace: ns, Name: "m1-new", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
		ModelProvider: system.LLMGatewayModelProvider,
		TargetModel:   "generic-responses/Qwen/Qwen3-32B",
	}}}
	policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
		Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
		Models:   []types.ModelResource{{ID: "Qwen/Qwen3*"}},
	}}}
	require.NoError(t, policy.Spec.Manifest.Validate())
	fail := true
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(legacy, target, policy).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if _, ok := obj.(*v1.ModelAccessPolicy); ok && fail {
				return errors.New("injected policy failure")
			}
			return c.Update(ctx, obj, opts...)
		}}).Build()
	sources := map[string]string{system.GenericResponsesModelProvider: "generic-responses"}
	save := func(mapping map[string]string) error {
		require.Equal(t, map[string]string{legacy.Name: target.Name}, mapping)
		return nil
	}
	require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, nil, save), "injected policy failure")
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{}))
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: "Qwen/Qwen3*"}}, policy.Spec.Manifest.Models)
	fail = false
	require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, save))
	require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})))
}
