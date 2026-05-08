package server

import (
	"context"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetModelFromReference_ReturnsNotFoundWhenNameAndTargetMiss(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.Model{}, "spec.manifest.name", func(obj kclient.Object) []string {
			return []string{obj.(*v1.Model).Spec.Manifest.Name}
		}).
		WithIndex(&v1.Model{}, "spec.manifest.targetModel", func(obj kclient.Object) []string {
			return []string{obj.(*v1.Model).Spec.Manifest.TargetModel}
		}).
		Build()

	_, err := getModelFromReference(context.Background(), client, "default", "missing-model")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected not found error type, got %T: %v", err, err)
	}
}

func TestGetModelFromReference_FallsBackToTargetModelAndReturnsOldest(t *testing.T) {
	oldest := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "model-old",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{Name: "provider-old", TargetModel: "gpt-4.1"}},
	}

	newest := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "model-new",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{Name: "provider-new", TargetModel: "gpt-4.1"}},
	}

	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.Model{}, "spec.manifest.name", func(obj kclient.Object) []string {
			return []string{obj.(*v1.Model).Spec.Manifest.Name}
		}).
		WithIndex(&v1.Model{}, "spec.manifest.targetModel", func(obj kclient.Object) []string {
			return []string{obj.(*v1.Model).Spec.Manifest.TargetModel}
		}).
		WithObjects(newest, oldest).
		Build()

	model, err := getModelFromReference(context.Background(), client, "default", "gpt-4.1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if model.Name != "model-old" {
		t.Fatalf("expected oldest model, got %q", model.Name)
	}
}
