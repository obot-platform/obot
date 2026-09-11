package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVMCPRevealReturnsOnlyFixedConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-reveal",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{
			{ID: "fixed", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}},
			{ID: "allowed", Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}}},
		}}},
	}
	tests := []struct {
		name    string
		secrets map[string]string
		want    types.VMCPConfiguration
	}{
		{
			name: "configured",
			secrets: map[string]string{
				vmcpconfig.ConfigurationKey("fixed", "TOKEN"):    "fixed-value",
				vmcpconfig.ConfigurationKey("allowed", "HEADER"): "user-value",
				vmcpconfig.ConfigurationKey("removed", "VALUE"):  "removed-value",
			},
			want: types.VMCPConfiguration{Components: map[string]map[string]string{
				"fixed": {"TOKEN": "fixed-value"},
			}},
		},
		{
			name: "missing credential",
			want: types.VMCPConfiguration{Components: map[string]map[string]string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newHandlerTestGateway(t)
			if tt.secrets != nil {
				require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
					Name:    vmcpconfig.ConfigurationCredentialName(),
					Secrets: tt.secrets,
				}))
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/reveal", nil)
			request.SetPathValue("vmcp_id", vmcp.Name)
			require.NoError(t, NewVMCPHandler(nil).Reveal(api.Context{
				ResponseWriter: recorder,
				Request:        request,
				Storage:        newVMCPTestStorage(vmcp),
				GatewayClient:  gatewayClient,
			}))
			var got types.VMCPConfiguration
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVMCPDeconfigureDeletesConfigurationAndPublishesHashes(t *testing.T) {
	component := types.VMCPComponent{ID: "component", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}}
	vmcp := &v1.VMCP{Name: "vmcp-deconfigure", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}}}}
	secrets := map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "secret"}
	vmcpconfig.SetStaticConfigurationHashes(vmcp, secrets)
	storage := newVMCPTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: secrets,
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/deconfigure", nil)
	request.SetPathValue("vmcp_id", vmcp.Name)
	require.NoError(t, NewVMCPHandler(nil).Deconfigure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}))

	_, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)}, vmcpconfig.ConfigurationCredentialName())
	var notFound gateway.CredentialNotFoundError
	require.ErrorAs(t, err, &notFound)
	var updated v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &updated))
	emptyHash := utils.Digest(map[string]string{})
	assert.Equal(t, emptyHash, updated.Spec.StaticConfigurationHash)
	assert.Equal(t, emptyHash, updated.Spec.ComponentStaticConfigurationHashes[component.ID])
}

func TestVMCPInstanceDeconfigureDeletesConfigurationAndTriggersSync(t *testing.T) {
	instance := &v1.VMCPInstance{
		Name:        "vmcpi-deconfigure",
		Namespace:   system.DefaultNamespace,
		Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "old"},
	}
	storage := newVMCPTestStorage(instance)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{"secret": "value"},
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/deconfigure", nil)
	request.SetPathValue("vmcp_instance_id", instance.Name)
	require.NoError(t, NewVMCPInstanceHandler().Deconfigure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}))

	_, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	var notFound gateway.CredentialNotFoundError
	require.ErrorAs(t, err, &notFound)
	var updated v1.VMCPInstance
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updated))
	assert.Equal(t, utils.Digest(map[string]string{}), updated.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
}
