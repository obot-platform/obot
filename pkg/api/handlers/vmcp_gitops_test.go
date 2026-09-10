package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGitManagedVMCPRejectsManualMutations(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "source-vmcp",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			SourceURL: "https://example.com/vmcps",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
				ID:            "component",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
			}}},
		},
	}
	secrets := map[string]string{"component.TOKEN": "source-secret"}
	vmcpconfig.SetStaticConfigurationHashes(vmcp, secrets)
	storage := newVMCPTestStorage(vmcp)
	gateway := newHandlerTestGateway(t)
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: secrets,
	}))
	handler := NewVMCPHandler(nil)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		call   func(api.Context) error
	}{
		{
			name:   "update",
			method: http.MethodPut,
			path:   "/api/vmcps/source-vmcp",
			body:   `{}`,
			call:   handler.Update,
		},
		{
			name:   "trigger update",
			method: http.MethodPost,
			path:   "/api/vmcps/source-vmcp/trigger-update",
			call:   handler.TriggerUpdate,
		},
		{
			name:   "deconfigure",
			method: http.MethodPost,
			path:   "/api/vmcps/source-vmcp/deconfigure",
			call:   handler.Deconfigure,
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   "/api/vmcps/source-vmcp",
			call:   handler.Delete,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.SetPathValue("vmcp_id", "source-vmcp")
			err := tc.call(api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage, GatewayClient: gateway})
			if err == nil || !strings.Contains(err.Error(), "not editable") {
				t.Fatalf("expected source-managed VMCP mutation to be rejected, got %v", err)
			}
		})
	}

	var persisted v1.VMCP
	if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "source-vmcp"}, &persisted); err != nil {
		t.Fatalf("source-managed VMCP was changed: %v", err)
	}
	assert.Equal(t, vmcp.Spec, persisted.Spec)
	credential, err := gateway.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)}, vmcpconfig.ConfigurationCredentialName())
	require.NoError(t, err)
	assert.Equal(t, secrets, credential.Secrets)
}

func TestNonGitManagedVMCPAllowsTriggerUpdateAndDelete(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec v1.VMCPSpec
	}{
		{name: "ordinary"},
		{
			name: "detached",
			spec: v1.VMCPSpec{
				SourceURL: "https://example.com/vmcps",
				Detached:  true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.Manifest.DisplayName = "Managed vMCP"
			storage := newVMCPTestStorage(&v1.VMCP{
				Name:      "managed-vmcp",
				Namespace: system.DefaultNamespace,
				Spec:      tc.spec,
			})
			handler := NewVMCPHandler(nil)
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps/managed-vmcp/trigger-update", nil)
			request.SetPathValue("vmcp_id", "managed-vmcp")
			if err := handler.TriggerUpdate(api.Context{Request: request, Storage: storage}); err != nil {
				t.Fatalf("trigger update: %v", err)
			}

			request = httptest.NewRequest(http.MethodDelete, "/api/vmcps/managed-vmcp", nil)
			request.SetPathValue("vmcp_id", "managed-vmcp")
			if err := handler.Delete(api.Context{Request: request, Storage: storage}); err != nil {
				t.Fatalf("delete: %v", err)
			}

			var deleted v1.VMCP
			if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "managed-vmcp"}, &deleted); !apierrors.IsNotFound(err) {
				t.Fatalf("VMCP still exists: %v", err)
			}
		})
	}
}
