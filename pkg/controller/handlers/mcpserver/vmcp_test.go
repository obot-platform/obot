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
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
		[]string{fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)},
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
		[]string{fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)},
		server.Name,
	)
	if !errors.As(err, &client.CredentialNotFoundError{}) {
		t.Fatalf("expected no MCPServer credential to be written, got %v", err)
	}
}
