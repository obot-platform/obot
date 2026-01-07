package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ModelAccessPolicyHandler struct{}

func NewModelAccessPolicyHandler() *ModelAccessPolicyHandler {
	return &ModelAccessPolicyHandler{}
}

// List returns all model access policies.
func (*ModelAccessPolicyHandler) List(req api.Context) error {
	var list v1.ModelAccessPolicyList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list model access policies: %w", err)
	}

	items := make([]types.ModelAccessPolicy, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertModelAccessPolicy(item))
	}

	return req.Write(types.ModelAccessPolicyList{
		Items: items,
	})
}

// Get returns a specific model access policy by ID.
func (*ModelAccessPolicyHandler) Get(req api.Context) error {
	policyID := req.PathValue("id")

	var policy v1.ModelAccessPolicy
	if err := req.Get(&policy, policyID); err != nil {
		return fmt.Errorf("failed to get model access policy: %w", err)
	}

	return req.Write(convertModelAccessPolicy(policy))
}

// Create creates a new model access policy.
func (h *ModelAccessPolicyHandler) Create(req api.Context) error {
	var manifest types.ModelAccessPolicyManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read model access policy manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid model access policy manifest: %v", err)
	}

	policy := v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ModelAccessPolicyPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ModelAccessPolicySpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&policy); err != nil {
		return fmt.Errorf("failed to create model access policy: %w", err)
	}

	return req.Write(convertModelAccessPolicy(policy))
}

// Update updates an existing model access policy.
func (h *ModelAccessPolicyHandler) Update(req api.Context) error {
	policyID := req.PathValue("id")

	var manifest types.ModelAccessPolicyManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read model access policy manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid model access policy manifest: %v", err)
	}

	var existing v1.ModelAccessPolicy
	if err := req.Get(&existing, policyID); err != nil {
		return types.NewErrBadRequest("failed to get model access policy: %v", err)
	}

	existing.Spec.Manifest = manifest
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update model access policy: %w", err)
	}

	return req.Write(convertModelAccessPolicy(existing))
}

// Delete deletes a model access policy.
func (*ModelAccessPolicyHandler) Delete(req api.Context) error {
	policyID := req.PathValue("id")

	return req.Delete(&v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyID,
			Namespace: req.Namespace(),
		},
	})
}

func convertModelAccessPolicy(policy v1.ModelAccessPolicy) types.ModelAccessPolicy {
	return types.ModelAccessPolicy{
		Metadata:                  MetadataFrom(&policy),
		ModelAccessPolicyManifest: policy.Spec.Manifest,
	}
}
