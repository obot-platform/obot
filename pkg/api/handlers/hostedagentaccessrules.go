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

type HostedAgentAccessRuleHandler struct{}

func NewHostedAgentAccessRuleHandler() *HostedAgentAccessRuleHandler {
	return nil
}

func (*HostedAgentAccessRuleHandler) List(req api.Context) error {
	var list v1.HostedAgentAccessRuleList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list hosted agent access rules: %w", err)
	}

	items := make([]types.HostedAgentAccessRule, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertHostedAgentAccessRule(item))
	}

	return req.Write(types.HostedAgentAccessRuleList{Items: items})
}

func (*HostedAgentAccessRuleHandler) Get(req api.Context) error {
	var rule v1.HostedAgentAccessRule
	if err := req.Get(&rule, req.PathValue("hosted_agent_access_rule_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent access rule: %w", err)
	}

	return req.Write(convertHostedAgentAccessRule(rule))
}

func (*HostedAgentAccessRuleHandler) Create(req api.Context) error {
	manifest, err := readAndValidateHostedAgentAccessRuleManifest(req)
	if err != nil {
		return err
	}

	rule := v1.HostedAgentAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.HostedAgentAccessRulePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.HostedAgentAccessRuleSpec{
			Manifest: *manifest,
		},
	}

	if err := req.Create(&rule); err != nil {
		return fmt.Errorf("failed to create hosted agent access rule: %w", err)
	}

	return req.WriteCreated(convertHostedAgentAccessRule(rule))
}

func (*HostedAgentAccessRuleHandler) Update(req api.Context) error {
	manifest, err := readAndValidateHostedAgentAccessRuleManifest(req)
	if err != nil {
		return err
	}

	var rule v1.HostedAgentAccessRule
	if err := req.Get(&rule, req.PathValue("hosted_agent_access_rule_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent access rule: %w", err)
	}

	rule.Spec.Manifest = *manifest
	if err := req.Update(&rule); err != nil {
		return fmt.Errorf("failed to update hosted agent access rule: %w", err)
	}

	return req.Write(convertHostedAgentAccessRule(rule))
}

func (*HostedAgentAccessRuleHandler) Delete(req api.Context) error {
	return req.Delete(&v1.HostedAgentAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("hosted_agent_access_rule_id"),
			Namespace: req.Namespace(),
		},
	})
}

func readAndValidateHostedAgentAccessRuleManifest(req api.Context) (*types.HostedAgentAccessRuleManifest, error) {
	var manifest types.HostedAgentAccessRuleManifest
	if err := req.Read(&manifest); err != nil {
		return nil, types.NewErrBadRequest("failed to read hosted agent access rule manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, types.NewErrBadRequest("invalid hosted agent access rule manifest: %v", err)
	}

	if err := validateReferencedHostedAgentResources(req, manifest.Resources); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func validateReferencedHostedAgentResources(req api.Context, resources []types.HostedAgentResource) error {
	for _, resource := range resources {
		switch resource.Type {
		case types.HostedAgentResourceTypeHostedAgent:
			if err := req.Get(&v1.HostedAgent{}, resource.ID); apierrors.IsNotFound(err) {
				return types.NewErrBadRequest("hosted agent %s not found", resource.ID)
			} else if err != nil {
				return fmt.Errorf("failed to get hosted agent %s: %w", resource.ID, err)
			}
		case types.HostedAgentResourceTypeSelector:
			// Wildcard selectors are validated by the manifest.
		default:
			return types.NewErrBadRequest("unsupported hosted agent resource type: %s", resource.Type)
		}
	}

	return nil
}

func convertHostedAgentAccessRule(rule v1.HostedAgentAccessRule) types.HostedAgentAccessRule {
	return types.HostedAgentAccessRule{
		Metadata:                      MetadataFrom(&rule),
		HostedAgentAccessRuleManifest: rule.Spec.Manifest,
	}
}
