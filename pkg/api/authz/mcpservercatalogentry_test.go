package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAllMCPCatalogEntryAuthorizationUsesAccessControlRules(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "entry-test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
		},
	}).Build()
	authorizer := newCatalogEntryTestAuthorizer(t, storage, &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "entry-access",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed-user"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "entry-test"}},
			},
		},
	})

	tests := []struct {
		name    string
		userID  string
		allowed bool
	}{
		{name: "user with entry ACR is allowed", userID: "allowed-user", allowed: true},
		{name: "user without entry ACR is denied", userID: "other-user", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries/entry-test", nil)
			u := &user.DefaultInfo{
				Name:   tt.userID,
				UID:    tt.userID,
				Groups: []string{types.GroupAPI, types.GroupAuthenticated},
			}

			if allowed := authorizer.Authorize(req, u); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}

func newCatalogEntryTestAuthorizer(t *testing.T, storage client.Client, acrs ...*v1.AccessControlRule) *Authorizer {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeMCPServerCatalogEntry {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"server-names": func(any) ([]string, error) {
			return nil, nil
		},
		"selectors": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	})

	for _, acr := range acrs {
		if err := indexer.Add(acr); err != nil {
			t.Fatalf("add access control rule to indexer: %v", err)
		}
	}

	return NewAuthorizer(nil, storage, storage, false, accesscontrolrule.NewAccessControlRuleHelper(indexer, storage), nil, nil, false)
}
