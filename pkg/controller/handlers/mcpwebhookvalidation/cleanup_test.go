package mcpwebhookvalidation

import (
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupResourcesPreservesVMCP(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1existing", Namespace: system.DefaultNamespace}
	server := &v1.MCPServer{Name: "ms1existing", Namespace: system.DefaultNamespace}
	want := []types.Resource{
		{Type: types.ResourceTypeMCPServer, ID: vmcp.Name},
		{Type: types.ResourceTypeMCPServer, ID: server.Name},
	}
	validation := &v1.MCPWebhookValidation{}
	validation.Name = "mwv1test"
	validation.Namespace = system.DefaultNamespace
	validation.Spec.Manifest.Resources = append(want, types.Resource{
		Type: types.ResourceTypeMCPServer,
		ID:   "vmcp1deleted",
	})
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, server, validation).Build()
	if err := (&Handler{}).CleanupResources(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    validation,
		Namespace: system.DefaultNamespace,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(validation.Spec.Manifest.Resources, want) {
		t.Fatalf("resources = %#v, want %#v", validation.Spec.Manifest.Resources, want)
	}
}
