package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCheckVMCPComponentAuthRequiresMatchingApprovedRequest(t *testing.T) {
	vmcp := vmcpComponentVMCP(true)
	instance := vmcpComponentInstance("vmcpi1selected", vmcp.Name)
	component := vmcpComponentServer("ms1selected", instance.Name, vmcp.Name)
	for _, tt := range []struct {
		name    string
		request v1.OAuthAuthRequestSpec
		wantErr bool
	}{
		{
			name: "approved matching request",
			request: v1.OAuthAuthRequestSpec{
				UserID:          42,
				MCPID:           instance.Name,
				ConsentApproved: true,
			},
		},
		{
			name: "different user",
			request: v1.OAuthAuthRequestSpec{
				UserID:          7,
				MCPID:           instance.Name,
				ConsentApproved: true,
			},
			wantErr: true,
		},
		{
			name: "different connection",
			request: v1.OAuthAuthRequestSpec{
				UserID:          42,
				MCPID:           vmcp.Name,
				ConsentApproved: true,
			},
			wantErr: true,
		},
		{
			name: "unapproved",
			request: v1.OAuthAuthRequestSpec{
				UserID: 42,
				MCPID:  instance.Name,
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authRequest := &v1.OAuthAuthRequest{
				Name:      "oar1request",
				Namespace: system.DefaultNamespace,
				Spec:      tt.request,
			}
			storage := vmcpConsentStorage(vmcp, instance, component, authRequest)
			recorder := httptest.NewRecorder()
			req := vmcpComponentRequest(storage, instance.Name, component.Name)
			req.ResponseWriter = recorder
			req.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/vmcp/"+instance.Name+"/components/"+component.Name+"?oauth_auth_request="+authRequest.Name, nil)
			req.SetPathValue("mcp_id", instance.Name)
			req.SetPathValue("component_mcp_id", component.Name)

			err := (&handler{}).checkVMCPComponentAuth(req)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, recorder.Body.String())
				return
			}
			require.NoError(t, err)
			var response componentAuthStatus
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
			require.Empty(t, response.AuthURL)
		})
	}
}

func vmcpComponentVMCP(singleUser bool) *v1.VMCP {
	return &v1.VMCP{
		Name:      "vmcp1shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{ID: "component", ForceSingleUser: singleUser}},
		}},
	}
}

func vmcpComponentInstance(name, vmcpID string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{UserID: "42", Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID}},
	}
}

func vmcpComponentServer(name, instanceID, vmcpID string) *v1.MCPServer {
	return &v1.MCPServer{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			Manifest:        types.MCPServerManifest{Runtime: types.RuntimeContainerized},
			UserID:          "42",
			VMCPID:          vmcpID,
			VMCPInstanceID:  instanceID,
			VMCPComponentID: "component",
		},
	}
}

func vmcpComponentRequest(storage storage.Client, vmcpID, componentID string) api.Context {
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/vmcp/"+vmcpID+"/components/"+componentID, nil)
	request.SetPathValue("mcp_id", vmcpID)
	request.SetPathValue("component_mcp_id", componentID)
	return api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		User:           &user.DefaultInfo{Name: "user", UID: "42", Groups: []string{types.GroupAuthenticated}},
	}
}
