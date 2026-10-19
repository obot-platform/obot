package vmcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storage "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestComponentOAuthReconciliationRetriesAndCleansRemovedCredentials(t *testing.T) {
	services, err := storage.New(storage.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	failNotification := true
	notificationErr := errors.New("notification failed")
	gw := gateway.New(t.Context(), db, nil, nil, func(context.Context, string) error {
		if failNotification {
			return notificationErr
		}
		return nil
	}, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gw.Close()) })
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Annotations: map[string]string{vmcpconfig.ComponentOAuthAnnotation("kept"): "pending:revision"}, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "kept"}}}}}
	server := &v1.MCPServer{Name: "ms1test", Namespace: vmcp.Namespace, Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "kept"}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, server).Build()
	failNotification = false
	require.NoError(t, gw.ReplaceMCPOAuthToken(t.Context(), "user", server.Name, "url", "", &oauth2.Config{ClientID: "old"}, &oauth2.Token{AccessToken: "token"}))
	failNotification = true
	for _, id := range []string{"kept", "removed"} {
		require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, id), Name: system.StaticOAuthCredentialName, Secrets: map[string]string{"CLIENT_ID": id}}))
	}
	handler := New(gw, nil, client)
	req := router.Request{Ctx: t.Context(), Client: client, Object: vmcp}
	require.ErrorIs(t, handler.SyncOAuthCredentials(req, nil), notificationErr)
	require.True(t, vmcpconfig.ComponentOAuthPending(*vmcp, "kept"))
	failNotification = false
	require.NoError(t, handler.SyncOAuthCredentials(req, nil))
	require.False(t, vmcpconfig.ComponentOAuthPending(*vmcp, "kept"))
	_, err = gw.GetMCPOAuthToken(t.Context(), "user", server.Name, "url")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = gw.RevealCredential(t.Context(), []string{vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, "removed")}, system.StaticOAuthCredentialName)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = gw.RevealCredential(t.Context(), []string{vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, "kept")}, system.StaticOAuthCredentialName)
	require.NoError(t, err)
}

func TestComponentOAuthCleanupPreservesNewComponentMissingFromReconcileObject(t *testing.T) {
	stale := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace}
	live := stale.DeepCopy()
	cached := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(stale.DeepCopy()).Build()
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(live).Build()
	credential := gatewaytypes.Credential{
		Context: vmcpconfig.ComponentOAuthCredentialContext(live.Name, "new"),
		Name:    system.StaticOAuthCredentialName,
	}
	deleted := false
	handler := &Handler{uncachedReader: client, credentials: fakeCredentials{
		list: func(context.Context, gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
			// The component and its credential appear after reconciliation began.
			live.Spec.Manifest.Components = []types.VMCPComponent{{ID: "new"}}
			require.NoError(t, client.Update(t.Context(), live))
			return []gatewaytypes.Credential{credential}, nil
		},
		delete: func(context.Context, string, string) (bool, error) {
			deleted = true
			return true, nil
		},
	}}
	require.NoError(t, handler.SyncOAuthCredentials(router.Request{Ctx: t.Context(), Client: cached, Object: stale}, nil))
	require.False(t, deleted, "cleanup must check the current definition after listing deletion candidates")
}
