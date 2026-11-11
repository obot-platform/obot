package mcp_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/billing"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// The storage server and the gateway share SQLite's one connection, as they do in Obot, so a write that deadlocked on
// the compute lock would hang here.
func TestStorageAdmitsComputeWithinTheSubscription(t *testing.T) {
	c, _, dbAccess, services, err := storage.Start(t.Context(), sservices.Config{
		DSN: "sqlite://file:" + filepath.Join(t.TempDir(), "obot.db") + "?_journal=WAL&cache=shared&_busy_timeout=30000",
	})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	gateway := gclient.New(t.Context(), database, c, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	services.Admission.Set(mcp.NewComputeAdmitter(c, billing.NewProvider(nil, c, c), gateway.WithComputeLock))

	// Each instance runs the dedicated component with the default 200 MiB request, and the subscription has room for
	// one.
	subscription := &v1.Subscription{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
	}
	if err := c.Create(t.Context(), subscription); err != nil {
		t.Fatal(err)
	}
	subscription.Status.Entitlements = types.Entitlements{
		MemoryMiB: new(int64(200)),
	}
	if err := c.Status().Update(t.Context(), subscription); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{
		Namespace: system.DefaultNamespace,
		Name:      "vmcp1",
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{{
					ID:              "dedicated",
					Name:            "dedicated",
					ForceSingleUser: true,
					CatalogEntry: types.MCPServerCatalogEntrySnapshot{
						Manifest: types.MCPServerCatalogEntryManifest{
							Runtime: types.RuntimeNPX,
						},
					},
				}},
				Profiles: []types.VMCPProfile{{
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				}},
			},
		},
	}
	if err := c.Create(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	instance := func(name string) *v1.VMCPInstance {
		return &v1.VMCPInstance{
			Namespace: system.DefaultNamespace,
			Name:      name,
			Spec: v1.VMCPInstanceSpec{
				Manifest: types.VMCPInstanceManifest{
					VMCPID: vmcp.Name,
				},
			},
		}
	}

	// The first instance fits.
	first := instance("instance1")
	if err := c.Create(t.Context(), first); err != nil {
		t.Fatalf("Create() of the instance that fits = %v", err)
	}

	// The second doesn't, and isn't stored.
	err = c.Create(t.Context(), instance("instance2"))
	if _, refused := mcp.ComputeLimitRefusal(err); !refused {
		t.Fatalf("Create() past the limit = %v, want a refusal", err)
	}
	if err := c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "instance2"}, new(v1.VMCPInstance)); !apierrors.IsNotFound(err) {
		t.Fatalf("Get() of the refused instance = %v, want not found", err)
	}

	// Writes that don't raise usage go through at the limit, and so does deleting.
	first.Labels = map[string]string{"still": "fits"}
	if err := c.Update(t.Context(), first); err != nil {
		t.Fatalf("Update() at the limit = %v", err)
	}
	if err := c.Delete(t.Context(), first); err != nil {
		t.Fatalf("Delete() = %v", err)
	}

	// With the first gone, there's room again.
	if err := c.Create(t.Context(), instance("instance2")); err != nil {
		t.Fatalf("Create() after room was freed = %v", err)
	}
}
