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
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newVMCPStatusTestStorage returns storage that keeps vMCP and instance status apart from the spec and checks resource
// versions on both, as the API server does.
func newVMCPStatusTestStorage(objects ...kclient.Object) *vmcpTestStorage {
	return &vmcpTestStorage{WithWatch: clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.VMCP{}, &v1.VMCPInstance{}).
		WithObjects(objects...).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.UserID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		Build()}
}

// agentProfilesTestVMCP returns a stored vMCP whose component has a required and an optional value a user would
// enter and a value the vMCP fixes, with a profile for agent 7, one for agent 8 and one for every user.
func agentProfilesTestVMCP() *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: "vmcp1profiles", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Profiles",
		Components: []types.VMCPComponent{{
			ID:                      "api",
			Name:                    "api",
			MCPServerCatalogEntryID: "entry",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
				{Key: "REGION", Required: true},
				{Key: "VERBOSE"},
				{Key: "ADMIN_TOKEN", Required: true},
			}}},
			Configuration: []types.VMCPConfigurationPolicy{
				{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
			},
		}},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile("vp1eight", types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
			profile("vp1users", types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		},
	}}}
}

func requireHTTPStatus(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
}

// TestVMCPInstanceConfigurationRefusedForAgents checks that an agent's instance cannot be configured, deconfigured or
// revealed through the instance API, even by an administrator: agents get only values fixed for everyone, so no one can
// give an agent a value a user would enter. Whatever an older Obot stored for the instance is left as it is, and never
// returned.
func TestVMCPInstanceConfigurationRefusedForAgents(t *testing.T) {
	vmcp := agentProfilesTestVMCP()
	instance := &v1.VMCPInstance{Name: "vmcpi1agent", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
		UserID:   "agent:7",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
	}}
	storage := newVMCPStatusTestStorage(vmcp, instance)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("api", "REGION"): "stored-secret"},
	}))
	handler := NewVMCPInstanceHandler()
	for _, tt := range []struct {
		action string
		call   func(api.Context) error
	}{
		{
			action: "configure",
			call:   handler.Configure,
		},
		{
			action: "reveal",
			call:   handler.Reveal,
		},
		{
			action: "deconfigure",
			call:   handler.Deconfigure,
		},
	} {
		t.Run(tt.action, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/"+tt.action, strings.NewReader(`{"components":{"api":{"REGION":"entered"}}}`))
			request.SetPathValue("vmcp_instance_id", instance.Name)
			recorder := httptest.NewRecorder()
			err := tt.call(api.Context{
				ResponseWriter: recorder,
				Request:        request,
				Storage:        storage,
				GatewayClient:  gatewayClient,
				User:           vmcpAdminForTest(),
			})
			requireHTTPStatus(t, err, http.StatusConflict)
			require.NotContains(t, recorder.Body.String(), "stored-secret")
		})
	}
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	require.NoError(t, err)
	require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("api", "REGION"): "stored-secret"}, credential.Secrets)
}
