package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SkillAccessRuleHandler struct{}

func NewSkillAccessRuleHandler() *SkillAccessRuleHandler {
	return &SkillAccessRuleHandler{}
}

func (*SkillAccessRuleHandler) List(req api.Context) error {
	var list v1.SkillAccessRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list skill access rules: %w", err)
	}

	items := make([]types.SkillAccessRule, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertSkillAccessRule(item))
	}

	return req.Write(types.SkillAccessRuleList{Items: items})
}

func (*SkillAccessRuleHandler) Get(req api.Context) error {
	var rule v1.SkillAccessRule
	if err := req.Get(&rule, req.PathValue("skill_access_rule_id")); err != nil {
		return fmt.Errorf("failed to get skill access rule: %w", err)
	}

	return req.Write(convertSkillAccessRule(rule))
}

func (h *SkillAccessRuleHandler) Create(req api.Context) error {
	manifest, err := h.readAndValidateManifest(req)
	if err != nil {
		return err
	}

	rule := v1.SkillAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SkillAccessRulePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.SkillAccessRuleSpec{
			Manifest: *manifest,
		},
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create skill access rule: %w", err)
	}

	return req.WriteCreated(convertSkillAccessRule(rule))
}

func (h *SkillAccessRuleHandler) Update(req api.Context) error {
	manifest, err := h.readAndValidateManifest(req)
	if err != nil {
		return err
	}

	var rule v1.SkillAccessRule
	if err := req.Get(&rule, req.PathValue("skill_access_rule_id")); err != nil {
		return fmt.Errorf("failed to get skill access rule: %w", err)
	}

	rule.Spec.Manifest = *manifest
	if err := req.Update(&rule); err != nil {
		return fmt.Errorf("failed to update skill access rule: %w", err)
	}

	return req.Write(convertSkillAccessRule(rule))
}

func (*SkillAccessRuleHandler) Delete(req api.Context) error {
	return req.Delete(&v1.SkillAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("skill_access_rule_id"),
			Namespace: req.Namespace(),
		},
	})
}

func (h *SkillAccessRuleHandler) readAndValidateManifest(req api.Context) (*types.SkillAccessRuleManifest, error) {
	var manifest types.SkillAccessRuleManifest
	if err := req.Read(&manifest); err != nil {
		return nil, types.NewErrBadRequest("failed to read skill access rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, types.NewErrBadRequest("invalid skill access rule manifest: %v", err)
	}

	if err := h.validateReferencedResources(req, manifest.Resources); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (*SkillAccessRuleHandler) validateReferencedResources(req api.Context, resources []types.SkillResource) error {
	for _, resource := range resources {
		switch resource.Type {
		case types.SkillResourceTypeSkill:
			if err := req.Get(&v1.Skill{}, resource.ID); apierrors.IsNotFound(err) {
				return types.NewErrBadRequest("skill %s not found", resource.ID)
			} else if err != nil {
				return fmt.Errorf("failed to get skill %s: %w", resource.ID, err)
			}
		case types.SkillResourceTypeSkillRepository:
			if err := req.Get(&v1.SkillRepository{}, resource.ID); apierrors.IsNotFound(err) {
				return types.NewErrBadRequest("skill repository %s not found", resource.ID)
			} else if err != nil {
				return fmt.Errorf("failed to get skill repository %s: %w", resource.ID, err)
			}
		case types.SkillResourceTypeSelector:
			// Wildcard selectors are validated by the manifest.
		default:
			return types.NewErrBadRequest("unsupported skill resource type: %s", resource.Type)
		}
	}

	return nil
}

func convertSkillAccessRule(rule v1.SkillAccessRule) types.SkillAccessRule {
	return types.SkillAccessRule{
		Metadata:                MetadataFrom(&rule),
		SkillAccessRuleManifest: rule.Spec.Manifest,
	}
}
