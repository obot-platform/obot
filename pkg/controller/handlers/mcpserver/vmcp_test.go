package mcpserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigratedConfigurationAndFixedValueRotation(t *testing.T) {
	component := types.VMCPComponent{ID: "one", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}}
	vmcp := &v1.VMCP{Name: "vmcp1migration", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                           types.VMCPManifest{ForceSingleUser: true, Components: []types.VMCPComponent{component}},
		StaticConfigurationHash:            "original",
		ComponentStaticConfigurationHashes: map[string]string{component.ID: "original"},
	}}
	legacy := *component.DeepCopy()
	legacy.SourceDigest = utils.Digest([]any{component, vmcp.Spec.ComponentStaticConfigurationHashes[component.ID]})
	legacy.Configuration[0].Policy = types.VMCPConfigurationPolicyUserAllowed
	instance := &v1.VMCPInstance{Name: "vmcpi1migration", Namespace: vmcp.Namespace, Spec: v1.VMCPInstanceSpec{
		UserID: "user", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}, LegacyComponents: []types.VMCPComponent{legacy},
	}, Status: v1.VMCPInstanceStatus{UserConfigurationHash: "user-hash"}}
	server := &v1.MCPServer{Name: "ms1migration", Namespace: vmcp.Namespace, Spec: v1.MCPServerSpec{
		UserID: "user", VMCPInstanceID: instance.Name, VMCPComponentID: component.ID,
	}}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance, server).WithStatusSubresource(server).Build()
	gw := newTestGatewayClient(t)
	key := vmcpconfig.ConfigurationKey(component.ID, "TOKEN")
	require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name), Name: vmcpconfig.ConfigurationCredentialName(), Secrets: map[string]string{key: "admin"},
	}))
	require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name), Name: vmcpconfig.ConfigurationCredentialName(), Secrets: map[string]string{key: "legacy-override"},
	}))
	handler := &Handler{gatewayClient: gw}
	req := router.Request{Ctx: t.Context(), Client: storage, Object: server}
	require.NoError(t, handler.SyncVMCPConfiguration(req, nil))
	credential, err := gw.RevealCredential(t.Context(), []string{instance.Name + "-" + server.Name}, server.Name)
	require.NoError(t, err)
	require.Equal(t, "legacy-override", credential.Secrets["TOKEN"])
	vmcp.Spec.StaticConfigurationHash = "rotated"
	vmcp.Spec.ComponentStaticConfigurationHashes[component.ID] = "rotated"
	require.NoError(t, storage.Update(t.Context(), vmcp))
	require.NoError(t, handler.SyncVMCPConfiguration(req, nil))
	credential, err = gw.RevealCredential(t.Context(), []string{instance.Name + "-" + server.Name}, server.Name)
	require.NoError(t, err)
	require.Equal(t, "admin", credential.Secrets["TOKEN"], "fixed configuration must retire the legacy override")
}

func TestVMCPOAuthCredentialStatusWithoutCatalogSource(t *testing.T) {
	ref := system.MCPOAuthCredentialName("deleted-source")
	vmcp := &v1.VMCP{Name: "vmcp1oauth", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one", OAuthCredentialID: ref}},
	}}}
	server := &v1.MCPServer{Name: "ms1oauth", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "one"}}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, server).WithStatusSubresource(server).Build()
	gw := newTestGatewayClient(t)
	handler := &Handler{gatewayClient: gw}
	for i, configured := range []bool{false, true, false} {
		// Changing the retained reference must invalidate the previous result.
		ref = system.MCPOAuthCredentialName(fmt.Sprintf("deleted-source-%d", i))
		vmcp.Spec.Manifest.Components[0].OAuthCredentialID = ref
		require.NoError(t, storage.Update(t.Context(), vmcp))
		if configured {
			if err := gw.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: ref, Name: system.StaticOAuthCredentialName, Secrets: map[string]string{"CLIENT_ID": "client"}}); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := gw.DeleteCredential(t.Context(), ref, system.StaticOAuthCredentialName); err != nil {
				t.Fatal(err)
			}
		}
		if err := handler.SyncOAuthCredentialStatus(router.Request{Ctx: t.Context(), Client: storage, Object: server}, nil); err != nil {
			t.Fatal(err)
		}
		if server.Status.OAuthCredentialConfigured != configured {
			t.Fatalf("configured = %v, want %v", server.Status.OAuthCredentialConfigured, configured)
		}
		// An unchanged result, including not-found, must not access the gateway.
		require.NoError(t, (&Handler{}).SyncOAuthCredentialStatus(router.Request{Ctx: t.Context(), Client: storage, Object: server}, nil))
	}
}

func TestVMCPOAuthCredentialStatusRechecksSourceRevision(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{Name: "source", Namespace: "default", Annotations: map[string]string{v1.OAuthCredentialRevisionAnnotation: "one"}}
	ref := system.MCPOAuthCredentialName(entry.Name)
	vmcp := &v1.VMCP{Name: "vmcp1oauth", Namespace: entry.Namespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one", MCPServerCatalogEntryID: entry.Name, OAuthCredentialID: ref}},
	}}}
	server := &v1.MCPServer{Name: "ms1oauth", Namespace: entry.Namespace, Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "one"}}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(entry, vmcp, server).WithStatusSubresource(server).Build()
	gw := newTestGatewayClient(t)
	handler := &Handler{gatewayClient: gw}
	req := router.Request{Ctx: t.Context(), Client: storage, Object: server}
	require.NoError(t, handler.SyncOAuthCredentialStatus(req, nil))
	require.False(t, server.Status.OAuthCredentialConfigured)
	require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: ref, Name: system.StaticOAuthCredentialName, Secrets: map[string]string{"CLIENT_ID": "client"}}))
	entry.Annotations[v1.OAuthCredentialRevisionAnnotation] = "two"
	require.NoError(t, storage.Update(t.Context(), entry))
	require.NoError(t, handler.SyncOAuthCredentialStatus(req, nil))
	require.True(t, server.Status.OAuthCredentialConfigured)
	require.NoError(t, (&Handler{}).SyncOAuthCredentialStatus(req, nil))
	_, err := gw.DeleteCredential(t.Context(), ref, system.StaticOAuthCredentialName)
	require.NoError(t, err)
	entry.Annotations[v1.OAuthCredentialRevisionAnnotation] = "three"
	require.NoError(t, storage.Update(t.Context(), entry))
	require.NoError(t, handler.SyncOAuthCredentialStatus(req, nil))
	require.False(t, server.Status.OAuthCredentialConfigured)
}

func TestSyncVMCPConfigurationCopiesComponentConfiguration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	component := types.VMCPComponent{
		ID:   "component-one",
		Name: "one",
		Configuration: []types.VMCPConfigurationPolicy{
			{
				Key:    "STATIC",
				Policy: types.VMCPConfigurationPolicyFixed,
			},
			{
				Key:    "HEADER",
				Policy: types.VMCPConfigurationPolicyUserAllowed,
			},
			{
				Key:    "PROHIBITED",
				Policy: types.VMCPConfigurationPolicyProhibited,
			},
		},
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{component},
			},
			StaticConfigurationHash: "static-hash",
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1test",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
			UserID: "user-1",
		},
		Status: v1.VMCPInstanceStatus{
			UserConfigurationHash: "user-hash",
		},
	}
	server := &v1.MCPServer{
		Name:      "ms1test",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(vmcp, instance, server).
		Build()
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), server); err != nil {
		t.Fatal(err)
	}

	gatewayClient := newTestGatewayClient(t)
	staticKey := vmcpconfig.ConfigurationKey(component.ID, "STATIC")
	userKey := vmcpconfig.ConfigurationKey(component.ID, "HEADER")
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			staticKey: "admin-value",
			userKey:   "wrong-static-source",
			vmcpconfig.ConfigurationKey(component.ID, "PROHIBITED"):  "prohibited-value",
			vmcpconfig.ConfigurationKey("other-component", "STATIC"): "other-value",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			userKey:   "user-value",
			staticKey: "wrong-user-source",
		},
	}); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{gatewayClient: gatewayClient}
	if err := handler.SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	credential, err := gatewayClient.RevealCredential(t.Context(),
		[]string{instance.Name + "-" + server.Name},
		server.Name,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(credential.Secrets) != 2 || credential.Secrets["STATIC"] != "admin-value" || credential.Secrets["HEADER"] != "user-value" {
		t.Fatalf("MCPServer credential = %#v, want fixed and user-allowed component values", credential.Secrets)
	}

	var updated v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.VMCPStaticConfigurationHash != vmcp.Spec.StaticConfigurationHash {
		t.Fatalf("static configuration hash = %q, want %q", updated.Status.VMCPStaticConfigurationHash, vmcp.Spec.StaticConfigurationHash)
	}
	if updated.Status.VMCPUserConfigurationHash != instance.Status.UserConfigurationHash {
		t.Fatalf("user configuration hash = %q, want %q", updated.Status.VMCPUserConfigurationHash, instance.Status.UserConfigurationHash)
	}
}

func TestSyncVMCPConfigurationSkipsMatchingHashes(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			StaticConfigurationHash: "static-hash",
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1test",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
			UserID: "user-1",
		},
		Status: v1.VMCPInstanceStatus{
			UserConfigurationHash: "user-hash",
		},
	}
	server := &v1.MCPServer{
		Name:      "ms1test",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			UserID:          "user-1",
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "component-one",
		},
		Status: v1.MCPServerStatus{
			VMCPStaticConfigurationHash: vmcp.Spec.StaticConfigurationHash,
			VMCPUserConfigurationHash:   instance.Status.UserConfigurationHash,
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(vmcp, instance, server).
		Build()
	gatewayClient := newTestGatewayClient(t)

	if err := (&Handler{gatewayClient: gatewayClient}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	_, err := gatewayClient.RevealCredential(t.Context(),
		[]string{instance.Name + "-" + server.Name},
		server.Name,
	)
	if !errors.As(err, &client.CredentialNotFoundError{}) {
		t.Fatalf("expected no MCPServer credential to be written, got %v", err)
	}
}

func TestSyncVMCPSharedConfiguration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	component := types.VMCPComponent{
		ID: "one",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "HEADER", Usage: types.Header}},
		}},
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "STATIC", Policy: types.VMCPConfigurationPolicyFixed},
			{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
	}
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{
		StaticConfigurationHash: "new-hash",
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
	}}
	server := &v1.MCPServer{Name: "ms1shared", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: component.ID}}
	instance := &v1.VMCPInstance{Name: "vmcpi1old", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	stale := &v1.MCPServer{Name: "ms1old", Namespace: "default", Spec: v1.MCPServerSpec{VMCPInstanceID: instance.Name, UserID: "1", VMCPComponentID: component.ID}}
	storage := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.MCPServer{}).WithObjects(vmcp, server, instance, stale).Build()
	gatewayClient := newTestGatewayClient(t)
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "STATIC"): "fixed", vmcpconfig.ConfigurationKey(component.ID, "HEADER"): "wrong"},
	}); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{gatewayClient: gatewayClient}
	for _, obj := range []*v1.MCPServer{server, stale} {
		if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatal(err)
		}
		if err := handler.SyncVMCPConfiguration(router.Request{Ctx: t.Context(), Client: storage, Object: obj}, nil); err != nil {
			t.Fatal(err)
		}
	}
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{vmcp.Name + "-" + server.Name}, server.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(credential.Secrets) != 1 || credential.Secrets["STATIC"] != "fixed" {
		t.Fatalf("shared credential = %#v", credential.Secrets)
	}
	if server.Status.VMCPStaticConfigurationHash != "new-hash" || server.Status.VMCPUserConfigurationHash != "" {
		t.Fatal("wrong shared configuration hashes")
	}
	if _, err := gatewayClient.RevealCredential(t.Context(), []string{instance.Name + "-" + stale.Name}, stale.Name); !errors.As(err, &client.CredentialNotFoundError{}) {
		t.Fatalf("obsolete instance credential was written: %v", err)
	}
}
