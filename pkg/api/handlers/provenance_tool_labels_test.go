package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

type toolLabelTestSupervisor struct {
	fakeProvAPISupervisor
	seen []provenance.ToolDescriptor
}

func (f *toolLabelTestSupervisor) ClassifyTool(_ context.Context, tool provenance.ToolDescriptor) (provenance.Labels, string, error) {
	f.seen = append(f.seen, tool)
	return provenance.NewLabels(provenance.CapReadsPrivate), "Reads private patient records.", nil
}

func toolLabelsTestCall(t *testing.T, storage storage.Client, handler func(api.Context) error, method, path string, body any) (*toolLabelInventory, error) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	rec := httptest.NewRecorder()
	err = handler(api.Context{Request: req, ResponseWriter: rec, Storage: storage})
	if err != nil {
		return nil, err
	}
	var out toolLabelInventory
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response %q: %v", rec.Body.String(), err)
	}
	return &out, nil
}

func TestProvenanceToolLabelReviewForGitCatalogAndVMCP(t *testing.T) {
	for _, resourceType := range []string{"catalog-entry", "vmcp"} {
		t.Run(resourceType, func(t *testing.T) {
			h, engine := newProvAPITest(t)
			supervisor := &toolLabelTestSupervisor{}
			engine.SetSupervisor(supervisor)
			entry := &v1.MCPServerCatalogEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: system.DefaultNamespace},
				Spec: v1.MCPServerCatalogEntrySpec{
					SourceURL: "https://github.com/example/catalog",
					Manifest: types.MCPServerCatalogEntryManifest{
						Name: "EHR", Runtime: types.RuntimeRemote,
						ToolPreview: []types.MCPServerTool{{Name: "lookup", Description: "Read synthetic patient record", Params: map[string]string{"patient_id": "Patient record identifier"}}},
					},
				},
			}
			vmcp := &v1.VMCP{
				ObjectMeta: metav1.ObjectMeta{Name: "vmcp1", Namespace: system.DefaultNamespace},
				Spec: v1.VMCPSpec{SourceURL: entry.Spec.SourceURL, Manifest: types.VMCPManifest{
					DisplayName: "Care team",
					Components: []types.VMCPComponent{{ID: "ehr", Name: "Patient records", MCPServerCatalogEntryID: entry.Name,
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest},
						ToolPrefix:   "care", ToolOverrides: []types.ToolOverride{{Name: "lookup", OverrideName: "find_patient", Enabled: true}},
					}},
				}},
			}
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, vmcp).Build()
			resourceID, tool := entry.Name, "lookup"
			if resourceType == "vmcp" {
				resourceID, tool = vmcp.Name, "care__find_patient"
			}
			resource := toolLabelResource{ResourceType: resourceType, ResourceID: resourceID}
			scope := provenance.ToolLabelScope(resourceType, resourceID)
			manual := provenance.ToolLabel{Server: scope, Tool: tool, Labels: provenance.NewLabels(provenance.CapExec), Source: "admin"}
			if err := engine.Store().PutToolLabel(t.Context(), &manual); err != nil {
				t.Fatal(err)
			}
			out, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/api/provenance/tool-label-suggestions", resource)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Items) != 1 || out.Items[0].Tool != tool || out.Items[0].Proposal == nil || out.Items[0].Stale || out.SourceURL != entry.Spec.SourceURL {
				t.Fatalf("suggestions: %+v", out)
			}
			if len(supervisor.seen) != 1 || supervisor.seen[0].Tool != tool || supervisor.seen[0].InputSchema == nil {
				t.Fatalf("model inventory: %+v", supervisor.seen)
			}
			if labels, ok := engine.ReviewedToolCapabilities(t.Context(), scope, tool); !ok || !labels.Has(provenance.CapExec) {
				t.Fatalf("suggestion overwrote explicit label: %v %v", labels, ok)
			}
			approve := map[string]any{"resourceType": resourceType, "resourceID": resourceID,
				"tools": []map[string]string{{"tool": tool, "schemaHash": out.Items[0].Proposal.SchemaHash}}}
			out, err = toolLabelsTestCall(t, storage, h.ApproveToolLabels, http.MethodPost, "/api/provenance/tool-label-suggestions/approve", approve)
			if err != nil {
				t.Fatal(err)
			}
			if out.Items[0].Source != "admin" || out.Items[0].Proposal != nil || !out.Items[0].Labels.Has(provenance.CapReadsPrivate) {
				t.Fatalf("approved labels: %+v", out.Items[0])
			}
			if labels, ok := engine.ReviewedToolCapabilities(t.Context(), scope, tool); !ok || !labels.Has(provenance.CapReadsPrivate) || labels.Has(provenance.CapExec) {
				t.Fatalf("effective labels: %v %v", labels, ok)
			}
			// Source definitions, including Git-managed ones, are never written by
			// suggestion generation or approval.
			var saved v1.MCPServerCatalogEntry
			if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.ResourceVersion != entry.ResourceVersion {
				t.Fatal("label review changed the Git source object")
			}
		})
	}
}

func TestProvenanceToolLabelRejectsChangedInventory(t *testing.T) {
	h, engine := newProvAPITest(t)
	engine.SetSupervisor(&toolLabelTestSupervisor{})
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{
			Name: "Records", ToolPreview: []types.MCPServerTool{{Name: "lookup", Description: "Original schema"}},
		}},
	}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry).Build()
	resource := toolLabelResource{ResourceType: "catalog-entry", ResourceID: entry.Name}
	out, err := toolLabelsTestCall(t, storage, h.SuggestToolLabels, http.MethodPost, "/", resource)
	if err != nil {
		t.Fatal(err)
	}
	entry.Spec.Manifest.ToolPreview[0].Description = "Changed meaning"
	if err := storage.Update(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	_, err = toolLabelsTestCall(t, storage, h.ApproveToolLabels, http.MethodPost, "/", map[string]any{
		"resourceType": resource.ResourceType, "resourceID": resource.ResourceID,
		"tools": []map[string]string{{"tool": "lookup", "schemaHash": out.Items[0].Proposal.SchemaHash}},
	})
	if err == nil {
		t.Fatal("approved a stale suggestion")
	}
	if labels, ok := engine.ReviewedToolCapabilities(t.Context(), provenance.ToolLabelScope(resource.ResourceType, resource.ResourceID), "lookup"); ok {
		t.Fatalf("stale labels became effective: %v", labels)
	}
}

func TestVMCPToolLabelInventoryUsesActualTransparentOrPrefixedNames(t *testing.T) {
	for _, tt := range []struct {
		name   string
		count  int
		prefix string
		want   string
	}{
		{
			name:  "single component stays transparent",
			count: 1,
			want:  "lookup",
		},
		{
			name:   "explicit prefix on single component",
			count:  1,
			prefix: "care",
			want:   "care__lookup",
		},
		{
			name:  "multiple components use implicit prefixes",
			count: 2,
			want:  "patient_records__lookup",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newProvAPITest(t)
			manifest := types.MCPServerCatalogEntryManifest{Name: "EHR", ToolPreview: []types.MCPServerTool{{Name: "lookup", Description: "Read a record"}}}
			entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerCatalogEntrySpec{Manifest: manifest}}
			components := []types.VMCPComponent{{ID: "ehr", Name: "Patient records", ToolPrefix: tt.prefix, MCPServerCatalogEntryID: entry.Name, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: manifest}}}
			if tt.count == 2 {
				components = append(components, types.VMCPComponent{ID: "claims", Name: "Claims", MCPServerCatalogEntryID: entry.Name, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: manifest}})
			}
			vmcp := &v1.VMCP{ObjectMeta: metav1.ObjectMeta{Name: "vmcp1", Namespace: system.DefaultNamespace}, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{DisplayName: "Care team", Components: components}}}
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, vmcp).Build()
			out, err := toolLabelsTestCall(t, storage, h.ListToolLabelSuggestions, http.MethodGet, "/?resourceType=vmcp&resourceID=vmcp1", nil)
			if err != nil || len(out.Items) != tt.count || out.Items[0].Tool != tt.want {
				t.Fatalf("inventory=%+v error=%v", out, err)
			}
		})
	}
}
