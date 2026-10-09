package cleanup

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUnassociatedMCPResources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		object  kclient.Object
		deleted bool
	}{
		{
			name:    "standalone server",
			object:  &v1.MCPServer{},
			deleted: true,
		},
		{
			name:    "Nanobot agent deployment without vMCP ownership",
			object:  &v1.MCPServer{Spec: v1.MCPServerSpec{NanobotAgentID: "na1parent"}},
			deleted: true,
		},
		{
			name:   "shared vMCP component",
			object: &v1.MCPServer{Spec: v1.MCPServerSpec{VMCPID: "vmcp1parent"}},
		},
		{
			name:   "single user vMCP component",
			object: &v1.MCPServer{Spec: v1.MCPServerSpec{VMCPInstanceID: "vmcpi1parent"}},
		},
		{
			name:    "standalone instance",
			object:  &v1.MCPServerInstance{},
			deleted: true,
		},
		{
			name:   "vMCP component connection",
			object: &v1.MCPServerInstance{Spec: v1.MCPServerInstanceSpec{VMCPInstanceID: "vmcpi1parent"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.object.SetName("test")
			tc.object.SetNamespace(metav1.NamespaceDefault)
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.object).Build()
			require.NoError(t, UnassociatedMCPResources(router.Request{Ctx: t.Context(), Client: client, Object: tc.object}, nil))
			err := client.Get(t.Context(), kclient.ObjectKeyFromObject(tc.object), tc.object)
			if tc.deleted {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}
