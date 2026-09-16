package vmcp

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAllowedToolsUnionsAllMatchingProfiles(t *testing.T) {
	tool := func(componentID, name string) types.VMCPToolReference {
		return types.VMCPToolReference{ComponentID: componentID, Name: name}
	}
	components := func(componentID string, names ...string) map[string]types.VMCPComponentSet {
		return map[string]types.VMCPComponentSet{componentID: {AllowedTools: names}}
	}
	u := &kuser.DefaultInfo{
		UID:    "1",
		Groups: []string{"role"},
		Extra:  map[string][]string{"obot_groups": {"owner"}, "auth_provider_groups": {"team"}},
	}
	profiles := []types.VMCPProfile{
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "role"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"role": {AllowedTools: []string{"tool"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "owner"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"gmail": {AllowedTools: []string{"send"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}, "gmail": {AllowedTools: []string{"read"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "other"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
	}
	wantGrant := []types.VMCPToolReference{tool("everything", "echo"), tool("gmail", "read"), tool("gmail", "send"), tool("role", "tool")}
	if got := AllowedTools(u, profiles, nil); !reflect.DeepEqual(got, wantGrant) {
		t.Fatalf("union = %v", got)
	}
	if got := AllowedTools(u, profiles, map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}, "gmail": {AllowedTools: []string{"forbidden"}}}); !reflect.DeepEqual(got, []types.VMCPToolReference{tool("everything", "echo")}) {
		t.Fatalf("selection widened grant: %v", got)
	}
	for _, ps := range [][]types.VMCPProfile{nil, profiles[4:5]} {
		if got := AllowedTools(u, ps, nil); len(got) != 0 {
			t.Fatalf("empty grant = %#v", got)
		}
		if got := AllowedTools(u, ps, components("everything", "echo")); len(got) != 0 {
			t.Fatalf("selection widened empty grant: %#v", got)
		}
	}
	profiles[4].Permissions.AllowAllComponents = true
	profiles[4].Permissions.AllowedComponents = map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{}}}
	if got := AllowedTools(u, profiles, nil); got != nil {
		t.Fatalf("allow all = %v", got)
	}
	if got := AllowedTools(u, profiles, map[string]types.VMCPComponentSet{}); got == nil || len(got) != 0 {
		t.Fatalf("empty selection = %#v", got)
	}
	if got := AllowedTools(u, profiles, components("new-component", "new-tool")); !reflect.DeepEqual(got, []types.VMCPToolReference{tool("new-component", "new-tool")}) {
		t.Fatalf("all-tools selection = %v", got)
	}
	if got := AllowedTools(u, profiles, components("everything", "new-tool")); !reflect.DeepEqual(got, []types.VMCPToolReference{tool("everything", "new-tool")}) {
		t.Fatalf("allow all components did not override the explicit tool restriction: %v", got)
	}
}

func TestComponentWildcardSelections(t *testing.T) {
	u := &kuser.DefaultInfo{UID: "1"}
	profiles := []types.VMCPProfile{{
		Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"gmail": {}, "everything": {AllowedTools: []string{"echo"}}}},
	}}
	selection := map[string]types.VMCPComponentSet{"gmail": {AllowedTools: []string{"list_emails"}}, "everything": {AllowedTools: []string{"echo", "other"}}}
	want := []types.VMCPToolReference{{ComponentID: "everything", Name: "echo"}, {ComponentID: "gmail", Name: "list_emails"}}
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("wildcard selection = %#v, want %#v", got, want)
	}
	selection = map[string]types.VMCPComponentSet{"gmail": {}, "everything": {}}
	want[1].Name = "*"
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("component intersection = %#v, want %#v", got, want)
	}
	profiles[0].Permissions.AllowedComponents["gmail"] = types.VMCPComponentSet{AllowedTools: []string{"list_emails"}}
	want[1].Name = "list_emails"
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("revoked wildcard = %#v, want %#v", got, want)
	}
	profiles[0].Permissions.AllowedComponents["gmail"] = types.VMCPComponentSet{AllowedTools: []string{}}
	for _, selection := range []map[string]types.VMCPComponentSet{nil, selection} {
		if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want[:1]) {
			t.Fatalf("empty component tools = %#v, want %#v", got, want[:1])
		}
	}
}
