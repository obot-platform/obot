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
	component := types.VMCPComponent{ID: "one", ForceSingleUser: true, Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}}
	vmcp := &v1.VMCP{Name: "vmcp1migration", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                           types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash:            "original",
		StaticConfigurationCredentialName:  "configuration-revision",
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
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name), Name: vmcpconfig.StaticConfigurationCredentialName(vmcp), Secrets: map[string]string{key: "admin"},
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
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
				ID: "component-one", ForceSingleUser: true,
			}}},
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
		Name:        "ms1test",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-1"},
		Spec: v1.MCPServerSpec{
			UserID:          "user-1",
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "component-one",
		},
		Status: v1.MCPServerStatus{
			VMCPStaticConfigurationHash: vmcp.Spec.StaticConfigurationHash,
			VMCPUserConfigurationHash:   instance.Status.UserConfigurationHash,
			VMCPSnapshotHash:            "snapshot-1",
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
		Manifest: types.VMCPManifest{Components: []types.VMCPComponent{
			component,
			{ID: "single", ForceSingleUser: true},
		}},
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

// remoteTemplateComponent builds a component whose remote URL is driven by a user-supplied
// value, the shape that broke vMCP connections to servers like Mixpanel.
func remoteTemplateComponent(usage types.Usage) types.VMCPComponent {
	return types.VMCPComponent{
		ID:   "component-one",
		Name: "one",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "MIXPANEL_HOST", Usage: usage, Required: true}},
		}},
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "MIXPANEL_HOST", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
	}
}

func remoteTemplateManifest() types.MCPServerManifest {
	return types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Config:  []types.MCPConfig{{Key: "MIXPANEL_HOST", Usage: types.Env, Required: true}},
		RemoteConfig: &types.RemoteRuntimeConfig{
			IsTemplate:  true,
			URLTemplate: "https://${MIXPANEL_HOST}/mcp",
		},
	}
}

func TestSyncVMCPConfigurationResolvesRemoteURLTemplate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	// A non-header user input keeps the VMCP single-user, so each instance owns its server.
	component := remoteTemplateComponent(types.Env)
	vmcp := &v1.VMCP{Name: "vmcp1url", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash",
	}}
	instance := &v1.VMCPInstance{Name: "vmcpi1url", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		UserID:   "user-1",
	}, Status: v1.VMCPInstanceStatus{UserConfigurationHash: "user-hash"}}
	server := &v1.MCPServer{
		Name:        "ms1url",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-1"},
		Spec: v1.MCPServerSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
			Manifest:        remoteTemplateManifest(),
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
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "MIXPANEL_HOST"): "mcp.mixpanel.com"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := (&Handler{gatewayClient: gatewayClient}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var stored v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Spec.Manifest.RemoteConfig.URL; got != "https://mcp.mixpanel.com/mcp" {
		t.Fatalf("resolved URL = %q, want %q", got, "https://mcp.mixpanel.com/mcp")
	}
	if stored.Status.VMCPSnapshotHash != "snapshot-1" {
		t.Fatalf("snapshot hash = %q, want %q", stored.Status.VMCPSnapshotHash, "snapshot-1")
	}
}

func TestSyncVMCPConfigurationLeavesPerUserSharedURLTemplateUnresolved(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	// A header-typed user input keeps the VMCP multi-user, so one server serves every user
	// and no single expansion of the template is correct.
	component := remoteTemplateComponent(types.Header)
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash",
	}}
	server := &v1.MCPServer{
		Name:        "ms1sharedurl",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-1"},
		Spec: v1.MCPServerSpec{
			VMCPID:          vmcp.Name,
			VMCPComponentID: component.ID,
			Manifest:        remoteTemplateManifest(),
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(vmcp, server).
		Build()
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), server); err != nil {
		t.Fatal(err)
	}

	if err := (&Handler{gatewayClient: newTestGatewayClient(t)}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var stored v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Spec.Manifest.RemoteConfig.URL; got != "" {
		t.Fatalf("URL = %q, want it left for the runtime to resolve per request", got)
	}
}

func TestSyncVMCPConfigurationResolvesURLAfterSnapshotRebuild(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	component := remoteTemplateComponent(types.Env)
	vmcp := &v1.VMCP{Name: "vmcp1rebuild", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash",
	}}
	instance := &v1.VMCPInstance{Name: "vmcpi1rebuild", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		UserID:   "user-1",
	}, Status: v1.VMCPInstanceStatus{UserConfigurationHash: "user-hash"}}
	// The owning controller just rewrote Spec.Manifest from a new catalog entry snapshot,
	// dropping the resolved URL without touching either configuration hash.
	server := &v1.MCPServer{
		Name:        "ms1rebuild",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-2"},
		Spec: v1.MCPServerSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
			Manifest:        remoteTemplateManifest(),
		},
		Status: v1.MCPServerStatus{
			VMCPStaticConfigurationHash: vmcp.Spec.StaticConfigurationHash,
			VMCPUserConfigurationHash:   instance.Status.UserConfigurationHash,
			VMCPSnapshotHash:            "snapshot-1",
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
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "MIXPANEL_HOST"): "mcp.eu.mixpanel.com"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := (&Handler{gatewayClient: gatewayClient}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var stored v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Spec.Manifest.RemoteConfig.URL; got != "https://mcp.eu.mixpanel.com/mcp" {
		t.Fatalf("resolved URL = %q, want %q", got, "https://mcp.eu.mixpanel.com/mcp")
	}
	if stored.Status.VMCPSnapshotHash != "snapshot-2" {
		t.Fatalf("snapshot hash = %q, want %q", stored.Status.VMCPSnapshotHash, "snapshot-2")
	}
}

func TestSyncVMCPConfigurationClearsURLWhenSharedTemplateBecomesPerUser(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	// The policy just moved from fixed to user-allowed, so the URL resolved while it was fixed
	// no longer describes every user of this shared server.
	component := remoteTemplateComponent(types.Header)
	vmcp := &v1.VMCP{Name: "vmcp1flip", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash-2",
	}}
	manifest := remoteTemplateManifest()
	manifest.RemoteConfig.URL = "https://mcp.mixpanel.com/mcp"
	server := &v1.MCPServer{
		Name:        "ms1flip",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-1"},
		Spec: v1.MCPServerSpec{
			VMCPID:          vmcp.Name,
			VMCPComponentID: component.ID,
			Manifest:        manifest,
		},
		Status: v1.MCPServerStatus{
			VMCPStaticConfigurationHash: "static-hash-1",
			VMCPSnapshotHash:            "snapshot-1",
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(vmcp, server).
		Build()
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), server); err != nil {
		t.Fatal(err)
	}

	if err := (&Handler{gatewayClient: newTestGatewayClient(t)}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var stored v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Spec.Manifest.RemoteConfig.URL; got != "" {
		t.Fatalf("URL = %q, want it cleared so the runtime expands the template per request", got)
	}
}

// Regression test for the URL template host advisory: the runtime uses a stored URL without
// re-expanding the template, so the controller must never store one whose host a value moved,
// and must clear a previously stored valid URL rather than keep it.
func TestSyncVMCPConfigurationClearsURLWhenValueChangesTemplateHost(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	// A non-header user input keeps the VMCP single-user, so each instance owns its server.
	component := types.VMCPComponent{
		ID:   "component-one",
		Name: "one",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "REGION", Usage: types.Env, Required: true}},
		}},
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
	}
	vmcp := &v1.VMCP{Name: "vmcp1host", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash",
	}}
	instance := &v1.VMCPInstance{Name: "vmcpi1host", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		UserID:   "user-1",
	}, Status: v1.VMCPInstanceStatus{UserConfigurationHash: "user-hash"}}
	server := &v1.MCPServer{
		Name:        "ms1host",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPSnapshotDigestAnnotation: "snapshot-1"},
		Spec: v1.MCPServerSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				Config:  []types.MCPConfig{{Key: "REGION", Usage: types.Env, Required: true}},
				RemoteConfig: &types.RemoteRuntimeConfig{
					IsTemplate:  true,
					URLTemplate: "https://${REGION}.api.example.com/mcp",
					// Resolved from an earlier, valid value.
					URL: "https://us.api.example.com/mcp",
				},
			},
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
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "REGION"): "attacker.example/x?"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := (&Handler{gatewayClient: gatewayClient}).SyncVMCPConfiguration(router.Request{
		Ctx:    t.Context(),
		Client: storageClient,
		Object: server,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var stored v1.MCPServer
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Spec.Manifest.RemoteConfig.URL; got != "" {
		t.Fatalf("stored URL = %q, want it cleared", got)
	}
}

func TestResolvedComponentURLTreatsFixedValuesAsAdminText(t *testing.T) {
	// DD_SITE forms the domain. Only the fixed policy on a shared vMCP says an admin set it; the
	// configuration map alone can't tell it apart from the user's TENANT.
	component := types.VMCPComponent{
		ID: "component-one",
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "TENANT", Policy: types.VMCPConfigurationPolicyUserAllowed},
			{Key: "DD_SITE", Policy: types.VMCPConfigurationPolicyFixed},
		},
	}
	server := &v1.MCPServer{Spec: v1.MCPServerSpec{
		VMCPInstanceID: "instance-1",
		Manifest: types.MCPServerManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{URLTemplate: "https://${TENANT}.mcp.${DD_SITE}/api/mcp"},
		},
	}}

	url, owned := resolvedComponentURL(component, server, map[string]string{"TENANT": "acme", "DD_SITE": "datadoghq.eu"}, true)
	if !owned || url != "https://acme.mcp.datadoghq.eu/api/mcp" {
		t.Fatalf("resolvedComponentURL() = %q, %v; want the expanded URL", url, owned)
	}

	// Any user can own a personal vMCP and fix its values, so there a fixed value is not admin text.
	if url, _ := resolvedComponentURL(component, server, map[string]string{"TENANT": "acme", "DD_SITE": "attacker.example"}, false); url != "" {
		t.Fatalf("resolvedComponentURL() = %q; want it rejected when a personal vMCP's fixed value sets the domain", url)
	}

	// The same values with DD_SITE left to the user would let the user choose the domain.
	component.Configuration[1].Policy = types.VMCPConfigurationPolicyUserAllowed
	if url, _ := resolvedComponentURL(component, server, map[string]string{"TENANT": "acme", "DD_SITE": "datadoghq.eu"}, true); url != "" {
		t.Fatalf("resolvedComponentURL() = %q; want it rejected when the user sets the domain", url)
	}
}

// TestSyncVMCPConfigurationGivesAgentsOnlyFixedValues checks that an agent's dedicated component server is given the
// values the vMCP fixes and never a value a user would enter, even one stored for the agent's instance, which is never
// read for an agent.
func TestSyncVMCPConfigurationGivesAgentsOnlyFixedValues(t *testing.T) {
	component := types.VMCPComponent{
		ID:   "component-one",
		Name: "one",
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "STATIC", Policy: types.VMCPConfigurationPolicyFixed},
			{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
	}
	vmcp := &v1.VMCP{Name: "vmcp1agent", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest:                types.VMCPManifest{Components: []types.VMCPComponent{component}},
		StaticConfigurationHash: "static-hash",
	}}
	instance := &v1.VMCPInstance{Name: "vmcpi1agent", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		UserID:   "agent:7",
	}}
	server := &v1.MCPServer{Name: "ms1agent", Namespace: "default", Spec: v1.MCPServerSpec{
		UserID:          instance.Spec.UserID,
		VMCPInstanceID:  instance.Name,
		VMCPComponentID: component.ID,
	}}
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.MCPServer{}).WithObjects(vmcp, instance, server).Build()
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(server), server))

	gatewayClient := newTestGatewayClient(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "STATIC"): "admin-value"},
	}))
	// What an earlier Obot copied to the agent's instance from its profile.
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "REGION"): "stored-value"},
	}))

	handler := &Handler{gatewayClient: gatewayClient}
	require.NoError(t, handler.SyncVMCPConfiguration(router.Request{Ctx: t.Context(), Client: storageClient, Object: server}, nil))
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"STATIC": "admin-value"}, credential.Secrets)
}
