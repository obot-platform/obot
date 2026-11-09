package llmengine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/obot-platform/kinm/pkg/strategy"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestMigrationReferenceRecoveryPreservesNewerSourceEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.ModelManifest)
	}{
		{
			name: "deactivation",
			edit: func(m *types.ModelManifest) { m.Active = false },
		},
		{
			name: "alias",
			edit: func(m *types.ModelManifest) { m.Alias = "new-source-alias" },
		},
		{
			name: "cost override",
			edit: func(m *types.ModelManifest) { m.OverrideCost = &types.ModelCost{Input: 3} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			legacy := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-old", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.OpenAIModelProvider, TargetModel: "gpt-5", Active: true}}}
			target := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-new", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "openai/gpt-5", Active: true}}}
			alias := &v1.DefaultModelAlias{Namespace: system.DefaultNamespace, Name: "llm", Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: legacy.Name}}}
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(legacy, target, alias).
				WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
					return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
				}).Build()
			sources := map[string]string{system.OpenAIModelProvider: "openai"}
			require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error {
				return errors.New("injected mapping-save failure")
			}), "injected mapping-save failure")
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), legacy))
			tc.edit(&legacy.Spec.Manifest)
			require.NoError(t, c.Update(ctx, legacy))
			require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(target), target))
			target.Spec.Manifest.Alias = "later-target-alias"
			target.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 9}
			require.NoError(t, c.Update(ctx, target))
			laterSource, laterTarget := legacy.DeepCopy(), target.DeepCopy()
			for range 2 {
				require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error {
					t.Fatal("a changed source must be reported before saving mappings or moving references")
					return nil
				}), "changed after")
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), legacy))
				require.Equal(t, laterSource.Spec, legacy.Spec)
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(target), target))
				require.Equal(t, laterTarget, target, "recovery must not replay newer source edits over later target edits")
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(alias), alias))
				require.Equal(t, legacy.Name, alias.Spec.Manifest.Model)
			}
		})
	}
}

func TestMigrationCleanupPreservesConcurrentSourceEdits(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	api, _, db, _, err := storage.Start(ctx, sservices.Config{
		DSN:          "sqlite://file:" + filepath.Join(t.TempDir(), "migration.db") + "?_journal=WAL&_busy_timeout=30000",
		StorageToken: "test",
	})
	require.NoError(t, err)
	defer db.SQLDB.Close()
	for _, phase := range []string{"before-delete-request", "after-delete-read", "same-name-replacement", "status-only-update"} {
		t.Run(phase, func(t *testing.T) {
			legacy := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-old-" + phase, Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.OpenAIModelProvider, TargetModel: phase, Active: true}}}
			target := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-new-" + phase, Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "openai/" + phase, Active: true}}}
			require.NoError(t, api.Create(ctx, legacy))
			require.NoError(t, api.Create(ctx, target))
			t.Cleanup(func() {
				require.NoError(t, kclient.IgnoreNotFound(api.Delete(ctx, legacy)))
				require.NoError(t, kclient.IgnoreNotFound(api.Delete(ctx, target)))
			})
			originalUID := legacy.UID
			edit := func() {
				require.NoError(t, api.Get(ctx, kclient.ObjectKeyFromObject(legacy), legacy))
				switch phase {
				case "same-name-replacement":
					require.NoError(t, api.Delete(ctx, legacy))
					legacy = &v1.Model{Namespace: legacy.Namespace, Name: legacy.Name, Spec: legacy.Spec}
					require.NoError(t, api.Create(ctx, legacy))
					require.NotEqual(t, originalUID, legacy.UID)
				case "status-only-update":
					legacy.Status.ObservedGeneration++
					require.NoError(t, api.Status().Update(ctx, legacy))
				default:
					legacy.Spec.Manifest.Active = false
					legacy.Spec.Manifest.Alias = "new-source-alias"
					legacy.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 3}
					require.NoError(t, api.Update(ctx, legacy))
				}
				require.NoError(t, api.Get(ctx, kclient.ObjectKeyFromObject(target), target))
				target.Spec.Manifest.Alias = "later-target-alias"
				require.NoError(t, api.Update(ctx, target))
			}
			c := interceptor.NewClient(api, interceptor.Funcs{
				Delete: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.DeleteOption) error {
					if obj.GetName() != legacy.Name {
						return c.Delete(ctx, obj, opts...)
					}
					if phase != "after-delete-read" {
						edit()
						err := c.Delete(ctx, obj, opts...)
						require.ErrorContains(t, err, "Precondition failed", "the real HTTP API must enforce cleanup preconditions")
						return err
					}
					// Exercise the same adapter and SQL strategy as the HTTP API, with
					// an edit after its read/precondition check but before its durable delete.
					modelStrategy, err := db.NewDBStrategy(&v1.Model{})
					require.NoError(t, err)
					deleter := strategy.NewDelete(scheme.Scheme, modelStrategy)
					options := (&kclient.DeleteOptions{}).ApplyOptions(opts).AsDeleteOptions()
					_, _, err = deleter.Delete(genericapirequest.WithNamespace(ctx, obj.GetNamespace()), obj.GetName(), func(context.Context, runtime.Object) error {
						edit()
						return nil
					}, options)
					require.True(t, apierrors.IsConflict(err), "the SQL strategy must reject a version changed after the adapter's read")
					return err
				},
			})
			sources := map[string]string{system.OpenAIModelProvider: "openai"}
			err := migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error { return nil })
			require.ErrorContains(t, err, "failed to delete legacy model")
			laterSource, laterTarget := legacy.DeepCopy(), target.DeepCopy()
			require.NoError(t, api.Get(ctx, kclient.ObjectKeyFromObject(legacy), legacy))
			require.Equal(t, laterSource, legacy, "a stale cleanup must preserve the edited or recreated source")
			err = migrateLegacyModels(ctx, api, sources, nil, func(map[string]string) error { return nil })
			if phase == "status-only-update" {
				require.NoError(t, err, "a status update must allow cleanup on retry without replaying source edits")
				require.True(t, apierrors.IsNotFound(api.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})))
			} else {
				require.ErrorContains(t, err, "changed after")
				require.NoError(t, api.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{}))
			}
			require.NoError(t, api.Get(ctx, kclient.ObjectKeyFromObject(target), target))
			require.Equal(t, laterTarget, target, "retry must preserve later target edits")
		})
	}
}
