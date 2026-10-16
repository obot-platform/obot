package modelaccesspolicy

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	accesspolicy "github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// PruneDefaultPolicy ensures invalid and ineffectual model resources are removed from
// the default ModelAccessPolicy. Custom policies are intentionally left intact
// so their legacy resources remain visible for users to remediate.
// This handler removes:
// - Models that no longer exist
// - Models and default aliases that are not LLM usage types
// - Duplicates
// - Explicit model references when a wildcard is present
//
// Wildcard suffix patterns (e.g. "claude-haiku-4-5*") are always kept, even when
// they currently match no models, since they apply to future models as well.
func PruneDefaultPolicy(req router.Request, _ router.Response) error {
	policy := req.Object.(*v1.ModelAccessPolicy)
	if policy.Namespace != system.DefaultNamespace ||
		policy.Name != system.ModelAccessPolicyPrefix+"-default" {
		return nil
	}

	var (
		resources = make([]types.ModelResource, 0, len(policy.Spec.Manifest.Models))
		included  = make(map[types.ModelResource]struct{}, len(policy.Spec.Manifest.Models))
	)
	for _, resource := range policy.Spec.Manifest.Models {
		if _, ok := included[resource]; ok {
			// Prune duplicate resources
			continue
		}
		included[resource] = struct{}{}

		if resource.IsWildcard() {
			// Prune unnecessary explicit model references, wildcard model takes precedence.
			resources = []types.ModelResource{resource}
			break
		}

		err := accesspolicy.ValidateModelResource(
			req.Ctx,
			req.Client,
			policy.Namespace,
			resource,
		)
		if err == nil {
			resources = append(resources, resource)
			continue
		}
		if accesspolicy.IsInvalidModelResource(err) || apierrors.IsNotFound(err) {
			continue
		}
		return err
	}

	if len(resources) == len(policy.Spec.Manifest.Models) {
		// Nothing was pruned, no update required
		return nil
	}

	// Update the models with the pruned resources
	policy.Spec.Manifest.Models = resources

	return req.Client.Update(req.Ctx, policy)
}

// DeleteProfileBudgetsAndGuardrailPolicies returns the finalizer of a model access policy, which is a
// gateway profile: it deletes the profile's budgets and guardrail policies, retrying until it
// succeeds. Until then the policy isn't removed, so a policy created later with the same
// name, such as the Default Policy, can't take over another's budgets and guardrail policies. A policy
// marked for deletion already applies to no one.
func DeleteProfileBudgetsAndGuardrailPolicies(gateway *gatewayclient.Client) router.HandlerFunc {
	return func(req router.Request, _ router.Response) error {
		if err := gateway.DeleteLLMGatewayBudgetsForProfile(req.Ctx, req.Name); err != nil {
			return fmt.Errorf("failed to delete the budgets of profile %s: %w", req.Name, err)
		}
		if err := gateway.DeleteGuardrailPoliciesForProfile(req.Ctx, req.Name); err != nil {
			return fmt.Errorf("failed to delete the guardrail policies of profile %s: %w", req.Name, err)
		}
		return nil
	}
}
