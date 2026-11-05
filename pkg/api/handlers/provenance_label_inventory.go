package handlers

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type labelResourceSummary struct {
	toolLabelResource
	DisplayName       string            `json:"displayName"`
	SourceURL         string            `json:"sourceURL,omitempty"`
	ToolCount         int               `json:"toolCount"`
	ReviewedToolCount int               `json:"reviewedToolCount"`
	PendingToolCount  int               `json:"pendingToolCount"`
	StaleToolCount    int               `json:"staleToolCount"`
	Labels            provenance.Labels `json:"labels"`
	UpdatedAt         *time.Time        `json:"updatedAt,omitempty"`
	InventoryError    string            `json:"inventoryError,omitempty"`
}

type labelInventory struct {
	Resources         []labelResourceSummary    `json:"resources"`
	ObservedTools     []provenance.ToolLabel    `json:"observedTools"`
	Capabilities      provenance.Labels         `json:"capabilities"`
	ContentLabels     provenance.Labels         `json:"contentLabels"`
	ContentTagging    provenance.ContentTagging `json:"contentTagging"`
	SupervisorEnabled bool                      `json:"supervisorEnabled"`
}

// LabelInventory describes current resource labels and legacy observed labels
// without classifying anything or changing effective policy. It reads the
// label store once and reuses the same authoritative inventory as review.
func (h *ProvenanceHandler) LabelInventory(req api.Context) error {
	labels, err := h.engine.Store().ListToolLabels(req.Context())
	if err != nil {
		return err
	}
	settings, err := h.loadSettings(req)
	if err != nil {
		return err
	}
	out := labelInventory{
		Resources: []labelResourceSummary{}, ObservedTools: []provenance.ToolLabel{},
		Capabilities: slices.Clone(provenance.CapabilityLabels), ContentLabels: slices.Clone(provenance.ContentLabels),
		ContentTagging: settings.ContentTagging, SupervisorEnabled: settings.SupervisorEnabled,
	}
	scoped := map[toolLabelResource]bool{}
	for _, label := range labels {
		kind, id, ok := strings.Cut(label.Server, ":")
		if ok && id != "" && (kind == "catalog-entry" || kind == "vmcp") {
			scoped[toolLabelResource{ResourceType: kind, ResourceID: id}] = true
		} else {
			out.ObservedTools = append(out.ObservedTools, label)
		}
	}
	var entries v1.MCPServerCatalogEntryList
	if err := req.List(&entries); err != nil {
		return err
	}
	var vmcps v1.VMCPList
	if err := req.List(&vmcps); err != nil {
		return err
	}
	resources := make([]toolLabelResource, 0, len(entries.Items)+len(vmcps.Items))
	metadata := map[toolLabelResource]labelResourceSummary{}
	entriesWithPreviews := map[string]bool{}
	for _, entry := range entries.Items {
		if entry.DeletionTimestamp.IsZero() {
			resource := toolLabelResource{ResourceType: "catalog-entry", ResourceID: entry.Name}
			metadata[resource] = labelResourceSummary{toolLabelResource: resource, DisplayName: entry.Spec.Manifest.Name, SourceURL: entry.Spec.SourceURL, Labels: provenance.Labels{}}
			entriesWithPreviews[entry.Name] = len(entry.Spec.Manifest.ToolPreview) > 0
			if entriesWithPreviews[entry.Name] || scoped[resource] {
				resources = append(resources, resource)
			}
		}
	}
	for _, vmcp := range vmcps.Items {
		if vmcp.DeletionTimestamp.IsZero() {
			resource := toolLabelResource{ResourceType: "vmcp", ResourceID: vmcp.Name}
			metadata[resource] = labelResourceSummary{toolLabelResource: resource, DisplayName: vmcp.Spec.Manifest.DisplayName, SourceURL: vmcp.Spec.SourceURL, Labels: provenance.Labels{}}
			hasPreviews := slices.ContainsFunc(vmcp.Spec.Manifest.Components, func(component types.VMCPComponent) bool {
				return len(component.CatalogEntry.Manifest.ToolPreview) > 0 || entriesWithPreviews[component.MCPServerCatalogEntryID]
			})
			if hasPreviews || scoped[resource] {
				resources = append(resources, resource)
			}
		}
	}
	for _, resource := range resources {
		inventory, err := h.toolLabelInventoryFromLabels(req, resource, labels)
		if err != nil {
			var httpErr *types.ErrHTTP
			if apierrors.IsNotFound(err) || (errors.As(err, &httpErr) && httpErr.Code == 404) {
				// A resource may be removed between the list and inventory reads.
				continue
			}
			if errors.As(err, &httpErr) && httpErr.Code == 400 {
				// One invalid legacy preview must not hide every other resource.
				// Keep its identity visible so an administrator can repair it.
				summary := metadata[resource]
				summary.InventoryError = httpErr.Message
				out.Resources = append(out.Resources, summary)
				continue
			}
			return err
		}
		if len(inventory.Items) == 0 && !scoped[resource] {
			continue
		}
		summary := labelResourceSummary{
			toolLabelResource: resource, DisplayName: inventory.DisplayName, SourceURL: inventory.SourceURL,
			ToolCount: len(inventory.Items), Labels: provenance.Labels{},
		}
		var updatedAt time.Time
		for _, item := range inventory.Items {
			summary.Labels = summary.Labels.Add(item.Labels...)
			if item.Source == "admin" {
				summary.ReviewedToolCount++
			}
			if item.UpdatedAt.After(updatedAt) {
				updatedAt = item.UpdatedAt
			}
			if item.Proposal != nil {
				if item.Stale {
					summary.StaleToolCount++
				} else if item.Proposal.SchemaHash != "" {
					summary.PendingToolCount++
				}
				if item.Proposal.GeneratedAt.After(updatedAt) {
					updatedAt = item.Proposal.GeneratedAt
				}
			}
		}
		if !updatedAt.IsZero() {
			summary.UpdatedAt = &updatedAt
		}
		out.Resources = append(out.Resources, summary)
	}
	slices.SortFunc(out.Resources, func(a, b labelResourceSummary) int {
		if cmp := strings.Compare(a.DisplayName, b.DisplayName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ResourceType+":"+a.ResourceID, b.ResourceType+":"+b.ResourceID)
	})
	slices.SortFunc(out.ObservedTools, func(a, b provenance.ToolLabel) int {
		if cmp := strings.Compare(a.Server, b.Server); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Tool, b.Tool)
	})
	return req.Write(out)
}
