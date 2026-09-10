package mcp

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMigratedVMCPConnectIDsPreserveInstanceAndAudience(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1migrated", Namespace: "default", Spec: v1.VMCPSpec{
		LegacySlug: "mcp1legacy",
		Manifest: types.VMCPManifest{
			Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, AllowAllTools: true}},
			Components: []types.VMCPComponent{{ID: "component", Name: "Tools", ForceSingleUser: true}},
		},
	}}
	first := &v1.VMCPInstance{Name: "vmcpi1first", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Unix(1, 0)), Spec: v1.VMCPInstanceSpec{
		UserID: "7", LegacySlug: "ms1first", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: types.VMCPToolSet{"component": []string{"first"}}},
	}}
	second := &v1.VMCPInstance{Name: "vmcpi1second", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Unix(2, 0)), Spec: v1.VMCPInstanceSpec{
		UserID: "7", LegacySlug: "ms1second", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: types.VMCPToolSet{"component": []string{"second"}}},
	}}
	sm := &SessionManager{storageClient: newVMCPTestStorage(vmcp, second, first,
		vmcpComponentServer("ms1componentfirst", first.Name, "7", "component", "Tools", "https://example.com"),
		vmcpComponentServer("ms1componentsecond", second.Name, "7", "component", "Tools", "https://example.com"),
	)}
	for _, test := range []struct{ id, instance, component, tool string }{
		{
			id:        vmcp.Name,
			instance:  first.Name,
			component: "ms1componentfirst",
			tool:      "first",
		},
		{
			id:        vmcp.Spec.LegacySlug,
			instance:  first.Name,
			component: "ms1componentfirst",
			tool:      "first",
		},
		{
			id:        first.Spec.LegacySlug,
			instance:  first.Name,
			component: "ms1componentfirst",
			tool:      "first",
		},
		{
			id:        second.Spec.LegacySlug,
			instance:  second.Name,
			component: "ms1componentsecond",
			tool:      "second",
		},
		{
			id:        second.Name,
			instance:  second.Name,
			component: "ms1componentsecond",
			tool:      "second",
		},
	} {
		t.Run(test.id, func(t *testing.T) {
			id, audience, err := sm.IDAndAudienceFromConnectURL(t.Context(), test.id, "7")
			if err != nil || id != test.id || audience != test.id {
				t.Fatalf("id=%s audience=%s error=%v", id, audience, err)
			}
			_, _, config, err := sm.ServerForActionWithConnectID(t.Context(), test.id, "7")
			if err != nil {
				t.Fatal(err)
			}
			if config.MCPServerName != test.instance || len(config.Components) != 1 || config.Components[0].Name != test.component || len(config.Components[0].Tools) != 1 || config.Components[0].Tools[0].Name != test.tool {
				t.Fatalf("wrong migrated connection: %#v", config)
			}
			if config.AuditLogMetadata["mcpID"] != test.instance || config.AuditLogMetadata["userID"] != "7" {
				t.Fatalf("wrong migrated connection audit attribution: %#v", config.AuditLogMetadata)
			}
			// This is the ID the aggregate gateway uses on its internal loopback.
			loopback, err := sm.ServerConfigForVMCP(t.Context(), config.MCPServerName, "7")
			if err != nil || loopback.Components[0].Name != test.component {
				t.Fatalf("loopback changed instance: %#v, %v", loopback, err)
			}
		})
	}
	for _, id := range []string{second.Spec.LegacySlug, second.Name} {
		if _, err := sm.ServerConfigForVMCP(t.Context(), id, "other-user"); err == nil {
			t.Fatalf("other user resolved %s", id)
		}
	}
}
