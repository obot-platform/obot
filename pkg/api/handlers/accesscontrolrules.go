package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccessControlRuleHandler struct{}

func NewAccessControlRuleHandler() *AccessControlRuleHandler {
	return &AccessControlRuleHandler{}
}

// List returns all access control rules for a catalog (admin only).
func (*AccessControlRuleHandler) List(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return types.NewErrBadRequest("catalog not found: %v", err)
	}

	var list v1.AccessControlRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	items := make([]types.AccessControlRule, 0, len(list.Items))
	for _, item := range list.Items {
		// Filter by catalog ID
		if item.Spec.MCPCatalogID == catalogID {
			items = append(items, convertAccessControlRule(item))
		}
	}

	return req.Write(types.AccessControlRuleList{
		Items: items,
	})
}

// Get returns a specific access control rule by ID (admin only).
func (*AccessControlRuleHandler) Get(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return types.NewErrBadRequest("catalog not found: %v", err)
	}

	var rule v1.AccessControlRule
	if err := req.Get(&rule, req.PathValue("access_control_rule_id")); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	// Verify rule belongs to the requested catalog
	if rule.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	}

	return req.Write(convertAccessControlRule(rule))
}

// Create creates a new access control rule (admin only).
func (h *AccessControlRuleHandler) Create(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return types.NewErrBadRequest("catalog not found: %v", err)
	}

	var manifest types.AccessControlRuleManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read access control rule manifest: %v", err)
	}

	rule := v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AccessControlRulePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: catalogID,
			Manifest:     manifest,
		},
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid access control rule manifest: %v", err)
	}

	// Validate that referenced resources exist in the same catalog
	if err := h.validateResourcesInCatalog(req, manifest.Resources, catalogID); err != nil {
		return types.NewErrBadRequest("%s", err.Error())
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(rule))
}

// Update updates an existing access control rule (admin only).
func (h *AccessControlRuleHandler) Update(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return types.NewErrBadRequest("catalog not found: %v", err)
	}

	var manifest types.AccessControlRuleManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read access control rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid access control rule manifest: %v", err)
	}

	var existing v1.AccessControlRule
	if err := req.Get(&existing, req.PathValue("access_control_rule_id")); err != nil {
		return types.NewErrBadRequest("failed to get access control rule: %v", err)
	}

	// Verify rule belongs to the requested catalog
	if existing.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	}

	if err := h.validateResourcesInCatalog(req, manifest.Resources, catalogID); err != nil {
		return types.NewErrBadRequest("%s", err.Error())
	}

	existing.Spec.Manifest = manifest
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(existing))
}

// Delete deletes an access control rule (admin only).
func (*AccessControlRuleHandler) Delete(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Get the rule first to verify it belongs to the catalog
	var rule v1.AccessControlRule
	ruleID := req.PathValue("access_control_rule_id")
	if err := req.Get(&rule, ruleID); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	// Verify rule belongs to the requested catalog
	if rule.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	}

	return req.Delete(&v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ruleID,
			Namespace: req.Namespace(),
		},
	})
}

// validateResourcesInCatalog validates that referenced resources exist in the specified catalog
func (*AccessControlRuleHandler) validateResourcesInCatalog(req api.Context, resources []types.Resource, catalogID string) error {
	for _, resource := range resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, resource.ID); err != nil {
				return types.NewErrBadRequest("MCPServerCatalogEntry %s not found: %v", resource.ID, err)
			}

			if entry.Spec.MCPCatalogName != catalogID {
				return types.NewErrBadRequest("MCPServerCatalogEntry %s does not belong to catalog %s", resource.ID, catalogID)
			}
		case types.ResourceTypeMCPServer:
			var server v1.MCPServer
			if err := req.Get(&server, resource.ID); err != nil {
				return types.NewErrBadRequest("MCPServer %s not found: %v", resource.ID, err)
			}

			// Check if server is shared within this catalog
			if server.Spec.SharedWithinMCPCatalogName != catalogID {
				return types.NewErrBadRequest("MCPServer %s does not belong to catalog %s", resource.ID, catalogID)
			}
		case types.ResourceTypeSelector:
			// Selector resources are allowed across all catalogs
		default:
			return types.NewErrBadRequest("unsupported resource type: %s", resource.Type)
		}
	}
	return nil
}

func convertAccessControlRule(rule v1.AccessControlRule) types.AccessControlRule {
	return types.AccessControlRule{
		Metadata:                  MetadataFrom(&rule),
		AccessControlRuleManifest: rule.Spec.Manifest,
	}
}
