package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type testLocker struct {
	locked []db.LockID
}

func (l *testLocker) WithLock(_ context.Context, id db.LockID, fn func(tx *gorm.DB) error) error {
	l.locked = append(l.locked, id)
	return fn(nil)
}

func testVMCP(name string, components ...types.VMCPComponent) *v1.VMCP {
	return &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: components,
			},
		},
	}
}

func testInstance(name, vmcpID string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcpID,
			},
		},
	}
}

func testSubscription(memoryMiB int64) *v1.BillingSubscription {
	return &v1.BillingSubscription{
		Namespace: Key.Namespace,
		Name:      Key.Name,
		Status: v1.BillingSubscriptionStatus{
			Entitlements: &types.Entitlements{
				MemoryMiB: new(memoryMiB),
			},
		},
	}
}

func TestLimits(t *testing.T) {
	// Each instance runs the dedicated component with the default 200 MiB request.
	dedicated := types.VMCPComponent{
		ID:              "dedicated",
		Name:            "dedicated",
		ForceSingleUser: true,
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
			},
		},
	}

	for _, tt := range []struct {
		name         string
		subscription *v1.BillingSubscription
		obj          kclient.Object
		update       bool
		wantWritten  func(context.Context, kclient.Client) bool
		wantLock     bool
		wantRefused  bool
	}{
		{
			name:         "a write within the plan is made under the lock",
			subscription: testSubscription(400),
			obj:          testInstance("instance2", "vmcp1"),
			wantWritten:  instanceExists("instance2"),
			wantLock:     true,
		},
		{
			name:         "a write past the plan is refused",
			subscription: testSubscription(200),
			obj:          testInstance("instance2", "vmcp1"),
			wantWritten:  instanceExists("instance2"),
			wantLock:     true,
			wantRefused:  true,
		},
		{
			name:         "a write that lowers usage is made while over",
			subscription: testSubscription(0),
			obj:          testVMCP("vmcp1"),
			update:       true,
			wantWritten: func(ctx context.Context, c kclient.Client) bool {
				var vmcp v1.VMCP
				return c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "vmcp1"}, &vmcp) == nil &&
					len(vmcp.Spec.Manifest.Components) == 0
			},
			wantLock: true,
		},
		{
			name:        "before billing answers nothing is limited",
			obj:         testInstance("instance2", "vmcp1"),
			wantWritten: instanceExists("instance2"),
			wantLock:    true,
		},
		{
			name:         "kinds no meter watches skip the lock",
			subscription: testSubscription(0),
			obj: &v1.MCPCatalog{
				Namespace: system.DefaultNamespace,
				Name:      "catalog",
			},
			wantWritten: func(ctx context.Context, c kclient.Client) bool {
				var catalog v1.MCPCatalog
				return c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "catalog"}, &catalog) == nil
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := []kclient.Object{testVMCP("vmcp1", dedicated), testInstance("instance1", "vmcp1")}
			if tt.subscription != nil {
				objs = append(objs, tt.subscription)
			}
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objs...).Build()
			locker := new(testLocker)

			write := func() error {
				return storage.Create(t.Context(), tt.obj)
			}
			if tt.update {
				stored := tt.obj.DeepCopyObject().(kclient.Object)
				if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(tt.obj), stored); err != nil {
					t.Fatal(err)
				}
				tt.obj.SetResourceVersion(stored.GetResourceVersion())
				write = func() error {
					return storage.Update(t.Context(), tt.obj)
				}
			}

			err := NewLimits(storage, locker).Within(t.Context(), tt.obj, write)
			if tt.wantRefused {
				var httpErr *types.ErrHTTP
				if !errors.As(err, &httpErr) || httpErr.Code != http.StatusPaymentRequired {
					t.Fatalf("Within() error = %v, want a %d", err, http.StatusPaymentRequired)
				}
			} else if err != nil {
				t.Fatalf("Within() error = %v", err)
			}
			if written := tt.wantWritten(t.Context(), storage); written == tt.wantRefused {
				t.Errorf("written = %v, want %v", written, !tt.wantRefused)
			}
			if locked := len(locker.locked) == 1 && locker.locked[0] == db.HostedResourcesLock; locked != tt.wantLock {
				t.Errorf("locked = %v, want %v", locker.locked, tt.wantLock)
			}
		})
	}
}

func instanceExists(name string) func(context.Context, kclient.Client) bool {
	return func(ctx context.Context, c kclient.Client) bool {
		var instance v1.VMCPInstance
		err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &instance)
		return !apierrors.IsNotFound(err)
	}
}

func TestRefusal(t *testing.T) {
	const message = "There isn't room on the plan for this."
	for _, tt := range []struct {
		name        string
		err         error
		wantRefusal bool
	}{
		{
			name: "storage refused the write",
			err: fmt.Errorf("failed to apply: %w", &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Code:    http.StatusPaymentRequired,
					Message: message,
				},
			}),
			wantRefusal: true,
		},
		{
			name: "storage failed for another reason",
			err:  apierrors.NewConflict(schema.GroupResource{}, "vmcp1", errors.New("conflict")),
		},
		{
			name: "no error",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refusal, ok := Refusal(tt.err)
			if ok != tt.wantRefusal {
				t.Fatalf("Refusal() ok = %v, want %v", ok, tt.wantRefusal)
			}
			if ok && (refusal.Code != http.StatusPaymentRequired || refusal.Message != message) {
				t.Errorf("Refusal() = %d %q, want %d %q", refusal.Code, refusal.Message, http.StatusPaymentRequired, message)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	sized := types.VMCPComponent{
		ID:   "sized",
		Name: "sized",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{
						CPU:    "500m",
						Memory: "1Gi",
					},
				},
			},
		},
	}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testVMCP("vmcp1", sized)).Build()

	usage, err := Usage(t.Context(), storage)
	if err != nil {
		t.Fatalf("Usage() error = %v", err)
	}
	if want := (types.EntitlementsUsage{MemoryMiB: 1024, CPUMillicores: 500}); usage != want {
		t.Errorf("Usage() = %+v, want %+v", usage, want)
	}
}
