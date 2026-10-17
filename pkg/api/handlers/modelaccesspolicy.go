package handlers

import (
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelAccessPolicyHandler struct{}

func NewModelAccessPolicyHandler() *ModelAccessPolicyHandler {
	return nil
}

// List returns all model access policies.
func (*ModelAccessPolicyHandler) List(req api.Context) error {
	var list v1.ModelAccessPolicyList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list model access policies: %w", err)
	}

	items := make([]types.ModelAccessPolicy, 0, len(list.Items))
	for _, item := range list.Items {
		if !item.DeletionTimestamp.IsZero() {
			// Being deleted: it applies to no one, and goes once its finalizer is done.
			continue
		}
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
func (*ModelAccessPolicyHandler) Create(req api.Context) error {
	manifest, err := readAndValidateModelAccessPolicyManifest(req)
	if err != nil {
		return err
	}

	policy := v1.ModelAccessPolicy{
		GenerateName: system.ModelAccessPolicyPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.ModelAccessPolicySpec{
			Manifest: manifest,
		},
	}

	if err := writeNewGroupSubjects(req, manifest.Subjects, nil, func() error {
		if err := req.Create(&policy); err != nil {
			return fmt.Errorf("failed to create model access policy: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return req.Write(convertModelAccessPolicy(policy))
}

// Update updates an existing model access policy.
func (*ModelAccessPolicyHandler) Update(req api.Context) error {
	policyID := req.PathValue("id")

	manifest, err := readAndValidateModelAccessPolicyManifest(req)
	if err != nil {
		return err
	}

	var existing v1.ModelAccessPolicy
	if err := req.Get(&existing, policyID); err != nil {
		return types.NewErrBadRequest("failed to get model access policy: %v", err)
	}

	if err := writeNewGroupSubjects(req, manifest.Subjects, existing.Spec.Manifest.Subjects, func() error {
		existing.Spec.Manifest = manifest
		if err := req.Update(&existing); err != nil {
			return fmt.Errorf("failed to update model access policy: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return req.Write(convertModelAccessPolicy(existing))
}

// Delete deletes a model access policy, a gateway profile. Its finalizer deletes its budgets
// and guardrail policies. The profile stops applying to anyone as soon as it is marked for
// deletion, so a cleanup that fails can't leave a profile that grants models without its
// budgets and guardrail policies: the finalizer retries until it succeeds.
func (*ModelAccessPolicyHandler) Delete(req api.Context) error {
	policyID := req.PathValue("id")

	var policy v1.ModelAccessPolicy
	if err := req.Get(&policy, policyID); err != nil {
		return err
	}
	// The controller adds the finalizer when it first sees a policy. Add it here if it hasn't
	// yet, so a policy deleted right after it was created still has its budgets and guardrail policies
	// deleted.
	if !slices.Contains(policy.Finalizers, v1.ModelAccessPolicyFinalizer) {
		policy.Finalizers = append(policy.Finalizers, v1.ModelAccessPolicyFinalizer)
		if err := req.Update(&policy); err != nil {
			return err
		}
	}
	return req.Delete(&policy)
}

// requireProfile checks that a budget's or guardrail policy's gateway profile exists and isn't
// being deleted.
func requireProfile(req api.Context, profileID string) error {
	_, err := getProfile(req, profileID)
	return err
}

// getProfile returns a budget's or guardrail policy's gateway profile, checking that it exists
// and isn't being deleted.
func getProfile(req api.Context, profileID string) (*v1.ModelAccessPolicy, error) {
	if profileID == "" {
		return nil, types.NewErrBadRequest("profileID is required")
	}
	var policy v1.ModelAccessPolicy
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: profileID}, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, types.NewErrBadRequest("profile %q does not exist", profileID)
		}
		return nil, err
	}
	if !policy.DeletionTimestamp.IsZero() {
		return nil, types.NewErrBadRequest("profile %q is being deleted", profileID)
	}
	return &policy, nil
}

// confirmProfile checks, after a budget or guardrail policy was written, that its profile still
// exists and isn't being deleted. A profile deleted between requireProfile and the write
// would otherwise keep a budget or guardrail policy its finalizer had already passed over, which would
// then apply to a later profile with the same name. undo deletes what was written.
func confirmProfile(req api.Context, profileID string, undo func() error) error {
	err := requireProfile(req, profileID)
	if err == nil {
		return nil
	}
	if undoErr := undo(); undoErr != nil {
		return fmt.Errorf("profile %s was deleted, and removing what was written for it failed: %w", profileID, undoErr)
	}
	return err
}

func readAndValidateModelAccessPolicyManifest(req api.Context) (types.ModelAccessPolicyManifest, error) {
	var manifest types.ModelAccessPolicyManifest
	if err := req.Read(&manifest); err != nil {
		return manifest, types.NewErrBadRequest("failed to read model access policy manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return manifest, types.NewErrBadRequest("invalid model access policy manifest: %v", err)
	}
	if err := modelaccesspolicy.ValidateModelResources(
		req.Context(),
		req.Storage,
		req.Namespace(),
		manifest.Models,
	); err != nil {
		return manifest, types.NewErrBadRequest("invalid model access policy manifest: %v", err)
	}

	return manifest, nil
}

func convertModelAccessPolicy(policy v1.ModelAccessPolicy) types.ModelAccessPolicy {
	return types.ModelAccessPolicy{
		Metadata:                  MetadataFrom(&policy),
		ModelAccessPolicyManifest: policy.Spec.Manifest,
	}
}
