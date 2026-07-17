package model

import (
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoveApplyUpdateAnnotation(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{
		Name:      "model",
		Namespace: "default",
		Annotations: map[string]string{
			apply.AnnotationUpdate: "false",
			"keep":                 "value",
		},
	}}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(model).Build()

	if err := new(Handler).RemoveApplyUpdateAnnotation(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: model,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var updated v1.Model
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(model), &updated); err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.Annotations[apply.AnnotationUpdate]; ok {
		t.Fatal("apply update annotation was not removed")
	}
	if updated.Annotations["keep"] != "value" {
		t.Fatalf("unrelated annotation = %q, want value", updated.Annotations["keep"])
	}
}
