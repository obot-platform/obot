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

// List returns all access control rules for a catalog or workspace.
func (*AccessControlRuleHandler) List(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	// Must have either catalog_id or workspace_id
	if catalogID == "" && workspaceID == "" {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Verify the scope exists
	if catalogID != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogID); err != nil {
			return types.NewErrBadRequest("catalog not found: %v", err)
		}
	} else {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return types.NewErrBadRequest("workspace not found: %v", err)
		}
	}

	var list v1.AccessControlRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	items := make([]types.AccessControlRule, 0, len(list.Items))
	for _, item := range list.Items {
		// Filter by catalog ID or workspace ID
		if catalogID != "" && item.Spec.MCPCatalogID == catalogID {
			items = append(items, convertAccessControlRule(item))
		} else if workspaceID != "" && item.Spec.PowerUserWorkspaceID == workspaceID {
			items = append(items, convertAccessControlRule(item))
		}
	}

	return req.Write(types.AccessControlRuleList{
		Items: items,
	})
}

// Get returns a specific access control rule by ID for a catalog or workspace.
func (*AccessControlRuleHandler) Get(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	ruleID := req.PathValue("access_control_rule_id")

	// Must have either catalog_id or workspace_id
	if catalogID == "" && workspaceID == "" {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Verify the scope exists
	if catalogID != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogID); err != nil {
			return types.NewErrBadRequest("catalog not found: %v", err)
		}
	} else {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return types.NewErrBadRequest("workspace not found: %v", err)
		}
	}

	var rule v1.AccessControlRule
	if err := req.Get(&rule, ruleID); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	// Verify rule belongs to the requested scope
	if catalogID != "" && rule.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	} else if workspaceID != "" && rule.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("access control rule does not belong to workspace %s", workspaceID)
	}

	return req.Write(convertAccessControlRule(rule))
}

// Create creates a new access control rule for a catalog or workspace.
func (h *AccessControlRuleHandler) Create(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	// Must have either catalog_id or workspace_id
	if catalogID == "" && workspaceID == "" {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Verify the scope exists
	if catalogID != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogID); err != nil {
			return types.NewErrBadRequest("catalog not found: %v", err)
		}
	} else {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return types.NewErrBadRequest("workspace not found: %v", err)
		}
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
			Manifest: manifest,
		},
	}

	// Set scope-specific fields
	if catalogID != "" {
		rule.Spec.MCPCatalogID = catalogID
	} else {
		rule.Spec.PowerUserWorkspaceID = workspaceID
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid access control rule manifest: %v", err)
	}

	// Validate that referenced resources exist in the same scope
	if catalogID != "" {
		if err := h.validateResourcesInCatalog(req, manifest.Resources, catalogID); err != nil {
			return err
		}
	} else {
		if err := h.validateResourcesInWorkspace(req, manifest.Resources, workspaceID); err != nil {
			return err
		}
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(rule))
}

// Update updates an existing access control rule for a catalog or workspace.
func (h *AccessControlRuleHandler) Update(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	ruleID := req.PathValue("access_control_rule_id")

	// Must have either catalog_id or workspace_id
	if catalogID == "" && workspaceID == "" {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Verify the scope exists
	if catalogID != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogID); err != nil {
			return types.NewErrBadRequest("catalog not found: %v", err)
		}
	} else {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return types.NewErrBadRequest("workspace not found: %v", err)
		}
	}

	var manifest types.AccessControlRuleManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read access control rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid access control rule manifest: %v", err)
	}

	var existing v1.AccessControlRule
	if err := req.Get(&existing, ruleID); err != nil {
		return types.NewErrBadRequest("failed to get access control rule: %v", err)
	}

	// Verify rule belongs to the requested scope
	if catalogID != "" && existing.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	} else if workspaceID != "" && existing.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("access control rule does not belong to workspace %s", workspaceID)
	}

	// Prevent updating generated access control rules
	if existing.Spec.Generated {
		return types.NewErrBadRequest("cannot update system-generated access control rule")
	}

	// Validate that referenced resources exist in the same scope
	if catalogID != "" {
		if err := h.validateResourcesInCatalog(req, manifest.Resources, catalogID); err != nil {
			return err
		}
	} else {
		if err := h.validateResourcesInWorkspace(req, manifest.Resources, workspaceID); err != nil {
			return err
		}
	}

	existing.Spec.Manifest = manifest
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(existing))
}

// Delete deletes an access control rule for a catalog or workspace.
func (*AccessControlRuleHandler) Delete(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	ruleID := req.PathValue("access_control_rule_id")

	// Must have either catalog_id or workspace_id
	if catalogID == "" && workspaceID == "" {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Get the rule first to verify it belongs to the correct scope
	var rule v1.AccessControlRule
	if err := req.Get(&rule, ruleID); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	// Verify rule belongs to the requested scope
	if catalogID != "" && rule.Spec.MCPCatalogID != catalogID {
		return types.NewErrBadRequest("access control rule does not belong to catalog %s", catalogID)
	} else if workspaceID != "" && rule.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("access control rule does not belong to workspace %s", workspaceID)
	}

	// Prevent deleting generated access control rules
	if rule.Spec.Generated {
		return types.NewErrBadRequest("cannot delete system-generated access control rule")
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

// validateResourcesInWorkspace validates that referenced resources exist in the specified workspace
func (*AccessControlRuleHandler) validateResourcesInWorkspace(req api.Context, resources []types.Resource, workspaceID string) error {
	for _, resource := range resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, resource.ID); err != nil {
				return types.NewErrBadRequest("MCPServerCatalogEntry %s not found: %v", resource.ID, err)
			}

			if entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrBadRequest("MCPServerCatalogEntry %s does not belong to workspace %s", resource.ID, workspaceID)
			}
		case types.ResourceTypeMCPServer:
			var server v1.MCPServer
			if err := req.Get(&server, resource.ID); err != nil {
				return types.NewErrBadRequest("MCPServer %s not found: %v", resource.ID, err)
			}

			if server.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrBadRequest("MCPServer %s does not belong to workspace %s", resource.ID, workspaceID)
			}
		case types.ResourceTypeSelector:
			// Selector resources are allowed within workspaces
		default:
			return types.NewErrBadRequest("unsupported resource type: %s", resource.Type)
		}
	}
	return nil
}

func convertAccessControlRule(rule v1.AccessControlRule) types.AccessControlRule {
	return types.AccessControlRule{
		Metadata:                  MetadataFrom(&rule),
		MCPCatalogID:              rule.Spec.MCPCatalogID,
		PowerUserWorkspaceID:      rule.Spec.PowerUserWorkspaceID,
		Generated:                 rule.Spec.Generated,
		AccessControlRuleManifest: rule.Spec.Manifest,
	}
}
