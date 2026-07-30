package modelaccesspolicy

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveModelReference(t *testing.T) {
	const (
		namespace = "default"
		provider  = "openai-model-provider"
	)

	model := newModel("openai-gpt-4o", provider, "openai/gpt-4o", true)
	model.Spec.Manifest.Name = "catalog-model-name"
	defaultAlias := &v1.DefaultModelAlias{
		ObjectMeta: metav1.ObjectMeta{Name: "default-llm"},
		Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{
			Alias: "llm",
			Model: model.Name,
		}},
	}
	modelAlias := &v1.Alias{
		ObjectMeta: metav1.ObjectMeta{Name: alias.KeyFromScopeID("Model", "llm")},
		Spec: v1.AliasSpec{
			TargetKind: "DefaultModelAlias",
			TargetName: defaultAlias.Name,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(model, defaultAlias, modelAlias).
		Build()
	helper := newModelHelper(t, []*v1.Model{model})

	for _, tt := range []struct {
		name         string
		reference    string
		wantName     string
		wantNotFound bool
	}{
		{name: "resource name", reference: model.Name, wantName: model.Name},
		{name: "default alias", reference: "llm", wantName: model.Name},
		{name: "provider native ID", reference: model.Spec.Manifest.TargetModel, wantName: model.Name},
		{name: "catalog model name is not a reference", reference: model.Spec.Manifest.Name, wantNotFound: true},
		{name: "unknown reference", reference: "openai/unknown", wantNotFound: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := helper.ResolveModelReference(t.Context(), client, namespace, provider, tt.reference)
			if tt.wantNotFound {
				require.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
		})
	}
}
