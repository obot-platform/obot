package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ModelPermissionRuleHandler struct{}

func NewModelPermissionRuleHandler() *ModelPermissionRuleHandler {
	return &ModelPermissionRuleHandler{}
}

// List returns all model permission rules.
func (*ModelPermissionRuleHandler) List(req api.Context) error {
	var list v1.ModelPermissionRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list model permission rules: %w", err)
	}

	items := make([]types.ModelPermissionRule, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertModelPermissionRule(item))
	}

	return req.Write(types.ModelPermissionRuleList{
		Items: items,
	})
}

// Get returns a specific model permission rule by ID.
func (*ModelPermissionRuleHandler) Get(req api.Context) error {
	ruleID := req.PathValue("id")

	var rule v1.ModelPermissionRule
	if err := req.Get(&rule, ruleID); err != nil {
		return fmt.Errorf("failed to get model permission rule: %w", err)
	}

	return req.Write(convertModelPermissionRule(rule))
}

// Create creates a new model permission rule.
func (h *ModelPermissionRuleHandler) Create(req api.Context) error {
	var manifest types.ModelPermissionRuleManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read model permission rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid model permission rule manifest: %v", err)
	}

	// Validate that referenced models exist (unless wildcard)
	if err := h.validateModels(req, manifest.Models); err != nil {
		return err
	}

	rule := v1.ModelPermissionRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ModelPermissionRulePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ModelPermissionRuleSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create model permission rule: %w", err)
	}

	return req.Write(convertModelPermissionRule(rule))
}

// Update updates an existing model permission rule.
func (h *ModelPermissionRuleHandler) Update(req api.Context) error {
	ruleID := req.PathValue("id")

	var manifest types.ModelPermissionRuleManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read model permission rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid model permission rule manifest: %v", err)
	}

	var existing v1.ModelPermissionRule
	if err := req.Get(&existing, ruleID); err != nil {
		return types.NewErrBadRequest("failed to get model permission rule: %v", err)
	}

	// Validate that referenced models exist (unless wildcard)
	if err := h.validateModels(req, manifest.Models); err != nil {
		return err
	}

	existing.Spec.Manifest = manifest
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update model permission rule: %w", err)
	}

	return req.Write(convertModelPermissionRule(existing))
}

// Delete deletes a model permission rule.
func (*ModelPermissionRuleHandler) Delete(req api.Context) error {
	ruleID := req.PathValue("id")

	return req.Delete(&v1.ModelPermissionRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ruleID,
			Namespace: req.Namespace(),
		},
	})
}

// validateModels validates that referenced model IDs exist
func (*ModelPermissionRuleHandler) validateModels(req api.Context, models []types.ModelResource) error {
	for _, model := range models {
		// Skip validation for wildcard selector
		if model.IsWildcard() {
			continue
		}

		var m v1.Model
		if err := req.Get(&m, model.ModelID); err != nil {
			return types.NewErrBadRequest("model %s not found: %v", model.ModelID, err)
		}
	}
	return nil
}

func convertModelPermissionRule(rule v1.ModelPermissionRule) types.ModelPermissionRule {
	return types.ModelPermissionRule{
		Metadata:                    MetadataFrom(&rule),
		ModelPermissionRuleManifest: rule.Spec.Manifest,
	}
}
