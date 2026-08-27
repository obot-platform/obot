package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func BenchmarkListCatalogEntries(b *testing.B) {
	for _, entryCount := range []int{50, 300} {
		b.Run(fmt.Sprintf("regular_user/entries_%d", entryCount), func(b *testing.B) {
			storageClient, acrHelper := newCatalogEntryBenchmarkFixture(b, entryCount)
			handler := &MCPHandler{
				acrHelper: acrHelper,
				serverURL: "https://example.com",
			}
			user := &kuser.DefaultInfo{
				Name:   "benchmark-user",
				UID:    "benchmark-user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			}
			request := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries", nil)
			newContext := func() (api.Context, *httptest.ResponseRecorder) {
				recorder := httptest.NewRecorder()
				return api.Context{
					Request:        request,
					ResponseWriter: recorder,
					Storage:        storageClient,
					User:           user,
				}, recorder
			}

			validateCatalogEntryBenchmarkResponse(b, handler.ListEntriesFromAllSources, newContext, entryCount)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				req, _ := newContext()
				if err := handler.ListEntriesFromAllSources(req); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("admin_all/entries_%d", entryCount), func(b *testing.B) {
			storageClient, _ := newCatalogEntryBenchmarkFixture(b, entryCount)
			handler := &MCPCatalogHandler{serverURL: "https://example.com"}
			user := &kuser.DefaultInfo{
				Name:   "benchmark-admin",
				UID:    "benchmark-admin",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
			}
			request := httptest.NewRequest(http.MethodGet, "/api/mcp-catalogs/default/entries?all=true", nil)
			request.SetPathValue("catalog_id", system.DefaultCatalog)
			newContext := func() (api.Context, *httptest.ResponseRecorder) {
				recorder := httptest.NewRecorder()
				return api.Context{
					Request:        request,
					ResponseWriter: recorder,
					Storage:        storageClient,
					User:           user,
				}, recorder
			}

			validateCatalogEntryBenchmarkResponse(b, handler.ListEntries, newContext, entryCount)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				req, _ := newContext()
				if err := handler.ListEntries(req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newCatalogEntryBenchmarkFixture(b *testing.B, entryCount int) (storage.Client, *accesscontrolrule.Helper) {
	b.Helper()

	objects := make([]kclient.Object, 0, entryCount+1)
	objects = append(objects, &v1.MCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
	})
	for i := range entryCount {
		name := fmt.Sprintf("benchmark-entry-%03d", i)
		objects = append(objects, &v1.MCPServerCatalogEntry{
			Name:      name,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: system.DefaultCatalog,
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:           name,
					Runtime:        types.RuntimeRemote,
					ServerUserType: types.ServerUserTypeSingleUser,
					RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
				},
			},
		})
	}

	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerCatalogEntry).Spec.MCPCatalogName}
		}).
		Build())

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(obj any) ([]string, error) {
			return accessControlRuleBenchmarkIndex(obj, types.SubjectTypeUser, ""), nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			return accessControlRuleBenchmarkIndex(obj, "", types.ResourceTypeMCPServerCatalogEntry), nil
		},
		"server-names": func(any) ([]string, error) {
			return nil, nil
		},
		"selectors": func(obj any) ([]string, error) {
			return accessControlRuleBenchmarkIndex(obj, "", types.ResourceTypeSelector), nil
		},
	})
	wildcardRule := &v1.AccessControlRule{
		Name:      "benchmark-wildcard",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
			},
		},
	}
	if err := indexer.Add(wildcardRule); err != nil {
		b.Fatal(err)
	}

	return storageClient, accesscontrolrule.NewAccessControlRuleHelper(indexer, storageClient)
}

func accessControlRuleBenchmarkIndex(obj any, subjectType types.SubjectType, resourceType types.ResourceType) []string {
	rule := obj.(*v1.AccessControlRule)
	var result []string
	for _, subject := range rule.Spec.Manifest.Subjects {
		if subjectType != "" && subject.Type == subjectType {
			result = append(result, subject.ID)
		}
	}
	for _, resource := range rule.Spec.Manifest.Resources {
		if resourceType != "" && resource.Type == resourceType {
			result = append(result, resource.ID)
		}
	}
	return result
}

func validateCatalogEntryBenchmarkResponse(
	b *testing.B,
	handler func(api.Context) error,
	newContext func() (api.Context, *httptest.ResponseRecorder),
	wantEntries int,
) {
	b.Helper()
	req, recorder := newContext()
	if err := handler(req); err != nil {
		b.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		b.Fatalf("unexpected status: got %d, want %d", recorder.Code, http.StatusOK)
	}
	var response types.MCPServerCatalogEntryList
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		b.Fatal(err)
	}
	if len(response.Items) != wantEntries {
		b.Fatalf("unexpected entry count: got %d, want %d", len(response.Items), wantEntries)
	}
}
