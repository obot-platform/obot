package authz

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestMigratedVMCPScopes(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1migrated", Namespace: "default", Spec: v1.VMCPSpec{
		LegacySlug: "mcp1legacy",
		Manifest:   types.VMCPManifest{Profiles: []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, AllowAllTools: true}}},
	}}
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{{ID: "component"}}
	first := &v1.VMCPInstance{Name: "vmcpi1first", Namespace: "default", Spec: v1.VMCPInstanceSpec{LegacySlug: "ms1first", UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	second := &v1.VMCPInstance{Name: "vmcpi1second", Namespace: "default", Spec: v1.VMCPInstanceSpec{LegacySlug: "ms1second", UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	component := &v1.MCPServer{Name: "ms1component", Namespace: "default", Spec: v1.MCPServerSpec{VMCPInstanceID: second.Name, UserID: "7"}}
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, first, second, component)
	for _, test := range []struct {
		scope   string
		id      string
		allowed bool
	}{
		{
			scope:   vmcp.Name,
			id:      first.Spec.LegacySlug,
			allowed: true,
		},
		{
			scope:   vmcp.Spec.LegacySlug,
			id:      second.Name,
			allowed: true,
		},
		{
			scope:   vmcp.Spec.LegacySlug,
			id:      component.Name,
			allowed: true,
		},
		{
			scope:   first.Spec.LegacySlug,
			id:      vmcp.Name,
			allowed: true,
		},
		{
			scope:   second.Spec.LegacySlug,
			id:      vmcp.Name,
			allowed: false,
		},
		{
			scope:   first.Spec.LegacySlug,
			id:      second.Name,
			allowed: false,
		},
		{
			scope:   first.Spec.LegacySlug,
			id:      component.Name,
			allowed: false,
		},
		{
			scope:   second.Spec.LegacySlug,
			id:      component.Name,
			allowed: true,
		},
		{
			scope:   second.Name,
			id:      second.Spec.LegacySlug,
			allowed: true,
		},
	} {
		t.Run(test.scope+"/"+test.id, func(t *testing.T) {
			allowed, err := MCPIDIsAuthorized(t.Context(), storage, []string{test.scope}, "7", test.id)
			if err != nil || allowed != test.allowed {
				t.Fatalf("allowed=%v error=%v", allowed, err)
			}
		})
	}
	for _, id := range []string{vmcp.Spec.LegacySlug, first.Spec.LegacySlug, second.Spec.LegacySlug, second.Name, component.Name} {
		allowed, err := CheckMCPIDAccess(t.Context(), storage, nil, &user.DefaultInfo{UID: "7"}, id)
		if err != nil || allowed != (id != component.Name) {
			t.Fatalf("owner access to %s: allowed=%v error=%v", id, allowed, err)
		}
		if allowed, _ := CheckMCPIDAccess(t.Context(), storage, nil, &user.DefaultInfo{UID: "other"}, id); allowed {
			t.Fatalf("other user allowed %s", id)
		}
	}
	vmcp.Spec.Manifest.Profiles = nil
	if err := storage.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if allowed, err := CheckMCPIDAccess(t.Context(), storage, nil, &user.DefaultInfo{UID: "7"}, second.Spec.LegacySlug); err != nil || allowed {
		t.Fatalf("revoked profile allowed=%v error=%v", allowed, err)
	}
}
