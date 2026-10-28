package modelinfosource

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// An upgrade deletes the models.dev source an earlier version created, and starting again
// with no source is a no-op.
func TestDeleteDefaultSource(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.ModelInfoSource{
		Name:      system.DefaultModelInfoSource,
		Namespace: system.DefaultNamespace,
		Spec:      v1.ModelInfoSourceSpec{Manifest: types.ModelInfoSourceManifest{URL: "https://models.dev/api.json"}},
	}).Build()

	for range 2 {
		if err := DeleteDefaultSource(t.Context(), c); err != nil {
			t.Fatal(err)
		}
	}
	err := c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultModelInfoSource}, &v1.ModelInfoSource{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("the model info source is still there: %v", err)
	}
}
