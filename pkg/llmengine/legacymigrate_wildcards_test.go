package llmengine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
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
		wantGrant       string
		denied          []string
	}{
		{
			name:            "bare prefix skips native namespace",
			pattern:         "Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
			wantGrant:       "generic-responses/Qwen/Qwen3*",
			denied: []string{
				"generic-responses/Qwen3-32B",
				"generic-responses/Other/Qwen3-32B",
				"generic-responses/Qwen/Other",
			},
		},
		{
			name:            "bare prefix matches native directly",
			pattern:         "Qwen3*",
			native:          "Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
			wantGrant:       "generic-responses/Qwen3*",
		},
		{
			name:            "direct prefix wins over overlapping native remainder",
			pattern:         "Qwen*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
			wantGrant:       "generic-responses/Qwen*",
		},
		{
			name:            "bare prefix before multiple native slashes",
			pattern:         "team*",
			native:          "org/team/model",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
			wantQualified:   true,
			wantGrant:       "generic-responses/org/team*",
			denied: []string{
				"generic-responses/team/model",
				"generic-responses/other/team/model",
				"generic-responses/org/other/model",
			},
		},
		{
			name:            "bare prefix cannot skip two native segments",
			pattern:         "Qwen3*",
			native:          "org/team/Qwen3-32B",
			gatewayProvider: "generic-responses",
			wantMigrated:    true,
		},
		{
			name:            "bare prefix unverified source",
			pattern:         "Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			unverified:      true,
		},
		{
			name:            "bare prefix undiscovered target",
			pattern:         "Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "generic-responses",
			undiscovered:    true,
		},
		{
			name:            "bare prefix unrelated target provider",
			pattern:         "Qwen3*",
			native:          "Qwen/Qwen3-32B",
			gatewayProvider: "other",
		},
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
			for _, target := range tc.denied {
				require.False(t, slices.ContainsFunc(grants, func(m types.ModelResource) bool { return m.MatchesTargetModel(target) }), "must not grant %s", target)
			}
			want := []types.ModelResource{original}
			if tc.wantQualified {
				if !strings.Contains(tc.pattern, "/") {
					want = nil
				}
				grant := tc.wantGrant
				if grant == "" {
					grant = "generic-responses/" + tc.pattern
				}
				want = append(want, types.ModelResource{ID: grant})
			}
			require.Equal(t, want, grants)
			require.NoError(t, migrateLegacyModels(ctx, c, sources, discovered, save))
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
			require.Equal(t, grants, policy.Spec.Manifest.Models, "reconciliation must be idempotent")
		})
	}
}

func TestMigrationNativeSlashWildcardPartialRecovery(t *testing.T) {
	for _, pattern := range []string{"Qwen/Qwen3*", "Qwen3*"} {
		t.Run(pattern, func(t *testing.T) {
			ctx := t.Context()
			ns := system.DefaultNamespace
			policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
				Models:   []types.ModelResource{{ID: pattern}, {ID: "m1-untouched"}, {ID: "obot://llm"}},
			}}}
			bare := policy.DeepCopy()
			bare.Name = "bare"
			bare.Spec.Manifest.Models = []types.ModelResource{{ID: "*"}}
			require.NoError(t, policy.Spec.Manifest.Validate())
			require.NoError(t, bare.Spec.Manifest.Validate())
			namespaces := []string{"Qwen"}
			if pattern == "Qwen3*" {
				namespaces = append(namespaces, "Other")
			}
			objects := []kclient.Object{policy, bare}
			for _, provider := range []string{"first", "second"} {
				for _, namespace := range namespaces {
					objects = append(objects,
						&v1.Model{Namespace: ns, Name: "m1-old-" + provider + namespace, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
							ModelProvider: provider,
							TargetModel:   namespace + "/Qwen3-32B",
						}}},
						&v1.Model{Namespace: ns, Name: "m1-new-" + provider + namespace, Spec: v1.ModelSpec{Manifest: types.ModelManifest{
							ModelProvider: system.LLMGatewayModelProvider,
							TargetModel:   provider + "/" + namespace + "/Qwen3-32B",
						}}})
				}
			}
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
			sources := map[string]string{}
			want := slices.Clone(policy.Spec.Manifest.Models)
			for _, provider := range []string{"first", "second"} {
				sources[provider] = provider
				require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error { return nil }))
				for _, namespace := range namespaces {
					want = append(want, types.ModelResource{ID: provider + "/" + namespace + "/Qwen3*"})
					require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: "m1-old-" + provider + namespace}, &v1.Model{})))
					if provider == "first" {
						require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: "m1-old-second" + namespace}, &v1.Model{}))
					}
				}
				if provider == "second" && !strings.Contains(pattern, "/") {
					want = want[1:]
				}
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
				require.NoError(t, policy.Spec.Manifest.Validate())
				require.ElementsMatch(t, want, policy.Spec.Manifest.Models)
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(bare), bare))
				require.Equal(t, []types.ModelResource{{ID: "*"}}, bare.Spec.Manifest.Models)
				for _, namespace := range namespaces {
					target := provider + "/" + namespace + "/Qwen3-32B"
					require.True(t, slices.ContainsFunc(policy.Spec.Manifest.Models, func(m types.ModelResource) bool { return m.MatchesTargetModel(target) }))
				}
				require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error { return nil }))
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
				require.ElementsMatch(t, want, policy.Spec.Manifest.Models, "partial and complete reconciliation must be idempotent")
			}
		})
	}
}

func TestMigrationNativeSlashWildcardPolicyFailureKeepsSource(t *testing.T) {
	for _, pattern := range []string{"Qwen/Qwen3*", "Qwen3*"} {
		t.Run(pattern, func(t *testing.T) {
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
				Models:   []types.ModelResource{{ID: pattern}},
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
			require.Equal(t, []types.ModelResource{{ID: pattern}}, policy.Spec.Manifest.Models)
			fail = false
			require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, save))
			require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})))
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
			want := []types.ModelResource{{ID: "generic-responses/Qwen/Qwen3*"}}
			if strings.Contains(pattern, "/") {
				want = append([]types.ModelResource{{ID: pattern}}, want...)
			}
			require.Equal(t, want, policy.Spec.Manifest.Models)
		})
	}
}

func TestMigrationBareWildcardPreservesExistingGatewayMatches(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing []string
		want     []types.ModelResource
		denied   []string
	}{
		{
			name: "undiscovered providers and duplicate families",
			existing: []string{
				"anthropic/claude-test",
				"anthropic/claude-test-extra",
				"bedrock/claude-test",
				"generic-responses/claude-extra",
				"openai/gpt-test",
				"other/Claude-test",
			},
			want: []types.ModelResource{
				{ID: "generic-responses/claude*"},
				{ID: "anthropic/claude*"},
				{ID: "bedrock/claude*"},
			},
			denied: []string{
				"unverified/claude-test",
				"undiscovered/claude-test",
				"openai/claude-test",
				"proxy/claude-test",
				"generic-responses/gpt-test",
			},
		},
		{
			name:     "bare prefix directly matches gateway provider",
			existing: []string{"claude-host/other-model", "anthropic/claude-test"},
			want: []types.ModelResource{
				{ID: "claude*"},
				{ID: "generic-responses/claude*"},
				{ID: "anthropic/claude*"},
			},
			denied: []string{"other/gpt-test", "generic-responses/gpt-test"},
		},
	} {
		for _, failFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retry=%t", tc.name, failFirst), func(t *testing.T) {
				ctx := t.Context()
				ns := system.DefaultNamespace
				legacy := &v1.Model{Namespace: ns, Name: "m1-old", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.GenericResponsesModelProvider,
					TargetModel:   "claude-test",
					Active:        true,
				}}}
				target := &v1.Model{Namespace: ns, Name: "m1-new", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   "generic-responses/claude-test",
					Active:        true,
				}}}
				policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
					Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "u1"}},
					Models:   []types.ModelResource{{ID: "claude*"}, {ID: legacy.Name}, {ID: "obot://llm"}},
				}}}
				provider := &v1.ModelProvider{Namespace: ns, Name: system.GenericResponsesModelProvider}
				objects := []kclient.Object{legacy, target, policy, provider,
					&v1.Model{Namespace: ns, Name: "m1-proxy", Spec: v1.ModelSpec{Manifest: types.ModelManifest{
						ModelProvider: system.ModelProxyModelProvider,
						TargetModel:   "proxy/claude-test",
					}}},
				}
				var existing []*v1.Model
				for i, upstream := range tc.existing {
					m := &v1.Model{Namespace: ns, Name: fmt.Sprintf("m1-existing-%d", i), Spec: v1.ModelSpec{Manifest: types.ModelManifest{
						ModelProvider: system.LLMGatewayModelProvider,
						TargetModel:   upstream,
						Active:        true,
						DisplayName:   "Administrator's model",
						Alias:         "custom-alias",
						OverrideCost:  &types.ModelCost{Input: 7},
					}}}
					existing = append(existing, m)
					objects = append(objects, m)
				}
				saved := false
				fail := failFirst
				c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).
					WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
						return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
					}).WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if _, ok := obj.(*v1.ModelAccessPolicy); ok {
						require.True(t, saved, "mappings must persist before policy updates")
						require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{}), "source must survive until references commit")
						if fail {
							return errors.New("injected policy failure")
						}
					}
					return c.Update(ctx, obj, opts...)
				}}).Build()
				// Only the imported Generic Responses model is discovery evidence.
				discovered := map[string]struct{}{target.Spec.Manifest.TargetModel: {}}
				sources := map[string]string{
					system.GenericResponsesModelProvider: "generic-responses",
					"undiscovered-source":                "undiscovered",
				}
				save := func(mapping map[string]string) error {
					if len(mapping) > 0 {
						require.Equal(t, map[string]string{legacy.Name: target.Name}, mapping)
					}
					saved = true
					return nil
				}
				if failFirst {
					require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, discovered, save), "injected policy failure")
					require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{}))
					require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(provider), &v1.ModelProvider{}))
					got := &v1.ModelAccessPolicy{}
					require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), got))
					require.Equal(t, policy.Spec, got.Spec)
					fail = false
				}
				require.NoError(t, migrateLegacyModels(ctx, c, sources, discovered, save))
				require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})))
				require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKeyFromObject(provider), &v1.ModelProvider{})))
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
				require.NoError(t, policy.Spec.Manifest.Validate())
				// Evaluate effective target access after cleanup with the policy helper's matcher.
				for _, m := range append(existing, target) {
					upstream := m.Spec.Manifest.TargetModel
					want := (types.ModelResource{ID: "claude*"}).MatchesTargetModel(upstream)
					for _, candidate := range []string{upstream, upstream + "-future"} {
						got := slices.ContainsFunc(policy.Spec.Manifest.Models, func(grant types.ModelResource) bool { return grant.MatchesTargetModel(candidate) })
						require.Equal(t, want, got, "effective access to %s", candidate)
					}
				}
				for _, upstream := range tc.denied {
					require.False(t, slices.ContainsFunc(policy.Spec.Manifest.Models, func(grant types.ModelResource) bool { return grant.MatchesTargetModel(upstream) }), "must not grant %s", upstream)
				}
				want := append(slices.Clone(tc.want), types.ModelResource{ID: target.Name}, types.ModelResource{ID: "obot://llm"})
				require.ElementsMatch(t, want, policy.Spec.Manifest.Models)
				for _, original := range existing {
					got := &v1.Model{}
					require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(original), got))
					require.Equal(t, original, got, "existing gateway configuration must remain unchanged")
				}
				grants := slices.Clone(policy.Spec.Manifest.Models)
				require.NoError(t, migrateLegacyModels(ctx, c, sources, discovered, save))
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(policy), policy))
				require.Equal(t, grants, policy.Spec.Manifest.Models, "retry must not duplicate qualifiers")
			})
		}
	}
}
