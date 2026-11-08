package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestComponentName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "GitHub", want: "github"},
		{in: "  Linear  MCP!! ", want: "linear-mcp"},
		{in: "@modelcontextprotocol/server-filesystem", want: "modelcontextprotocol-server-filesystem"},
		{in: "!!!", want: "server"},
	}
	for _, tt := range tests {
		if got := componentName(tt.in); got != tt.want {
			t.Errorf("componentName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUniqueComponentName(t *testing.T) {
	components := []types.VMCPComponent{{Name: "github"}, {Name: "github-2"}}
	if got := uniqueComponentName(components, "github"); got != "github-3" {
		t.Fatalf("got %q, want github-3", got)
	}
	if got := uniqueComponentName(components, "linear"); got != "linear" {
		t.Fatalf("got %q, want linear", got)
	}
}

func TestGrantRequester(t *testing.T) {
	spec := v1.MCPServerRequestSpec{RequesterID: "3", RequesterEmail: "dev@example.com"}
	manifest := types.VMCPManifest{
		DisplayName: "Engineering",
		Components:  []types.VMCPComponent{{ID: "c1", Name: "sentry", MCPServerCatalogEntryID: "e1"}, {ID: "c2", Name: "notion", MCPServerCatalogEntryID: "e2"}},
		Profiles: []types.VMCPProfile{{
			Name:        "default",
			Subjects:    []types.Subject{{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		}},
	}

	grantRequester(&manifest, spec, "sentry")
	if len(manifest.Profiles) != 2 {
		t.Fatalf("expected a requester profile, got %+v", manifest.Profiles)
	}
	p := manifest.Profiles[1]
	if p.Name != "Requested by dev@example.com" || p.Permissions.AllowAllComponents || len(p.Permissions.AllowedComponents) != 1 {
		t.Fatalf("unexpected profile %+v", p)
	}
	if set, ok := p.Permissions.AllowedComponents["c1"]; !ok || set.AllowedTools != nil {
		t.Fatalf("expected every tool of c1, got %+v", p.Permissions.AllowedComponents)
	}

	// A second approval for the same requester extends their profile.
	grantRequester(&manifest, spec, "notion")
	if len(manifest.Profiles) != 2 || len(manifest.Profiles[1].Permissions.AllowedComponents) != 2 {
		t.Fatalf("expected the profile to grow, got %+v", manifest.Profiles)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequesterHasComponent(t *testing.T) {
	requester := &kuser.DefaultInfo{Name: "dev", UID: "3", Extra: map[string][]string{"obot_groups": {types.GroupBasic}}}
	vmcp := v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "c1", Name: "deepwiki"}, {ID: "c2", Name: "semgrep"}},
		Profiles: []types.VMCPProfile{
			{Name: "admins", Subjects: []types.Subject{{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			{Name: "dev", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "3"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"c1": {}}}},
		},
	}}}
	if !requesterHasComponent(requester, vmcp, "deepwiki") {
		t.Fatal("dev's profile grants deepwiki")
	}
	if requesterHasComponent(requester, vmcp, "semgrep") {
		t.Fatal("dev's profile doesn't grant semgrep, though dev can connect to the vMCP")
	}
	admin := &kuser.DefaultInfo{Name: "a", UID: "1", Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}}
	if !requesterHasComponent(admin, vmcp, "semgrep") {
		t.Fatal("admins are granted every component")
	}
}
