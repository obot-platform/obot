package apiproxy

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
)

func TestWorkerReferenceUsesCurrentUserConnection(t *testing.T) {
	const componentID = "component"
	vmcp := &v1.VMCP{
		ObjectMeta: metav1.ObjectMeta{Name: "vmcp1test", Namespace: system.DefaultNamespace},
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{
				ID: componentID, Name: "API",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Name: "API", Runtime: types.RuntimeOpenAPI,
				}},
			}},
			Profiles: []types.VMCPProfile{{
				Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			}},
		}},
	}
	instance := &v1.VMCPInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vmcpi1test", Namespace: system.DefaultNamespace},
		Spec:       v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}, UserID: "1"},
	}
	connection := &v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "msi1test", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerInstanceSpec{
			VMCPInstanceID: instance.Name, VMCPComponentID: componentID,
			UserID: "1", MCPServerName: "ms1test",
		},
	}
	otherUserConnection := connection.DeepCopy()
	otherUserConnection.Name = "msi1other"
	otherUserConnection.Spec.UserID = "2"

	storage := kclientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(vmcp, instance, connection, otherUserConnection).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(object client.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(object client.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.UserID}
		}).
		Build()
	adapter := NewWorkerExecutorAdapter(nil, storage)

	reference, err := adapter.workerReference(t.Context(), &kuser.DefaultInfo{UID: "1"}, Reference{
		VMCPID: vmcp.Name, ComponentID: componentID,
	})

	require.NoError(t, err)
	require.Equal(t, connection.Name, reference.InstanceName)
	require.Equal(t, componentID, reference.ComponentID)
}
