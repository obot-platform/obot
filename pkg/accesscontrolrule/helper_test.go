package accesscontrolrule

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

func TestUserHasAccessToMCPServerCatalogEntryInCatalogMatchesGroups(t *testing.T) {
	tests := []struct {
		name      string
		user      kuser.Info
		subjectID string
		want      bool
	}{
		{
			name: "owner matches inherited admin role group",
			user: &kuser.DefaultInfo{
				UID:    "owner",
				Groups: types.RoleOwner.Groups(),
			},
			subjectID: types.GroupAdmin,
			want:      true,
		},
		{
			name: "auth provider group still matches",
			user: &kuser.DefaultInfo{
				UID: "member",
				Extra: map[string][]string{
					"auth_provider_groups": {"idp-team"},
				},
			},
			subjectID: "idp-team",
			want:      true,
		},
		{
			name: "unrelated group does not match",
			user: &kuser.DefaultInfo{
				UID:    "basic",
				Groups: []string{types.GroupBasic},
			},
			subjectID: types.GroupAdmin,
			want:      false,
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
							Type: types.SubjectTypeGroup,
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
