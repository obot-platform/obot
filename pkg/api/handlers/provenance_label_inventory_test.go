package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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

type countedLabelInventoryStore struct {
	provenance.Store
	reads  int
	writes int
}

func (s *countedLabelInventoryStore) ListToolLabels(ctx context.Context) ([]provenance.ToolLabel, error) {
	s.reads++
	return s.Store.ListToolLabels(ctx)
}

func (s *countedLabelInventoryStore) PutToolLabel(ctx context.Context, label *provenance.ToolLabel) error {
	s.writes++
	return s.Store.PutToolLabel(ctx, label)
}

func labelInventoryTestCall(t *testing.T, h *ProvenanceHandler, storage storage.Client) (*labelInventory, error) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/provenance/label-inventory", nil)
	if err := h.LabelInventory(api.Context{Request: req, ResponseWriter: rec, Storage: storage}); err != nil {
		return nil, err
	}
	var out labelInventory
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return &out, nil
}

func TestLabelInventoryShowsEffectiveSourcesReviewStateAndContentSettings(t *testing.T) {
	h, engine := newProvAPITest(t)
	entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "entry", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{
		SourceURL: "https://github.com/example/catalog", Manifest: types.MCPServerCatalogEntryManifest{Name: "Meridian Research", ToolPreview: []types.MCPServerTool{
			{Name: "read_record", Description: "Read a private record"}, {Name: "run_task", Description: "Execute a task"}, {Name: "read_public", Description: "Read public documents"},
		}},
	}}
	unreviewed := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "unreviewed", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "Unreviewed", ToolPreview: []types.MCPServerTool{{Name: "search_public_docs", Description: "Search public documentation"}}}}}
	empty := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "No previews"}}}
	vmcp := &v1.VMCP{ObjectMeta: metav1.ObjectMeta{Name: "vmcp1", Namespace: system.DefaultNamespace}, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Care profile", Components: []types.VMCPComponent{{ID: "records", Name: "records", MCPServerCatalogEntryID: entry.Name, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}}}}}}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, unreviewed, empty, vmcp).Build()
	req := api.Context{Request: httptest.NewRequest(http.MethodGet, "/", nil), Storage: storage}
	resource := toolLabelResource{ResourceType: "catalog-entry", ResourceID: entry.Name}
	current, err := h.toolLabelInventory(req, resource)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, label := range []provenance.ToolLabel{
		{Server: "catalog-entry:entry", Tool: "read_record", Source: "admin", Labels: provenance.NewLabels(provenance.CapReadsPrivate), Rationale: "Reviewed private records access", UpdatedAt: now.Add(-time.Hour)},
		{Server: "catalog-entry:entry", Tool: "run_task", Source: "heuristic", Labels: provenance.NewLabels(provenance.CapExec), UpdatedAt: now.Add(-time.Minute), Proposal: &provenance.ToolLabelProposal{Labels: provenance.NewLabels(provenance.CapDestructive), SchemaHash: current.Items[1].schemaHash, GeneratedAt: now}},
		{Server: "catalog-entry:entry", Tool: "read_public", Source: "heuristic", Labels: provenance.NewLabels(provenance.CapReadsPrivate), UpdatedAt: now.Add(-time.Minute), Proposal: &provenance.ToolLabelProposal{Labels: provenance.NewLabels(provenance.CapReadsUntrusted), SchemaHash: "obsolete-schema", GeneratedAt: now}},
		{Server: "Meridian Research", Tool: "read_public", Source: "admin", Labels: provenance.NewLabels(provenance.CapReadsUntrusted), Rationale: "Manual legacy override", UpdatedAt: now.Add(-time.Minute)},
		{Server: "codex", Tool: "shell", Source: "heuristic", Labels: provenance.NewLabels(provenance.CapExec), UpdatedAt: now},
	} {
		if err := engine.Store().PutToolLabel(t.Context(), &label); err != nil {
			t.Fatal(err)
		}
	}
	settings := provenance.DefaultSettings()
	settings.ContentTagging.Enabled = true
	settings.ContentTagging.Mode = provenance.ContentTagSync
	if err := engine.Store().PutSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	counter := &countedLabelInventoryStore{Store: engine.Store()}
	readEngine := provenance.New(counter)
	supervisor := &toolLabelTestSupervisor{}
	readEngine.SetSupervisor(supervisor)
	out, err := labelInventoryTestCall(t, NewProvenanceHandler(readEngine), storage)
	if err != nil {
		t.Fatal(err)
	}
	if counter.reads != 1 || counter.writes != 0 || len(supervisor.seen) != 0 {
		t.Fatalf("overview side effects: reads=%d writes=%d modelCalls=%d", counter.reads, counter.writes, len(supervisor.seen))
	}
	if len(out.Resources) != 3 {
		t.Fatalf("resources=%+v", out.Resources)
	}
	var research *labelResourceSummary
	for i := range out.Resources {
		if out.Resources[i].ResourceType == "catalog-entry" && out.Resources[i].ResourceID == entry.Name {
			research = &out.Resources[i]
		}
	}
	if research == nil || research.ToolCount != 3 || research.ReviewedToolCount != 2 || research.PendingToolCount != 1 || research.StaleToolCount != 1 {
		t.Fatalf("research summary=%+v", research)
	}
	if research.SourceURL != entry.Spec.SourceURL || !research.Labels.Has(provenance.CapReadsPrivate) || !research.Labels.Has(provenance.CapReadsUntrusted) || !research.Labels.Has(provenance.CapExec) || research.UpdatedAt == nil || !research.UpdatedAt.Equal(now) {
		t.Fatalf("effective management metadata=%+v", research)
	}
	if len(out.ObservedTools) != 2 || out.ObservedTools[0].Source != "admin" || out.ObservedTools[0].Rationale != "Manual legacy override" {
		t.Fatalf("observed tools=%+v", out.ObservedTools)
	}
	if !reflect.DeepEqual(out.Capabilities, provenance.CapabilityLabels) || !reflect.DeepEqual(out.ContentLabels, provenance.ContentLabels) || !reflect.DeepEqual(out.ContentTagging, settings.ContentTagging) || !out.SupervisorEnabled {
		t.Fatalf("label vocabulary/settings=%+v", out)
	}
}

type failedLabelInventoryStorage struct {
	storage.Client
	err error
}

func (s failedLabelInventoryStorage) List(context.Context, kclient.ObjectList, ...kclient.ListOption) error {
	return s.err
}

func TestLabelInventorySkipsDeletingResourcesAndPropagatesErrors(t *testing.T) {
	h, engine := newProvAPITest(t)
	deleted := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "deleted", Namespace: system.DefaultNamespace, Finalizers: []string{"cleanup"}, DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()}}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "Deleting", ToolPreview: []types.MCPServerTool{{Name: "read"}}}}}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(deleted).Build()
	if err := engine.Store().PutToolLabel(t.Context(), &provenance.ToolLabel{Server: "catalog-entry:deleted", Tool: "read", Source: "admin", Labels: provenance.NewLabels(provenance.CapReadsPrivate)}); err != nil {
		t.Fatal(err)
	}
	out, err := labelInventoryTestCall(t, h, storage)
	if err != nil || len(out.Resources) != 0 || out.Resources == nil || out.ObservedTools == nil {
		t.Fatalf("empty inventory=%+v error=%v", out, err)
	}
	failure := errors.New("database unavailable")
	if _, err := labelInventoryTestCall(t, h, failedLabelInventoryStorage{err: failure}); !errors.Is(err, failure) {
		t.Fatalf("resource list error=%v", err)
	}
	h = NewProvenanceHandler(provenance.New(failedToolLabelStore{Store: provenance.NewMemStore(), err: failure}))
	if _, err := labelInventoryTestCall(t, h, nil); !errors.Is(err, failure) {
		t.Fatalf("label store error=%v", err)
	}
}

func TestLabelInventoryKeepsInvalidResourceVisibleWithoutHidingHealthyResources(t *testing.T) {
	h, engine := newProvAPITest(t)
	entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "Healthy records", ToolPreview: []types.MCPServerTool{{Name: "lookup", Description: "Read a record"}}}}}
	badSource := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "template", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Name: "Legacy template", ToolPreview: []types.MCPServerTool{{Name: "fetch_<repo>_documentation"}}}}}
	vmcp := &v1.VMCP{ObjectMeta: metav1.ObjectMeta{Name: "invalid-vmcp", Namespace: system.DefaultNamespace}, Spec: v1.VMCPSpec{SourceURL: "https://github.com/example/catalog", Manifest: types.VMCPManifest{DisplayName: "Legacy documentation", Components: []types.VMCPComponent{{ID: "docs", Name: "docs", MCPServerCatalogEntryID: badSource.Name, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: badSource.Spec.Manifest}}}}}}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, badSource, vmcp).Build()
	label := provenance.ToolLabel{Server: "vmcp:invalid-vmcp", Tool: "fetch_<repo>_documentation", Source: "heuristic", Proposal: &provenance.ToolLabelProposal{SchemaHash: "old-template-schema"}}
	if err := engine.Store().PutToolLabel(t.Context(), &label); err != nil {
		t.Fatal(err)
	}
	out, err := labelInventoryTestCall(t, h, storage)
	if err != nil {
		t.Fatal(err)
	}
	var healthy, invalid bool
	for _, row := range out.Resources {
		if row.ResourceType == "catalog-entry" && row.ResourceID == entry.Name {
			healthy = row.ToolCount == 1 && row.InventoryError == ""
		}
		if row.ResourceType == "vmcp" && row.ResourceID == vmcp.Name {
			invalid = row.ToolCount == 0 && row.PendingToolCount == 0 && row.InventoryError != "" && row.DisplayName == vmcp.Spec.Manifest.DisplayName && row.SourceURL == vmcp.Spec.SourceURL
		}
	}
	if !healthy || !invalid {
		t.Fatalf("expected healthy and actionable invalid rows: %+v", out.Resources)
	}
	current, err := h.toolLabelInventory(api.Context{Request: httptest.NewRequest(http.MethodGet, "/", nil), Storage: storage}, toolLabelResource{ResourceType: "catalog-entry", ResourceID: entry.Name})
	if err != nil {
		t.Fatal(err)
	}
	valid := provenance.ToolLabel{Server: "catalog-entry:" + entry.Name, Tool: "lookup", Source: "heuristic", Proposal: &provenance.ToolLabelProposal{SchemaHash: current.Items[0].schemaHash}}
	if err := engine.Store().PutToolLabel(t.Context(), &valid); err != nil {
		t.Fatal(err)
	}
	reviews, err := listToolLabelReviewsTest(t, h, storage)
	if err != nil || reviews.Total != 1 || reviews.Items[0].ResourceID != entry.Name {
		t.Fatalf("invalid resource must not hide valid reviews: %+v (%v)", reviews, err)
	}
	// Classification and approval remain strict. The aggregate's tolerance
	// must never turn an invalid template into an approvable tool.
	resource := toolLabelResource{ResourceType: "vmcp", ResourceID: vmcp.Name}
	if _, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/", resource); err == nil {
		t.Fatal("invalid inventory accepted for classification")
	}
	if _, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/", toolLabelResource{ResourceType: "catalog-entry", ResourceID: badSource.Name}); err == nil {
		t.Fatal("invalid catalog template accepted for classification")
	}
	if _, err := toolLabelsTestCall(t, storage, h.ApproveToolLabels, http.MethodPost, "/", map[string]any{"resourceType": "vmcp", "resourceID": vmcp.Name, "tools": []map[string]string{{"tool": label.Tool, "schemaHash": label.Proposal.SchemaHash}}}); err == nil {
		t.Fatal("invalid inventory accepted for approval")
	}
}
