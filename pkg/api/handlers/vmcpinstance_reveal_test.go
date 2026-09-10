package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestVMCPInstanceRevealReturnsOnlyAllowedConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-reveal",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{
			{ID: "allowed", Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}}},
			{ID: "fixed", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}},
		}}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi-reveal",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	gatewayClient := newHandlerTestGateway(t)
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			vmcpconfig.ConfigurationKey("allowed", "HEADER"): "saved-value",
			vmcpconfig.ConfigurationKey("fixed", "TOKEN"):    "fixed-value",
			vmcpconfig.ConfigurationKey("removed", "VALUE"):  "removed-value",
		},
	}); err != nil {
		t.Fatalf("store configuration: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/reveal", nil)
	request.SetPathValue("vmcp_instance_id", instance.Name)
	err := NewVMCPInstanceHandler().Reveal(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        newVMCPTestStorage(vmcp, instance),
		GatewayClient:  gatewayClient,
		User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
	})
	if err != nil {
		t.Fatalf("Reveal() error = %v", err)
	}
	var got types.VMCPConfiguration
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := types.VMCPConfiguration{Components: map[string]map[string]string{
		"allowed": {"HEADER": "saved-value"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configuration = %#v, want %#v", got, want)
	}
}

func TestVMCPInstanceRevealReturnsEmptyConfigurationWhenMissing(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp-reveal", Namespace: system.DefaultNamespace}
	instance := &v1.VMCPInstance{Name: "vmcpi-reveal", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/reveal", nil)
	request.SetPathValue("vmcp_instance_id", instance.Name)
	err := NewVMCPInstanceHandler().Reveal(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        newVMCPTestStorage(vmcp, instance),
		GatewayClient:  newHandlerTestGateway(t),
		User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
	})
	if err != nil {
		t.Fatalf("Reveal() error = %v", err)
	}
	var got types.VMCPConfiguration
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(got, types.VMCPConfiguration{Components: map[string]map[string]string{}}) {
		t.Fatalf("configuration = %#v, want empty configuration", got)
	}
}
