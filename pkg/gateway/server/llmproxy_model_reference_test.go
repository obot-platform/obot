package server

import (
	"context"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetModelFromReference_ReturnsNotFoundWhenNameAndTargetMiss(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		Build()

	_, err := getModelFromReference(context.Background(), client, "default", "missing-model")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected not found error type, got %T: %v", err, err)
	}
}

func TestGetModelFromReference_ReturnsModelByResourceName(t *testing.T) {
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-gpt-4.1-mini",
			Namespace: "default",
		},
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			Name:        "manifest-name",
			TargetModel: "target-model-id",
			Active:      true,
		}},
	}

	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(model).
		Build()

	got, err := getModelFromReference(context.Background(), client, "default", "openai-gpt-4.1-mini")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got.Name != "openai-gpt-4.1-mini" {
		t.Fatalf("expected openai-gpt-4.1-mini, got %q", got.Name)
	}
}

func TestGetModelFromReference_DoesNotFallbackToManifestNameOrTargetModel(t *testing.T) {
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-gpt-4.1-mini",
			Namespace: "default",
		},
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			Name:        "manifest-name",
			TargetModel: "target-model-id",
			Active:      true,
		}},
	}

	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(model).
		Build()

	for _, ref := range []string{"manifest-name", "target-model-id"} {
		_, err := getModelFromReference(context.Background(), client, "default", ref)
		if err == nil {
			t.Fatalf("%s: expected error, got nil", ref)
		}

		if !apierrors.IsNotFound(err) {
			t.Fatalf("%s: expected not found error type, got %T: %v", ref, err, err)
		}
	}
}
