package modelaccesspolicy

import (
	"context"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type invalidModelResourceError struct {
	message string
}

func (e *invalidModelResourceError) Error() string {
	return e.message
}

// IsInvalidModelResource reports whether err identifies a model resource that
// cannot be used in a model access policy.
func IsInvalidModelResource(err error) bool {
	var target *invalidModelResourceError
	return errors.As(err, &target)
}

// IsAllowedModelUsage reports whether usage can be granted by a model access policy.
func IsAllowedModelUsage(usage types.ModelUsage) bool {
	return usage == types.ModelUsageLLM
}

// IsAllowedDefaultModelAlias reports whether alias can be granted by a model access policy.
func IsAllowedDefaultModelAlias(alias string) bool {
	aliasType := types.DefaultModelAliasTypeFromString(alias)
	return aliasType == types.DefaultModelAliasTypeLLM || aliasType == types.DefaultModelAliasTypeLLMMini
}

// ValidateModelResources validates all explicit models and default aliases in a policy.
// Wildcard selectors are allowed because they are selection rules rather than concrete
// model additions.
func ValidateModelResources(
	ctx context.Context,
	reader kclient.Reader,
	namespace string,
	resources []types.ModelResource,
) error {
	for _, resource := range resources {
		if err := ValidateModelResource(ctx, reader, namespace, resource); err != nil {
			return err
		}
	}
	return nil
}

// ValidateModelResource validates one model access policy resource.
func ValidateModelResource(
	ctx context.Context,
	reader kclient.Reader,
	namespace string,
	resource types.ModelResource,
) error {
	if alias, ok := resource.IsDefaultModelAliasRef(); ok {
		if IsAllowedDefaultModelAlias(alias) {
			return nil
		}
		return invalidDefaultModelAlias(resource.ID)
	}

	if resource.IsWildcard() {
		return nil
	}
	if _, ok := resource.IsWildcardSuffix(); ok {
		return nil
	}
	if !system.IsModelID(resource.ID) {
		return &invalidModelResourceError{
			message: fmt.Sprintf("model %q must reference a valid model ID", resource.ID),
		}
	}

	var model v1.Model
	if err := reader.Get(ctx, kclient.ObjectKey{
		Namespace: namespace,
		Name:      resource.ID,
	}, &model); err != nil {
		return fmt.Errorf("failed to get model %q: %w", resource.ID, err)
	}
	if !IsAllowedModelUsage(model.Spec.Manifest.Usage) {
		return invalidModelUsage(resource.ID)
	}

	return nil
}

func invalidModelUsage(modelID string) error {
	return &invalidModelResourceError{
		message: fmt.Sprintf(
			"model %q must have a usage type of %q",
			modelID,
			types.ModelUsageLLM,
		),
	}
}

func invalidDefaultModelAlias(modelID string) error {
	return &invalidModelResourceError{
		message: fmt.Sprintf(
			"model %q must reference default model alias %q or %q",
			modelID,
			types.DefaultModelAliasTypeLLM,
			types.DefaultModelAliasTypeLLMMini,
		),
	}
}
