package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func listToolLabelReviewsTest(t *testing.T, h *ProvenanceHandler, storage storage.Client) (*toolLabelReviews, error) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/provenance/tool-label-reviews", nil)
	if err := h.ListToolLabelReviews(api.Context{Request: req, ResponseWriter: rec, Storage: storage}); err != nil {
		return nil, err
	}
	var result toolLabelReviews
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return &result, nil
}

func TestToolLabelReviewsGroupAndTrackExplicitApproval(t *testing.T) {
	for _, resourceType := range []string{"catalog-entry", "vmcp"} {
		t.Run(resourceType, func(t *testing.T) {
			h, engine := newProvAPITest(t)
			supervisor := &toolLabelTestSupervisor{}
			engine.SetSupervisor(supervisor)
			entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "meridian", Namespace: system.DefaultNamespace},
				Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "Meridian Research"}}}
			for i := range 7 {
				entry.Spec.Manifest.ToolPreview = append(entry.Spec.Manifest.ToolPreview, types.MCPServerTool{Name: fmt.Sprintf("tool_%d", i), Description: "Research tool"})
			}
			vmcp := &v1.VMCP{ObjectMeta: metav1.ObjectMeta{Name: "vmcp1", Namespace: system.DefaultNamespace}, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Meridian Research", Components: []types.VMCPComponent{{ID: "research", Name: "research", MCPServerCatalogEntryID: entry.Name, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}}}}}}
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, vmcp).Build()
			resource := toolLabelResource{ResourceType: resourceType, ResourceID: entry.Name}
			if resourceType == "vmcp" {
				resource.ResourceID = vmcp.Name
			}
			inventory, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/", resource)
			if err != nil {
				t.Fatal(err)
			}
			modelCalls := len(supervisor.seen)
			latest := time.Now().UTC().Add(time.Minute)
			labels, err := engine.Store().ListToolLabels(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			labels[0].Proposal.GeneratedAt = latest
			if err := engine.Store().PutToolLabel(t.Context(), &labels[0]); err != nil {
				t.Fatal(err)
			}
			reviews, err := listToolLabelReviewsTest(t, h, storage)
			if err != nil || reviews.Total != 1 || len(reviews.Items) != 1 {
				t.Fatalf("reviews=%+v err=%v", reviews, err)
			}
			review := reviews.Items[0]
			if review.ResourceType != resource.ResourceType || review.ResourceID != resource.ResourceID || review.DisplayName != "Meridian Research" || review.ToolCount != 7 || !review.GeneratedAt.Equal(latest) {
				t.Fatalf("grouped review=%+v", review)
			}
			approve := func(items []toolLabelSuggestion) {
				t.Helper()
				tools := make([]map[string]string, 0, len(items))
				for _, item := range items {
					tools = append(tools, map[string]string{"tool": item.Tool, "schemaHash": item.Proposal.SchemaHash})
				}
				if _, err := toolLabelsTestCall(t, storage, h.ApproveToolLabels, http.MethodPost, "/", map[string]any{"resourceType": resource.ResourceType, "resourceID": resource.ResourceID, "tools": tools}); err != nil {
					t.Fatal(err)
				}
			}
			approve(inventory.Items[:2])
			reviews, err = listToolLabelReviewsTest(t, h, storage)
			if err != nil || reviews.Total != 1 || reviews.Items[0].ToolCount != 5 {
				t.Fatalf("partial approval=%+v err=%v", reviews, err)
			}
			approve(inventory.Items[2:])
			reviews, err = listToolLabelReviewsTest(t, h, storage)
			if err != nil || reviews.Total != 0 || len(reviews.Items) != 0 || reviews.Items == nil {
				t.Fatalf("full approval=%+v err=%v", reviews, err)
			}
			if len(supervisor.seen) != modelCalls {
				t.Fatal("listing reviews called the model")
			}
		})
	}
}

func TestToolLabelReviewsExcludeObsoleteOrUnscopedProposals(t *testing.T) {
	for _, state := range []string{"fresh", "stale", "removed-tool", "deleted-resource", "deleting-resource", "legacy", "unknown-resource-type", "unscoped"} {
		t.Run(state, func(t *testing.T) {
			h, engine := newProvAPITest(t)
			engine.SetSupervisor(&toolLabelTestSupervisor{})
			entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "entry", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{
				Name: "Research", ToolPreview: []types.MCPServerTool{{Name: "lookup", Description: "Original tool"}},
			}}}
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry).Build()
			resource := toolLabelResource{ResourceType: "catalog-entry", ResourceID: entry.Name}
			if _, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/", resource); err != nil {
				t.Fatal(err)
			}
			labels, err := engine.Store().ListToolLabels(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			label := labels[0]
			switch state {
			case "stale":
				entry.Spec.Manifest.ToolPreview[0].Params = map[string]string{"new_parameter": "Changed schema"}
				if err := storage.Update(t.Context(), entry); err != nil {
					t.Fatal(err)
				}
			case "removed-tool":
				entry.Spec.Manifest.ToolPreview = nil
				if err := storage.Update(t.Context(), entry); err != nil {
					t.Fatal(err)
				}
			case "deleted-resource":
				if err := storage.Delete(t.Context(), entry); err != nil {
					t.Fatal(err)
				}
			case "deleting-resource":
				entry.Finalizers = []string{"test-cleanup"}
				if err := storage.Update(t.Context(), entry); err != nil {
					t.Fatal(err)
				}
				if err := storage.Delete(t.Context(), entry); err != nil {
					t.Fatal(err)
				}
			case "legacy", "unknown-resource-type", "unscoped":
				// Replace the store so the invalid proposal is the only candidate.
				s := provenance.NewMemStore()
				if state == "legacy" {
					label.Proposal.SchemaHash = ""
				}
				if state == "unknown-resource-type" {
					label.Server = "unknown:entry"
				}
				if state == "unscoped" {
					label.Server = "Research"
				}
				if err := s.PutToolLabel(t.Context(), &label); err != nil {
					t.Fatal(err)
				}
				h = NewProvenanceHandler(provenance.New(s))
			}
			reviews, err := listToolLabelReviewsTest(t, h, storage)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if state == "fresh" {
				want = 1
			}
			if reviews.Total != want || len(reviews.Items) != want {
				t.Fatalf("reviews=%+v want %d", reviews, want)
			}
		})
	}
}

type failedToolLabelStore struct {
	provenance.Store
	err error
}

func (s failedToolLabelStore) ListToolLabels(context.Context) ([]provenance.ToolLabel, error) {
	return nil, s.err
}

type failedResourceStorage struct {
	storage.Client
	err        error
	failOnName string
}

func (s failedResourceStorage) Get(ctx context.Context, key kclient.ObjectKey, object kclient.Object, options ...kclient.GetOption) error {
	if s.failOnName == "" || s.failOnName == key.Name {
		return s.err
	}
	return s.Client.Get(ctx, key, object, options...)
}

func TestToolLabelReviewsPropagateStoreAndResourceErrors(t *testing.T) {
	failure := errors.New("database unavailable")
	h := NewProvenanceHandler(provenance.New(failedToolLabelStore{Store: provenance.NewMemStore(), err: failure}))
	if _, err := listToolLabelReviewsTest(t, h, nil); !errors.Is(err, failure) {
		t.Fatalf("store error=%v", err)
	}
	store := provenance.NewMemStore()
	label := provenance.ToolLabel{Server: "catalog-entry:entry", Tool: "lookup", Proposal: &provenance.ToolLabelProposal{SchemaHash: "schema"}}
	if err := store.PutToolLabel(t.Context(), &label); err != nil {
		t.Fatal(err)
	}
	h = NewProvenanceHandler(provenance.New(store))
	if _, err := listToolLabelReviewsTest(t, h, failedResourceStorage{err: failure}); !errors.Is(err, failure) {
		t.Fatalf("resource error=%v", err)
	}
	// A real source-preview error is different from an entry removed from a
	// pinned vMCP. Do not silently hide every review when that lookup fails.
	vmcp := &v1.VMCP{ObjectMeta: metav1.ObjectMeta{Name: "vmcp1", Namespace: system.DefaultNamespace}, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Care team", Components: []types.VMCPComponent{{ID: "records", Name: "records", MCPServerCatalogEntryID: "entry"}},
	}}}
	resources := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(vmcp).Build()
	store = provenance.NewMemStore()
	label.Server = "vmcp:vmcp1"
	if err := store.PutToolLabel(t.Context(), &label); err != nil {
		t.Fatal(err)
	}
	h = NewProvenanceHandler(provenance.New(store))
	if _, err := listToolLabelReviewsTest(t, h, failedResourceStorage{Client: resources, err: failure, failOnName: "entry"}); !errors.Is(err, failure) {
		t.Fatalf("vMCP preview source error=%v", err)
	}
}
