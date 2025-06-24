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

// List returns all access control rules (admin only).
func (*AccessControlRuleHandler) List(req api.Context) error {
	var list v1.AccessControlRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var items []types.AccessControlRule
	for _, item := range list.Items {
		items = append(items, convertAccessControlRule(item))
	}

	return req.Write(types.AccessControlRuleList{
		Items: items,
	})
}

// Get returns a specific access control rule by ID (admin only).
func (*AccessControlRuleHandler) Get(req api.Context) error {
	var rule v1.AccessControlRule
	if err := req.Get(&rule, req.PathValue("access_control_rule_id")); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}
	return req.Write(convertAccessControlRule(rule))
}

// Create creates a new access control rule (admin only).
func (*AccessControlRuleHandler) Create(req api.Context) error {
	var manifest types.AccessControlRuleManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read access control rule manifest: %w", err)
	}

	rule := v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AccessControlRulePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AccessControlRuleSpec{
			DisplayName:                manifest.DisplayName,
			UserIDs:                    manifest.UserIDs,
			MCPServerCatalogEntryNames: manifest.MCPServerCatalogEntryIDs,
			MCPServerNames:             manifest.MCPServerIDs,
		},
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(rule))
}

// Update updates an existing access control rule (admin only).
func (*AccessControlRuleHandler) Update(req api.Context) error {
	var manifest types.AccessControlRuleManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read access control rule manifest: %w", err)
	}

	var existing v1.AccessControlRule
	if err := req.Get(&existing, req.PathValue("access_control_rule_id")); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	existing.Spec.DisplayName = manifest.DisplayName
	existing.Spec.UserIDs = manifest.UserIDs
	existing.Spec.MCPServerCatalogEntryNames = manifest.MCPServerCatalogEntryIDs
	existing.Spec.MCPServerNames = manifest.MCPServerIDs

	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update access control rule: %w", err)
	}

	return req.Write(convertAccessControlRule(existing))
}

// Delete deletes an access control rule (admin only).
func (*AccessControlRuleHandler) Delete(req api.Context) error {
	var rule v1.AccessControlRule
	if err := req.Get(&rule, req.PathValue("access_control_rule_id")); err != nil {
		return fmt.Errorf("failed to get access control rule: %w", err)
	}

	if err := req.Delete(&rule); err != nil {
		return fmt.Errorf("failed to delete access control rule: %w", err)
	}

	return req.Write(map[string]any{
		"id": rule.Name,
	})
}

func convertAccessControlRule(rule v1.AccessControlRule) types.AccessControlRule {
	return types.AccessControlRule{
		Metadata: MetadataFrom(&rule),
		AccessControlRuleManifest: types.AccessControlRuleManifest{
			DisplayName:              rule.Spec.DisplayName,
			UserIDs:                  rule.Spec.UserIDs,
			MCPServerCatalogEntryIDs: rule.Spec.MCPServerCatalogEntryNames,
			MCPServerIDs:             rule.Spec.MCPServerNames,
		},
	}
}
