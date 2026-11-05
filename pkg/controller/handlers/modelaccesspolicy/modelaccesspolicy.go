package modelaccesspolicy

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
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
