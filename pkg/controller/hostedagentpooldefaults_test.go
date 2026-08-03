package controller

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureHostedAgentPoolDefaults(t *testing.T) {
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	if err := ensureHostedAgentPoolDefaults(ctx, client); err != nil {
		t.Fatal(err)
	}

	var defaults v1.HostedAgentPoolDefaults
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "default"}
	if err := client.Get(ctx, key, &defaults); err != nil {
		t.Fatal(err)
	}
	if err := defaults.Spec.Manifest.Validate(); err != nil {
		t.Fatalf("created invalid defaults: %v", err)
	}

	defaults.Spec.Manifest = types.HostedAgentPoolDefaultsManifest{
		Capacity: types.HostedAgentResourceQuantity{
			CPUVCPUs:     8,
			MemoryBytes:  16,
			StorageBytes: 32,
		},
		Suspended: true,
	}
	if err := client.Update(ctx, &defaults); err != nil {
		t.Fatal(err)
	}
	if err := ensureHostedAgentPoolDefaults(ctx, client); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, key, &defaults); err != nil {
		t.Fatal(err)
	}
	if defaults.Spec.Manifest.Capacity.CPUVCPUs != 8 || !defaults.Spec.Manifest.Suspended {
		t.Fatalf("existing administrator defaults were changed: %#v", defaults.Spec.Manifest)
	}
}

func TestEnsureHostedAgentPoolDefaultsPreservesExisting(t *testing.T) {
	existing := &v1.HostedAgentPoolDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace},
		Spec: v1.HostedAgentPoolDefaultsSpec{
			Manifest: types.HostedAgentPoolDefaultsManifest{
				Capacity: types.HostedAgentResourceQuantity{
					CPUVCPUs:     2,
					MemoryBytes:  1,
					StorageBytes: 1,
				},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(existing).Build()
	if err := ensureHostedAgentPoolDefaults(context.Background(), client); err != nil {
		t.Fatal(err)
	}

	var got v1.HostedAgentPoolDefaults
	if err := client.Get(context.Background(), kclient.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Manifest.Capacity.CPUVCPUs != 2 {
		t.Fatalf("existing defaults were overwritten: %#v", got.Spec.Manifest)
	}
}

// The seeded defaults must state the sandbox count rather than leaving it to
// the fallback, or an administrator opening the defaults sees a blank field
// while a different number is actually in force.
func TestEnsureHostedAgentPoolDefaultsSeedsMaxSandboxes(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	if err := ensureHostedAgentPoolDefaults(context.Background(), client); err != nil {
		t.Fatalf("ensureHostedAgentPoolDefaults: %v", err)
	}

	var defaults v1.HostedAgentPoolDefaults
	if err := client.Get(context.Background(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace, Name: "default",
	}, &defaults); err != nil {
		t.Fatalf("get defaults: %v", err)
	}

	if defaults.Spec.Manifest.MaxSandboxes <= 0 {
		t.Errorf("maxSandboxes = %d, want a positive seeded value", defaults.Spec.Manifest.MaxSandboxes)
	}
	if err := defaults.Spec.Manifest.Validate(); err != nil {
		t.Errorf("seeded defaults are invalid: %v", err)
	}
}
