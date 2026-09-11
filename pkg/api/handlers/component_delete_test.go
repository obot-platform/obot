package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeleteServerProtectsComponents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spec   v1.MCPServerSpec
		parent string
	}{
		{
			name: "standalone",
		},
		{
			name:   "composite component",
			spec:   v1.MCPServerSpec{CompositeName: "ms1composite"},
			parent: "ms1composite",
		},
		{
			name: "shared vMCP component",
			spec: v1.MCPServerSpec{
				VMCPID:          "vmcp1shared",
				VMCPComponentID: "one",
			},
			parent: "vmcp1shared",
		},
		{
			name: "dedicated vMCP component",
			spec: v1.MCPServerSpec{
				VMCPInstanceID:  "vmcpi1dedicated",
				VMCPComponentID: "one",
			},
			parent: "vmcpi1dedicated",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &v1.MCPServer{Name: "ms1test", Namespace: system.DefaultNamespace, Spec: tc.spec}
			server.Spec.UserID = "1"
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(server).Build()
			request := httptest.NewRequest(http.MethodDelete, "/", nil)
			request.SetPathValue("mcp_server_id", server.Name)
			err := (&MCPHandler{}).DeleteServer(api.Context{
				Request:        request,
				ResponseWriter: httptest.NewRecorder(),
				Storage:        client,
				User:           &kuser.DefaultInfo{UID: "1"},
			})
			if tc.parent != "" {
				var httpErr *types.ErrHTTP
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, http.StatusForbidden, httpErr.Code)
				require.ErrorContains(t, err, tc.parent)
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{}))
			} else {
				require.NoError(t, err)
				require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{})))
			}
		})
	}
}

func TestDeleteServerInstanceProtectsCompositeComponents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		composite string
		missing   bool
	}{
		{
			name: "standalone",
		},
		{
			name:      "composite component",
			composite: "ms1composite",
		},
		{
			name:    "already deleted",
			missing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := &v1.MCPServerInstance{
				Name:      "msi1test",
				Namespace: system.DefaultNamespace,
				Spec:      v1.MCPServerInstanceSpec{CompositeName: tc.composite},
			}
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			if !tc.missing {
				require.NoError(t, client.Create(t.Context(), instance))
			}
			request := httptest.NewRequest(http.MethodDelete, "/", nil)
			request.SetPathValue("mcp_server_instance_id", instance.Name)
			err := (&ServerInstancesHandler{}).DeleteServerInstance(api.Context{Request: request, Storage: client})
			if tc.composite != "" {
				var httpErr *types.ErrHTTP
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, http.StatusBadRequest, httpErr.Code)
				require.ErrorContains(t, err, tc.composite)
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.MCPServerInstance{}))
			} else {
				require.NoError(t, err)
				require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.MCPServerInstance{})))
			}
		})
	}
}
