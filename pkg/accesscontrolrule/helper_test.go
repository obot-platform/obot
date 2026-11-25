package accesscontrolrule

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

func TestUserHasAccessToMCPServerCatalogEntryInCatalogMatchesGroups(t *testing.T) {
	tests := []struct {
		name        string
		user        kuser.Info
		subjectType types.SubjectType
		subjectID   string
		want        bool
	}{
		{
			name: "owner role group does not match auth provider group",
			user: &kuser.DefaultInfo{
				UID:    "owner",
				Groups: types.RoleOwner.Groups(),
			},
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			want:        false,
		},
		{
			name: "obot admin group does not match auth provider group",
			user: &kuser.DefaultInfo{
				UID: "admin",
				Extra: map[string][]string{
					"obot_groups": {types.GroupAdmin},
				},
			},
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			want:        false,
		},
		{
			name: "auth provider admin group matches auth provider group",
			user: &kuser.DefaultInfo{
				UID: "member",
				Extra: map[string][]string{
					"auth_provider_groups": {types.GroupAdmin},
				},
			},
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			want:        true,
		},
		{
			name: "auth provider group still matches",
			user: &kuser.DefaultInfo{
				UID: "member",
				Extra: map[string][]string{
					"auth_provider_groups": {"idp-team"},
				},
			},
			subjectType: types.SubjectTypeGroup,
			subjectID:   "idp-team",
			want:        true,
		},
		{
			name: "unrelated group does not match",
			user: &kuser.DefaultInfo{
				UID:    "basic",
				Groups: []string{types.GroupBasic},
			},
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			want:        false,
		},
		{
			name: "obot admin matches obot groups",
			user: &kuser.DefaultInfo{
				UID: "admin",
				Extra: map[string][]string{
					"obot_groups": {types.GroupAdmin},
				},
			},
			subjectType: types.SubjectTypeObotGroup,
			subjectID:   types.GroupAdmin,
			want:        true,
		},
		{
			name: "obot group ignores token groups",
			user: &kuser.DefaultInfo{
				UID:    "admin",
				Groups: []string{types.GroupAdmin},
			},
			subjectType: types.SubjectTypeObotGroup,
			subjectID:   types.GroupAdmin,
			want:        false,
		},
		{
			name: "obot group does not match auth provider group",
			user: &kuser.DefaultInfo{
				UID: "member",
				Extra: map[string][]string{
					"auth_provider_groups": {types.GroupAdmin},
				},
			},
			subjectType: types.SubjectTypeObotGroup,
			subjectID:   types.GroupAdmin,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, map[string]gocache.IndexFunc{
				"selectors": func(obj any) ([]string, error) {
					rule := obj.(*v1.AccessControlRule)
					return resourceIDs(rule, types.ResourceTypeSelector), nil
				},
				"catalog-entry-names": func(obj any) ([]string, error) {
					rule := obj.(*v1.AccessControlRule)
					return resourceIDs(rule, types.ResourceTypeMCPServerCatalogEntry), nil
				},
			})
			rule := &v1.AccessControlRule{
				Name:      "acr-test",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{{
							Type: tt.subjectType,
							ID:   tt.subjectID,
						}},
						Resources: []types.Resource{{
							Type: types.ResourceTypeSelector,
							ID:   "*",
						}},
					},
				},
			}
			if err := indexer.Add(rule); err != nil {
				t.Fatal(err)
			}

			h := NewAccessControlRuleHelper(indexer, nil)
			got, err := h.UserHasAccessToMCPServerCatalogEntryInCatalog(tt.user, "entry", system.DefaultCatalog)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("access = %t, want %t", got, tt.want)
			}
		})
	}
}

func resourceIDs(rule *v1.AccessControlRule, resourceType types.ResourceType) []string {
	var ids []string
	for _, resource := range rule.Spec.Manifest.Resources {
		if resource.Type == resourceType {
			ids = append(ids, resource.ID)
		}
	}
	return ids
}

// TestAgentsNeverMatchAccessControlRules checks that no subject of an access control rule, the selector wildcard
// included, grants an agent access, in a catalog or a workspace, while the same rules grant users access.
func TestAgentsNeverMatchAccessControlRules(t *testing.T) {
	const workspaceID = "workspace"
	everySubject := []types.Subject{
		{Type: types.SubjectTypeSelector, ID: "*"},
		{Type: types.SubjectTypeUser, ID: "5"},
		{Type: types.SubjectTypeUser, ID: "agent:5"},
		{Type: types.SubjectTypeGroup, ID: "eng"},
		{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
	}
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, map[string]gocache.IndexFunc{
		"selectors": func(obj any) ([]string, error) {
			return resourceIDs(obj.(*v1.AccessControlRule), types.ResourceTypeSelector), nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			return resourceIDs(obj.(*v1.AccessControlRule), types.ResourceTypeMCPServerCatalogEntry), nil
		},
		"server-names": func(obj any) ([]string, error) {
			return resourceIDs(obj.(*v1.AccessControlRule), types.ResourceTypeMCPServer), nil
		},
	})
	for _, rule := range []*v1.AccessControlRule{
		{
			Name:      "catalog",
			Namespace: system.DefaultNamespace,
			Spec: v1.AccessControlRuleSpec{
				MCPCatalogID: system.DefaultCatalog,
				Manifest: types.AccessControlRuleManifest{
					Subjects:  everySubject,
					Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
				},
			},
		},
		{
			Name:      "workspace",
			Namespace: system.DefaultNamespace,
			Spec: v1.AccessControlRuleSpec{
				PowerUserWorkspaceID: workspaceID,
				Manifest: types.AccessControlRuleManifest{
					Subjects:  everySubject,
					Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
				},
			},
		},
	} {
		if err := indexer.Add(rule); err != nil {
			t.Fatal(err)
		}
	}
	h := NewAccessControlRuleHelper(indexer, nil)

	withKind := func(uid string, kind principal.Kind) kuser.Info {
		extra := map[string][]string{
			"auth_provider_groups": {"eng"},
			"obot_groups":          {types.GroupAdmin},
		}
		principal.RecordKind(extra, kind)
		return &kuser.DefaultInfo{UID: uid, Extra: extra}
	}

	checks := map[string]func(kuser.Info) (bool, error){
		"server in catalog": func(u kuser.Info) (bool, error) {
			return h.UserHasAccessToMCPServerInCatalog(u, "server", system.DefaultCatalog)
		},
		"entry in catalog": func(u kuser.Info) (bool, error) {
			return h.UserHasAccessToMCPServerCatalogEntryInCatalog(u, "entry", system.DefaultCatalog)
		},
		"server in workspace": func(u kuser.Info) (bool, error) {
			return h.UserHasAccessToMCPServerInWorkspace(u, "server", workspaceID, "")
		},
		"server in workspace owned by the caller's UID": func(u kuser.Info) (bool, error) {
			return h.UserHasAccessToMCPServerInWorkspace(u, "server", "other-workspace", u.GetUID())
		},
		"entry in workspace": func(u kuser.Info) (bool, error) {
			return h.UserHasAccessToMCPServerCatalogEntryInWorkspace(t.Context(), u, "entry", workspaceID)
		},
	}

	for _, tt := range []struct {
		name string
		user kuser.Info
		want bool
	}{
		{
			name: "user 5",
			user: withKind("5", principal.KindUser),
			want: true,
		},
		{
			name: "agent 5",
			user: withKind("agent:5", principal.KindAgent),
			want: false,
		},
		{
			name: "agent UID recorded as a user",
			user: withKind("agent:5", principal.KindUser),
			want: false,
		},
		{
			name: "decimal UID recorded as an agent",
			user: withKind("5", principal.KindAgent),
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, check := range checks {
				got, err := check(tt.user)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if got != tt.want {
					t.Errorf("%s: access = %t, want %t", name, got, tt.want)
				}
			}
		})
	}
}
