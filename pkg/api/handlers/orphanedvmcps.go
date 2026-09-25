package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"go.yaml.in/yaml/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// catalogVMCPItem is the catalog source form of a vMCP. Field order is the
// order in the generated YAML.
type catalogVMCPItem struct {
	Type        string                 `json:"type"`
	EntryKey    string                 `json:"entryKey"`
	DisplayName string                 `json:"displayName"`
	Description string                 `json:"description,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Components  []catalogVMCPComponent `json:"components"`
	// Profiles is never omitted: sync replaces a missing list with an admin-only default.
	Profiles []types.VMCPProfile `json:"profiles"`
}

type catalogVMCPComponent struct {
	// ID is always set so sync keeps the configuration and credentials scoped to it.
	ID                       string                          `json:"id"`
	Name                     string                          `json:"name"`
	MCPServerCatalogEntryKey string                          `json:"mcpServerCatalogEntryKey"`
	Configuration            []types.VMCPConfigurationPolicy `json:"configuration,omitempty"`
	AllowedTools             []string                        `json:"allowedTools,omitempty"`
	ToolPrefix               string                          `json:"toolPrefix,omitempty"`
	ToolOverrides            []types.ToolOverride            `json:"toolOverrides,omitempty"`
}

// orphanedVMCPError explains why a vMCP cannot be expressed as catalog YAML.
type orphanedVMCPError struct {
	msg string
}

// ListOrphanedVMCPs returns catalog YAML for orphaned vMCPs: vMCPs that were
// migrated from composite entries synced from the sourceURL query parameter and
// that catalog sync has not adopted yet. Publishing the YAML in that source lets sync take
// over each vMCP without disrupting its connections.
func (h *MCPCatalogHandler) ListOrphanedVMCPs(req api.Context) error {
	sourceURL := strings.TrimSpace(req.URL.Query().Get("sourceURL"))
	if sourceURL == "" {
		return types.NewErrBadRequest("sourceURL is required")
	}

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var vmcps v1.VMCPList
	if err := req.List(&vmcps); err != nil {
		return fmt.Errorf("failed to list vMCPs: %w", err)
	}

	sourceID := mcp.SourceIDForURL(sourceURL)
	items := make([]types.OrphanedVMCPCatalogItem, 0)
	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		if !isOrphaned(vmcp, catalog.Name, sourceID) {
			continue
		}

		item := types.OrphanedVMCPCatalogItem{
			VMCPID:      vmcp.Name,
			DisplayName: vmcp.Spec.Manifest.DisplayName,
			EntryKey:    vmcp.Spec.AdoptionEntryKey,
		}
		data, err := orphanedVMCPCatalogYAML(req, &catalog, vmcp)
		if _, ok := errors.AsType[orphanedVMCPError](err); ok {
			item.Error = err.Error()
		} else if err != nil {
			return err
		} else {
			item.YAML = string(data)
		}
		items = append(items, item)
	}

	slices.SortFunc(items, func(a, b types.OrphanedVMCPCatalogItem) int {
		if c := strings.Compare(a.DisplayName, b.DisplayName); c != 0 {
			return c
		}
		return strings.Compare(a.EntryKey, b.EntryKey)
	})
	return req.Write(types.OrphanedVMCPCatalogItemList{Items: items})
}

// isOrphaned reports whether vmcp was migrated from a composite in the catalog
// source and has not yet been taken over by catalog sync.
func isOrphaned(vmcp *v1.VMCP, catalogName, sourceID string) bool {
	if vmcp.Spec.Adopted == nil || *vmcp.Spec.Adopted || !vmcp.DeletionTimestamp.IsZero() || vmcp.Spec.UserID != "" ||
		vmcp.Spec.AdoptionEntryKey == "" || mcp.SourceIDForURL(vmcp.Spec.AdoptionSourceURL) != sourceID {
		return false
	}
	// Sync adopts by name, so generated YAML is only useful if it produces this name.
	return vmcp.Name == mcpcatalog.VMCPName(catalogName, vmcp.Spec.AdoptionSourceURL, vmcp.Spec.AdoptionEntryKey, vmcp.Spec.Manifest.DisplayName)
}

func (e orphanedVMCPError) Error() string {
	return e.msg
}

func orphanedVMCPCatalogYAML(req api.Context, catalog *v1.MCPCatalog, vmcp *v1.VMCP) ([]byte, error) {
	sourceID := mcp.SourceIDForURL(vmcp.Spec.AdoptionSourceURL)
	item := catalogVMCPItem{
		Type:        "vmcp",
		EntryKey:    vmcp.Spec.AdoptionEntryKey,
		DisplayName: vmcp.Spec.Manifest.DisplayName,
		Description: vmcp.Spec.Manifest.Description,
		Icon:        vmcp.Spec.Manifest.Icon,
		Components:  make([]catalogVMCPComponent, 0, len(vmcp.Spec.Manifest.Components)),
		Profiles:    vmcp.Spec.Manifest.Profiles,
	}
	if item.Profiles == nil {
		item.Profiles = []types.VMCPProfile{}
	}

	for _, component := range vmcp.Spec.Manifest.Components {
		reference, err := catalogEntryReference(req, catalog, sourceID, component)
		if err != nil {
			return nil, err
		}

		configuration := make([]types.VMCPConfigurationPolicy, 0, len(component.Configuration))
		for _, policy := range component.Configuration {
			// Sync keeps fixed values already stored for this component ID.
			configuration = append(configuration, types.VMCPConfigurationPolicy{Key: policy.Key, Policy: policy.Policy})
		}
		item.Components = append(item.Components, catalogVMCPComponent{
			ID:                       component.ID,
			Name:                     component.Name,
			MCPServerCatalogEntryKey: reference,
			Configuration:            configuration,
			AllowedTools:             component.AllowedTools,
			ToolPrefix:               component.ToolPrefix,
			ToolOverrides:            component.ToolOverrides,
		})
	}

	return marshalBlockYAML(item)
}

// catalogEntryReference returns the mcpServerCatalogEntryKey that resolves to
// the component's catalog entry from a vMCP in the source identified by sourceID.
func catalogEntryReference(req api.Context, catalog *v1.MCPCatalog, sourceID string, component types.VMCPComponent) (string, error) {
	if system.IsMCPServerID(component.MCPServerCatalogEntryID) {
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses multi-user MCP server %q, which has no catalog entry that a catalog vMCP can reference", component.Name, component.MCPServerCatalogEntryID)}
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, component.MCPServerCatalogEntryID); apierrors.IsNotFound(err) {
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses catalog entry %q, which no longer exists", component.Name, component.MCPServerCatalogEntryID)}
	} else if err != nil {
		return "", fmt.Errorf("failed to get catalog entry %q: %w", component.MCPServerCatalogEntryID, err)
	}

	switch {
	case entry.Spec.MCPCatalogName != catalog.Name:
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses catalog entry %q, which is not in catalog %q", component.Name, entry.Spec.Manifest.Name, catalog.Name)}
	case entry.Spec.SourceURL == "":
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses catalog entry %q, which is not synced from a catalog source", component.Name, entry.Spec.Manifest.Name)}
	case entry.Spec.Manifest.EntryKey == "":
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses catalog entry %q from %s, which has no entryKey; add an entryKey to that entry so the vMCP can reference it", component.Name, entry.Spec.Manifest.Name, entry.Spec.SourceURL)}
	}

	entrySourceID := mcp.SourceIDForURL(entry.Spec.SourceURL)
	// An entry keeps its old source URL until the catalog syncs a changed source.
	// A reference built from it would not resolve once that sync runs.
	if !slices.ContainsFunc(catalog.Spec.SourceURLs, func(sourceURL string) bool {
		return mcp.SourceIDForURL(sourceURL) == entrySourceID
	}) {
		return "", orphanedVMCPError{fmt.Sprintf("component %q uses catalog entry %q from %s, which is no longer a source of catalog %q; sync the catalog and generate the YAML again", component.Name, entry.Spec.Manifest.Name, entry.Spec.SourceURL, catalog.Name)}
	}
	if entrySourceID != sourceID {
		return entrySourceID + "::" + entry.Spec.Manifest.EntryKey, nil
	}
	return entry.Spec.Manifest.EntryKey, nil
}

// marshalBlockYAML renders obj as block-style YAML using its JSON field names
// and field order.
func marshalBlockYAML(obj any) ([]byte, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	// JSON is YAML, and decoding it into a node keeps the struct field order.
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	clearYAMLStyle(&node)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// clearYAMLStyle drops the flow and quoting styles inherited from JSON. The
// encoder still quotes strings that would otherwise decode as another type.
func clearYAMLStyle(node *yaml.Node) {
	node.Style = 0
	for _, child := range node.Content {
		clearYAMLStyle(child)
	}
}
