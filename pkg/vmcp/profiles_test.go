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
	toolSet := func(componentID string, names ...string) types.VMCPToolSet {
		return types.VMCPToolSet{componentID: names}
	}
	u := &kuser.DefaultInfo{
		UID:    "1",
		Groups: []string{"role"},
		Extra:  map[string][]string{"obot_groups": {"owner"}, "auth_provider_groups": {"team"}},
	}
	profiles := []types.VMCPProfile{
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, AllowedTools: toolSet("everything", "echo")},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "role"}}, AllowedTools: toolSet("role", "tool")},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "owner"}}, AllowedTools: toolSet("gmail", "send")},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}, AllowedTools: types.VMCPToolSet{"everything": []string{"echo"}, "gmail": []string{"read"}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "other"}}, AllowAllTools: true},
	}
	wantGrant := []types.VMCPToolReference{tool("everything", "echo"), tool("gmail", "read"), tool("gmail", "send"), tool("role", "tool")}
	if got := AllowedTools(u, profiles, nil); !reflect.DeepEqual(got, wantGrant) {
		t.Fatalf("union = %v", got)
	}
	if got := AllowedTools(u, profiles, types.VMCPToolSet{"everything": []string{"echo"}, "gmail": []string{"forbidden"}}); !reflect.DeepEqual(got, []types.VMCPToolReference{tool("everything", "echo")}) {
		t.Fatalf("selection widened grant: %v", got)
	}
	for _, ps := range [][]types.VMCPProfile{nil, profiles[4:5]} {
		if got := AllowedTools(u, ps, nil); len(got) != 0 {
			t.Fatalf("empty grant = %#v", got)
		}
		if got := AllowedTools(u, ps, toolSet("everything", "echo")); len(got) != 0 {
			t.Fatalf("selection widened empty grant: %#v", got)
		}
	}
	profiles[4].AllowAllTools = true
	if got := AllowedTools(u, profiles, nil); got != nil {
		t.Fatalf("allow all = %v", got)
	}
	if got := AllowedTools(u, profiles, types.VMCPToolSet{}); got == nil || len(got) != 0 {
		t.Fatalf("empty selection = %#v", got)
	}
	if got := AllowedTools(u, profiles, toolSet("new-component", "new-tool")); !reflect.DeepEqual(got, []types.VMCPToolReference{tool("new-component", "new-tool")}) {
		t.Fatalf("all-tools selection = %v", got)
	}
}

func TestComponentWildcardSelections(t *testing.T) {
	u := &kuser.DefaultInfo{UID: "1"}
	profiles := []types.VMCPProfile{{
		Subjects:     []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}},
		AllowedTools: types.VMCPToolSet{"gmail": {"*"}, "everything": {"echo"}},
	}}
	selection := types.VMCPToolSet{"gmail": {"list_emails"}, "everything": {"echo", "other"}}
	want := []types.VMCPToolReference{{ComponentID: "everything", Name: "echo"}, {ComponentID: "gmail", Name: "list_emails"}}
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("wildcard selection = %#v, want %#v", got, want)
	}
	selection = types.VMCPToolSet{"gmail": {"*"}, "everything": {"*"}}
	want[1].Name = "*"
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("component intersection = %#v, want %#v", got, want)
	}
	profiles[0].AllowedTools["gmail"] = []string{"list_emails"}
	want[1].Name = "list_emails"
	if got := AllowedTools(u, profiles, selection); !reflect.DeepEqual(got, want) {
		t.Fatalf("revoked wildcard = %#v, want %#v", got, want)
	}
}
