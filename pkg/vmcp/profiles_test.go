package vmcp

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// userInfo returns the principal of the user with uid, as an authenticator records it.
func userInfo(uid string, extra map[string][]string) *kuser.DefaultInfo {
	extra = maps.Clone(extra)
	if extra == nil {
		extra = map[string][]string{}
	}
	principal.RecordKind(extra, principal.KindUser)
	return &kuser.DefaultInfo{UID: uid, Extra: extra}
}

// agentInfo returns the principal of the agent with id, as its authenticator records it.
func agentInfo(id uint, extra map[string][]string) *kuser.DefaultInfo {
	extra = maps.Clone(extra)
	if extra == nil {
		extra = map[string][]string{}
	}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Extra: extra}
}

func TestAllowedToolsUnionsAllMatchingProfiles(t *testing.T) {
	tool := func(componentID, name string) types.VMCPToolReference {
		return types.VMCPToolReference{ComponentID: componentID, Name: name}
	}
	components := func(componentID string, names ...string) map[string]types.VMCPComponentSet {
		return map[string]types.VMCPComponentSet{componentID: {AllowedTools: names}}
	}
	u := userInfo("1", map[string][]string{"obot_groups": {"owner"}, "auth_provider_groups": {"team"}})
	u.Groups = []string{"role"}
	profiles := []types.VMCPProfile{
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "role"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"role": {AllowedTools: []string{"tool"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "owner"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"gmail": {AllowedTools: []string{"send"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}, "gmail": {AllowedTools: []string{"read"}}}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
		{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "other"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
	}
	wantGrant := []types.VMCPToolReference{tool("everything", "echo"), tool("gmail", "read"), tool("gmail", "send")}
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
	u := userInfo("1", nil)
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

func TestEnabledComponents(t *testing.T) {
	u := userInfo("1", map[string][]string{"obot_groups": {"team"}})
	components := []types.VMCPComponent{{ID: "one"}, {ID: "two"}, {ID: "three"}}
	profile := func(subject types.Subject, permissions types.VMCPProfilePermissions) types.VMCPProfile {
		return types.VMCPProfile{Subjects: []types.Subject{subject}, Permissions: permissions}
	}
	user := types.Subject{Type: types.SubjectTypeUser, ID: "1"}
	tests := []struct {
		name      string
		vmcpOwner string
		profiles  []types.VMCPProfile
		want      []string
	}{
		{
			name:      "no matching profiles",
			vmcpOwner: "",
			profiles:  []types.VMCPProfile{profile(types.Subject{Type: types.SubjectTypeUser, ID: "2"}, types.VMCPProfilePermissions{AllowAllComponents: true})},
			want:      []string{},
		},
		{
			name:      "allow all components",
			vmcpOwner: "",
			profiles:  []types.VMCPProfile{profile(user, types.VMCPProfilePermissions{AllowAllComponents: true})},
			want:      []string{"one", "two", "three"},
		},
		{
			name:      "entries enable components even without tools",
			vmcpOwner: "",
			profiles: []types.VMCPProfile{profile(user, types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
				"one": {AllowedTools: []string{}},
				"two": {AllowedTools: []string{"echo"}},
			}})},
			want: []string{"one", "two"},
		},
		{
			name:      "profiles are additive",
			vmcpOwner: "",
			profiles: []types.VMCPProfile{
				profile(user, types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"one": {}}}),
				profile(types.Subject{Type: types.SubjectTypeObotGroup, ID: "team"}, types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"three": {}}}),
			},
			want: []string{"one", "three"},
		},
		{
			name:      "personal VMCP ignores profiles",
			vmcpOwner: "1",
			profiles:  nil,
			want:      []string{"one", "two", "three"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := v1.VMCP{Spec: v1.VMCPSpec{UserID: tt.vmcpOwner, Manifest: types.VMCPManifest{Profiles: tt.profiles}}}
			got := []string{}
			for _, component := range EnabledComponents(u, vmcp, slices.Clone(components)) {
				got = append(got, component.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EnabledComponents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstanceGrant(t *testing.T) {
	u := userInfo("1", nil)
	selection := map[string]types.VMCPComponentSet{"one": {AllowedTools: []string{"echo"}}}
	profiles := []types.VMCPProfile{{
		Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"one": {}, "two": {}}},
	}}
	tests := []struct {
		name      string
		vmcpOwner string
		selection map[string]types.VMCPComponentSet
		want      []types.VMCPToolReference
	}{
		{
			name:      "shared VMCP uses profiles",
			vmcpOwner: "",
			selection: nil,
			want:      []types.VMCPToolReference{{ComponentID: "one", Name: "*"}, {ComponentID: "two", Name: "*"}},
		},
		{
			name:      "shared VMCP narrows to selection",
			vmcpOwner: "",
			selection: selection,
			want:      []types.VMCPToolReference{{ComponentID: "one", Name: "echo"}},
		},
		{
			name:      "personal VMCP owner without selection",
			vmcpOwner: "1",
			selection: nil,
			want:      nil,
		},
		{
			name:      "personal VMCP owner selection",
			vmcpOwner: "1",
			selection: selection,
			want:      []types.VMCPToolReference{{ComponentID: "one", Name: "echo"}},
		},
		{
			name:      "personal VMCP of another user",
			vmcpOwner: "2",
			selection: nil,
			want:      []types.VMCPToolReference{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := v1.VMCP{Spec: v1.VMCPSpec{UserID: tt.vmcpOwner, Manifest: types.VMCPManifest{Profiles: profiles}}}
			instance := v1.VMCPInstance{Spec: v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{ComponentSet: tt.selection}}}
			if got := InstanceGrant(u, vmcp, instance); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("InstanceGrant() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMatchingProfileStrengths(t *testing.T) {
	profile := func(name string, s ...types.Subject) types.VMCPProfile {
		return types.VMCPProfile{Name: name, Subjects: s}
	}
	profiles := []types.VMCPProfile{
		profile("everyone", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}),
		profile("users", types.Subject{Type: types.SubjectTypeUser, ID: "*"}),
		profile("agents", types.Subject{Type: types.SubjectTypeAgent, ID: "*"}),
		profile("user-5", types.Subject{Type: types.SubjectTypeUser, ID: "5"}),
		profile("agent-5", types.Subject{Type: types.SubjectTypeAgent, ID: "5"}),
		profile("team", types.Subject{Type: types.SubjectTypeGroup, ID: "team"}),
		profile("admins", types.Subject{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}),
		profile("mixed", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}, types.Subject{Type: types.SubjectTypeAgent, ID: "5"}),
	}
	groups := map[string][]string{"auth_provider_groups": {"team"}, "obot_groups": {types.GroupAdmin}}

	for _, tt := range []struct {
		name string
		u    kuser.Info
		want map[string]subjects.Match
	}{
		{
			name: "user 5 in groups",
			u:    userInfo("5", groups),
			want: map[string]subjects.Match{
				"everyone": subjects.MatchWildcardAll,
				"users":    subjects.MatchWildcardKind,
				"user-5":   subjects.MatchExplicit,
				"team":     subjects.MatchGroup,
				"admins":   subjects.MatchGroup,
				"mixed":    subjects.MatchWildcardAll,
			},
		},
		{
			name: "agent 5 never matches user subjects or groups",
			u:    agentInfo(5, groups),
			want: map[string]subjects.Match{
				"everyone": subjects.MatchWildcardAll,
				"agents":   subjects.MatchWildcardKind,
				"agent-5":  subjects.MatchExplicit,
				"mixed":    subjects.MatchExplicit,
			},
		},
		{
			name: "agent 6 matches only the wildcards",
			u:    agentInfo(6, nil),
			want: map[string]subjects.Match{
				"everyone": subjects.MatchWildcardAll,
				"agents":   subjects.MatchWildcardKind,
				"mixed":    subjects.MatchWildcardAll,
			},
		},
		{
			name: "principal without a kind matches nothing",
			u:    &kuser.DefaultInfo{UID: "5", Extra: groups},
			want: map[string]subjects.Match{},
		},
		{
			name: "agent UID recorded as a user matches nothing",
			u:    userInfo("agent:5", groups),
			want: map[string]subjects.Match{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]subjects.Match{}
			for _, match := range MatchingProfileStrengths(tt.u, profiles) {
				got[match.Profile.Name] = match.Match
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MatchingProfileStrengths() = %v, want %v", got, tt.want)
			}
			var names []string
			for _, p := range MatchingProfiles(tt.u, profiles) {
				names = append(names, p.Name)
			}
			if len(names) != len(tt.want) {
				t.Fatalf("MatchingProfiles() = %v, want the %d profiles of MatchingProfileStrengths", names, len(tt.want))
			}
		})
	}
}
