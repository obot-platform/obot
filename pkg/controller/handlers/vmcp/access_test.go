package vmcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPruneUnauthorizedComponents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		components []string
		want       []string
		shared     bool
		lostGroup  bool
		userError  bool
		deleted    bool
	}{
		{
			name:       "prune only denied entry",
			components: []string{"allowed", "denied", "workspace"},
			want:       []string{"allowed", "workspace"},
		},
		{
			name:       "missing source preserves snapshot",
			components: []string{"missing", "denied"},
			want:       []string{"missing"},
		},
		{
			name:       "last denied entry deletes VMCP",
			components: []string{"denied"},
			deleted:    true,
		},
		{
			name: "empty personal VMCP is retained",
		},
		{
			name:       "shared VMCP is unchanged",
			shared:     true,
			components: []string{"denied"},
			want:       []string{"denied"},
		},
		{
			name:       "group membership loss",
			components: []string{"allowed"},
			lostGroup:  true,
			deleted:    true,
		},
		{
			name:       "user lookup failure preserves VMCP",
			components: []string{"denied", "allowed"},
			userError:  true,
			want:       []string{"denied", "allowed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{UserID: "1"}}
			if tc.shared {
				vmcp.Spec.UserID = ""
			}
			for _, id := range tc.components {
				vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, types.VMCPComponent{
					ID:                      id,
					MCPServerCatalogEntryID: id,
					CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Name: "cached-" + id}},
				})
			}
			grants := types.VMCPToolSet{}
			for _, id := range tc.components {
				grants[id] = []string{"*"}
			}
			vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{{Name: "tools", AllowedTools: grants}}
			objects := []kclient.Object{vmcp, &v1.PowerUserWorkspace{
				Name: "workspace", Namespace: system.DefaultNamespace,
				Spec: v1.PowerUserWorkspaceSpec{UserID: "1"},
			}}
			for _, id := range []string{"allowed", "denied", "workspace"} {
				entry := &v1.MCPServerCatalogEntry{
					Name: id, Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{MCPCatalogName: id},
				}
				if id == "workspace" {
					entry.Spec.MCPCatalogName = ""
					entry.Spec.PowerUserWorkspaceID = id
				}
				objects = append(objects, entry)
			}
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
			indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
				"selectors":           func(any) ([]string, error) { return []string{"*"}, nil },
				"catalog-entry-names": func(any) ([]string, error) { return nil, nil },
			})
			if err := indexer.Add(&v1.AccessControlRule{
				Name: "grant", Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: "allowed",
					Manifest:     types.AccessControlRuleManifest{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}},
				},
			}); err != nil {
				t.Fatal(err)
			}
			handler := Handler{
				acrHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, client),
				userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
					if tc.shared {
						t.Fatal("shared VMCP should not look up user access")
					}
					if id != 1 {
						t.Fatalf("looked up user %d, want owner 1", id)
					}
					if tc.userError {
						return nil, errors.New("user lookup failed")
					}
					owner := &kuser.DefaultInfo{UID: "1"}
					if !tc.lostGroup {
						owner.Extra = map[string][]string{"auth_provider_groups": {"team"}}
					}
					return owner, nil
				},
			}
			// Repeated reconciliation must neither recreate nor further prune state.
			for range 2 {
				err := handler.PruneUnauthorizedComponents(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, &router.ResponseWrapper{})
				if (err != nil) != tc.userError {
					t.Fatalf("reconcile error = %v", err)
				}
				var stored v1.VMCP
				err = client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &stored)
				if tc.deleted {
					if !apierrors.IsNotFound(err) {
						t.Fatalf("expected VMCP deletion, got %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				var got []string
				for _, component := range stored.Spec.Manifest.Components {
					got = append(got, component.ID)
					if component.CatalogEntry.Manifest.Name != "cached-"+component.ID {
						t.Fatal("changed retained snapshot")
					}
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("remaining components = %v, want %v", got, tc.want)
				}
				wantGrants := types.VMCPToolSet{}
				if len(tc.components) == 0 {
					wantGrants = nil
				}
				for _, id := range tc.want {
					wantGrants[id] = []string{"*"}
				}
				if !reflect.DeepEqual(stored.Spec.Manifest.Profiles[0].AllowedTools, wantGrants) {
					t.Fatalf("profile grants = %#v, want %#v", stored.Spec.Manifest.Profiles[0].AllowedTools, wantGrants)
				}
				vmcp = &stored
			}
		})
	}
}
