package cleanup

import (
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUserCleanupVMCPs(t *testing.T) {
	userDelete := &v1.UserDelete{
		Name:      "delete-user",
		Namespace: system.DefaultNamespace,
		Spec:      v1.UserDeleteSpec{UserID: 1},
	}
	builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(userDelete)
	for _, obj := range []kclient.Object{
		&v1.HostedAgentInstance{}, &v1.HostedAgentPoolAssignment{}, &v1.Project{},
		&v1.VMCP{}, &v1.VMCPInstance{}, &v1.MCPServer{}, &v1.MCPServerInstance{}, &v1.PowerUserWorkspace{},
	} {
		builder.WithIndex(obj, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(fields.Fields).Get("spec.userID")}
		})
	}
	var deleted, retained []kclient.Object
	for _, userID := range []string{"1", "2"} {
		objects := []kclient.Object{
			&v1.VMCP{Name: "vmcp-" + userID, Spec: v1.VMCPSpec{UserID: userID}},
			&v1.VMCPInstance{Name: "instance-" + userID, Spec: v1.VMCPInstanceSpec{UserID: userID}},
			&v1.MCPServer{Name: "server-" + userID, Spec: v1.MCPServerSpec{UserID: userID}},
			&v1.MCPServerInstance{Name: "server-instance-" + userID, Spec: v1.MCPServerInstanceSpec{UserID: userID}},
		}
		if userID == "1" {
			deleted = append(deleted, objects...)
		} else {
			retained = append(retained, objects...)
		}
	}
	retained = append(retained,
		&v1.VMCP{Name: "shared"},
		&v1.MCPServer{Name: "component", Spec: v1.MCPServerSpec{UserID: "1", VMCPComponentID: "component"}},
		&v1.MCPServerInstance{Name: "component-instance", Spec: v1.MCPServerInstanceSpec{UserID: "1", VMCPComponentID: "component"}},
	)
	for _, objects := range [][]kclient.Object{deleted, retained} {
		for _, obj := range objects {
			obj.SetNamespace(system.DefaultNamespace)
			builder.WithObjects(obj)
		}
	}
	client := builder.Build()
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(any) ([]string, error) { return nil, nil },
	})
	handler := NewUserCleanup(newCredentialsCleanupGatewayClient(t), accesscontrolrule.NewAccessControlRuleHelper(indexer, client))
	require.NoError(t, handler.Cleanup(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    userDelete,
		Namespace: system.DefaultNamespace,
	}, nil))
	for _, obj := range append(deleted, userDelete) {
		err := client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj)
		require.True(t, apierrors.IsNotFound(err), "expected %T %s deleted, got %v", obj, obj.GetName(), err)
	}
	// Component resources are left for reference cleanup; shared and other users' resources survive.
	for _, obj := range retained {
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	}
}

func TestSavedPoolIDs(t *testing.T) {
	userDelete := &v1.UserDelete{
		Annotations: map[string]string{
			hostedAgentPoolCleanupAnnotation: `["pool-a","pool-b"]`,
		},
	}
	got, err := savedPoolIDs(userDelete)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"pool-a", "pool-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pool IDs = %#v, want %#v", got, want)
	}
}

func TestSavedPoolIDsRejectsInvalidCheckpoint(t *testing.T) {
	userDelete := &v1.UserDelete{
		Annotations: map[string]string{
			hostedAgentPoolCleanupAnnotation: "not-json",
		},
	}
	if _, err := savedPoolIDs(userDelete); err == nil {
		t.Fatal("expected invalid cleanup checkpoint to fail")
	}
}
