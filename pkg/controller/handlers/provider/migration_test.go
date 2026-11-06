package provider

import (
	"context"
	"errors"
	"testing"

	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type failingMigrationProvider struct{}

func (failingMigrationProvider) Ready() bool                               { return true }
func (failingMigrationProvider) HasUpstreamProviders(context.Context) bool { return true }
func (failingMigrationProvider) ListModels(context.Context) (*openai.ModelsList, error) {
	return &openai.ModelsList{}, nil
}
func (failingMigrationProvider) MigrateLegacyModels(context.Context, kclient.Client) error {
	return errors.New("injected migration conflict")
}

func TestBackPopulateRetriesMigrationFailures(t *testing.T) {
	mp := llmengine.ModelProviderResource()
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	for _, kind := range []string{"Model", "ModelInfo", "ModelProvider"} {
		mapper.Add(v1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRESTMapper(mapper).
		WithObjects(mp).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).Build()
	d := dispatcher.New(nil, c, nil, nil, "", "", "")
	d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, failingMigrationProvider{})
	require.ErrorContains(t, BackPopulateModels(t.Context(), c, d, mp), "injected migration conflict")
}
