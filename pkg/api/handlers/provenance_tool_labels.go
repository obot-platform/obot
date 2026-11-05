package handlers

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/mmmcp/namespace"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type toolLabelResource struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceID"`
}

type toolLabelInventory struct {
	toolLabelResource
	DisplayName string                `json:"displayName"`
	SourceURL   string                `json:"sourceURL,omitempty"`
	Items       []toolLabelSuggestion `json:"items"`
}

type toolLabelSuggestion struct {
	provenance.ToolLabel
	Description string `json:"description"`
	Stale       bool   `json:"stale"`
	// schemaHash and schema are reconstructed from stored inventory, never from
	// caller-supplied schemas. Only the proposal hash is exposed to the client.
	schemaHash  string
	schema      map[string]any
	storedLabel provenance.ToolLabel
}

type approveToolLabelsRequest struct {
	toolLabelResource
	Tools []struct {
		Tool       string `json:"tool"`
		SchemaHash string `json:"schemaHash"`
	} `json:"tools"`
}

// ListToolLabelSuggestions shows approved labels and pending recommendations
// for a saved resource. Git-managed definitions remain untouched.
func (h *ProvenanceHandler) ListToolLabelSuggestions(req api.Context) error {
	resource := toolLabelResource{ResourceType: req.URL.Query().Get("resourceType"), ResourceID: req.URL.Query().Get("resourceID")}
	inventory, err := h.toolLabelInventory(req, resource)
	if err != nil {
		return err
	}
	return req.Write(inventory)
}

// SuggestToolLabels asks the Supervisor about the actual stored inventory.
// Existing effective labels stay unchanged until ApproveToolLabels is called.
func (h *ProvenanceHandler) SuggestToolLabels(req api.Context) error {
	var resource toolLabelResource
	if err := req.Read(&resource); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	inventory, err := h.toolLabelInventory(req, resource)
	if err != nil {
		return err
	}
	if h.engine.Supervisor() == nil || !h.engine.Settings(req.Context()).SupervisorEnabled {
		return req.WriteCode(map[string]string{"error": "Configure and enable the Supervisor before requesting AI labels."}, 503)
	}
	if len(inventory.Items) == 0 {
		return req.Write(inventory)
	}
	if len(inventory.Items) > 100 {
		return types.NewErrBadRequest("this resource has more than 100 tools; review smaller component inventories first")
	}
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Minute)
	defer cancel()
	for i := range inventory.Items {
		item := &inventory.Items[i]
		labels, rationale, err := h.engine.Supervisor().ClassifyTool(ctx, provenance.ToolDescriptor{
			Server: inventory.DisplayName, Tool: item.Tool, Description: item.Description,
			InputSchema: item.schema, Heuristic: item.Labels,
		})
		if err != nil {
			return fmt.Errorf("suggest labels for %s: %w", item.Tool, err)
		}
		for _, label := range labels {
			if !provenance.CapabilityLabels.Has(label) {
				return fmt.Errorf("Supervisor returned an unknown capability label %q", label)
			}
		}
		item.Proposal = &provenance.ToolLabelProposal{Labels: labels.Normalize(), Rationale: rationale, SchemaHash: item.schemaHash, GeneratedAt: time.Now().UTC()}
	}
	// Re-read after model calls. A saved resource edit invalidates the entire
	// result, and an administrator's concurrent label edit is retained.
	current, err := h.toolLabelInventory(req, resource)
	if err != nil {
		return err
	}
	byTool := make(map[string]toolLabelSuggestion, len(current.Items))
	for _, item := range current.Items {
		byTool[item.Tool] = item
	}
	for _, item := range inventory.Items {
		if byTool[item.Tool].schemaHash != item.schemaHash {
			return types.NewErrHTTP(409, "The tool inventory changed while the Supervisor was reviewing it. Request fresh suggestions.")
		}
	}
	for _, item := range inventory.Items {
		label := byTool[item.Tool].storedLabel
		label.Proposal = item.Proposal
		if err := h.engine.Store().PutToolLabel(req.Context(), &label); err != nil {
			return err
		}
	}
	h.engine.Invalidate()
	return h.writeToolLabelInventory(req, resource)
}

// ApproveToolLabels accepts only explicitly selected, current proposals. It
// writes reviewed sidecar labels, never the source catalog or Git repository.
func (h *ProvenanceHandler) ApproveToolLabels(req api.Context) error {
	var in approveToolLabelsRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if len(in.Tools) == 0 {
		return types.NewErrBadRequest("select at least one tool proposal")
	}
	inventory, err := h.toolLabelInventory(req, in.toolLabelResource)
	if err != nil {
		return err
	}
	byTool := make(map[string]toolLabelSuggestion, len(inventory.Items))
	for _, item := range inventory.Items {
		byTool[item.Tool] = item
	}
	var approved []provenance.ToolLabel
	for _, selection := range in.Tools {
		item, ok := byTool[selection.Tool]
		if !ok || item.Proposal == nil || item.Stale || selection.SchemaHash == "" || selection.SchemaHash != item.Proposal.SchemaHash {
			return types.NewErrHTTP(409, fmt.Sprintf("The suggestion for %q is missing or stale. Request fresh suggestions before applying it.", selection.Tool))
		}
		label := item.ToolLabel
		label.Labels = slices.Clone(item.Proposal.Labels)
		label.Rationale = item.Proposal.Rationale
		label.Source = "admin"
		label.UpdatedAt = time.Now().UTC()
		label.Proposal = nil
		approved = append(approved, label)
	}
	for _, label := range approved {
		if err := h.engine.Store().PutToolLabel(req.Context(), &label); err != nil {
			return err
		}
	}
	h.engine.Invalidate()
	return h.writeToolLabelInventory(req, in.toolLabelResource)
}

func (h *ProvenanceHandler) writeToolLabelInventory(req api.Context, resource toolLabelResource) error {
	inventory, err := h.toolLabelInventory(req, resource)
	if err != nil {
		return err
	}
	return req.Write(inventory)
}

func (h *ProvenanceHandler) toolLabelInventory(req api.Context, resource toolLabelResource) (*toolLabelInventory, error) {
	labels, err := h.engine.Store().ListToolLabels(req.Context())
	if err != nil {
		return nil, err
	}
	return h.toolLabelInventoryFromLabels(req, resource, labels)
}

func (h *ProvenanceHandler) toolLabelInventoryFromLabels(req api.Context, resource toolLabelResource, labels []provenance.ToolLabel) (*toolLabelInventory, error) {
	if strings.TrimSpace(resource.ResourceID) == "" {
		return nil, types.NewErrBadRequest("resourceID is required")
	}
	out := &toolLabelInventory{toolLabelResource: resource, Items: []toolLabelSuggestion{}}
	add := func(tool types.MCPServerTool, context any) {
		properties := map[string]any{}
		for name, description := range tool.Params {
			properties[name] = map[string]any{"description": description}
		}
		out.Items = append(out.Items, toolLabelSuggestion{
			ToolLabel: provenance.ToolLabel{Server: provenance.ToolLabelScope(resource.ResourceType, resource.ResourceID), Tool: tool.Name,
				Labels: provenance.ToolCapabilityHeuristics(out.DisplayName, tool.Name, tool.Description), Source: "heuristic"},
			Description: tool.Description, schema: map[string]any{"type": "object", "properties": properties}, schemaHash: utils.Digest([]any{context, tool}),
		})
	}
	switch resource.ResourceType {
	case "catalog-entry":
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, resource.ResourceID); err != nil {
			return nil, err
		}
		if !entry.DeletionTimestamp.IsZero() {
			return nil, types.NewErrNotFound("catalog entry %q is being deleted", resource.ResourceID)
		}
		out.DisplayName, out.SourceURL = entry.Spec.Manifest.Name, entry.Spec.SourceURL
		for _, tool := range entry.Spec.Manifest.ToolPreview {
			if !tool.Unsupported {
				if _, err := namespace.Tool("", tool.Name); err != nil {
					return nil, types.NewErrBadRequest("invalid tool identity: %v", err)
				}
				add(tool, vmcpconfig.SourceDigest(types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}))
			}
		}
	case "vmcp":
		var vmcp v1.VMCP
		if err := req.Get(&vmcp, resource.ResourceID); err != nil {
			return nil, err
		}
		if !vmcp.DeletionTimestamp.IsZero() {
			return nil, types.NewErrNotFound("vMCP %q is being deleted", resource.ResourceID)
		}
		out.DisplayName, out.SourceURL = vmcp.Spec.Manifest.DisplayName, vmcp.Spec.SourceURL
		for _, component := range vmcp.Spec.Manifest.Components {
			var entry v1.MCPServerCatalogEntry
			tools := component.CatalogEntry.Manifest.ToolPreview
			// Tool previews are excluded from persisted vMCP snapshots. A
			// matching source may supply them without silently refreshing the
			// pinned runtime definition.
			if len(tools) == 0 && component.MCPServerCatalogEntryID != "" {
				err := req.Get(&entry, component.MCPServerCatalogEntryID)
				if err != nil && !apierrors.IsNotFound(err) {
					return nil, err
				}
				if err == nil && entry.DeletionTimestamp.IsZero() && vmcpconfig.SourceDigest(component.CatalogEntry) == vmcpconfig.SourceDigest(types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}) {
					tools = entry.Spec.Manifest.ToolPreview
				}
			}
			var prefix string
			if len(vmcp.Spec.Manifest.Components) > 1 || component.ToolPrefix != "" {
				var err error
				prefix, err = namespace.Prefix(component.Name, component.ToolPrefix)
				if err != nil {
					return nil, types.NewErrBadRequest("invalid component tool prefix: %v", err)
				}
			}
			for _, tool := range tools {
				if tool.Unsupported {
					continue
				}
				if len(component.ToolOverrides) > 0 {
					idx := slices.IndexFunc(component.ToolOverrides, func(override types.ToolOverride) bool { return override.Name == tool.Name })
					if idx == -1 || !component.ToolOverrides[idx].Enabled {
						continue
					}
					override := component.ToolOverrides[idx]
					if override.OverrideName != "" {
						tool.Name = override.OverrideName
					}
					if override.OverrideDescription != "" {
						tool.Description = override.OverrideDescription
					}
				}
				var err error
				tool.Name, err = namespace.Tool(prefix, tool.Name)
				if err != nil {
					return nil, types.NewErrBadRequest("invalid tool identity: %v", err)
				}
				add(tool, []any{component.ID, component.SourceDigest, component.CatalogEntry, component.ToolOverrides})
			}
		}
	default:
		return nil, types.NewErrBadRequest("resourceType must be catalog-entry or vmcp")
	}
	byTool := map[string]provenance.ToolLabel{}
	legacyByTool := map[string]provenance.ToolLabel{}
	for _, label := range labels {
		if label.Server == provenance.ToolLabelScope(resource.ResourceType, resource.ResourceID) {
			byTool[label.Tool] = label
		}
		if label.Server == out.DisplayName && label.Source == "admin" {
			legacyByTool[label.Tool] = label
		}
	}
	for i := range out.Items {
		item := &out.Items[i]
		if label, ok := byTool[item.Tool]; ok {
			item.ToolLabel = label
		}
		item.storedLabel = item.ToolLabel
		// Display the same explicit legacy fallback used by the gateway, but
		// never turn it into a new resource override merely by suggesting.
		if legacy, ok := legacyByTool[item.Tool]; ok && item.Source != "admin" {
			item.Labels, item.Source, item.Rationale, item.UpdatedAt = legacy.Labels, legacy.Source, legacy.Rationale, legacy.UpdatedAt
		}
		item.Stale = item.Proposal != nil && item.Proposal.SchemaHash != item.schemaHash
	}
	return out, nil
}
