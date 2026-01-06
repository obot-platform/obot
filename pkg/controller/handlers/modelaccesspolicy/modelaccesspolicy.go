package modelaccesspolicy

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// PruneModels ensures invalid and ineffectual model resources are removed from ModelAccessPolicies.
// This handler removes:
// - Models that no longer exist
// - Duplicates
// - Explicit model references when a wildcard is present
func PruneModels(req router.Request, _ router.Response) error {
	policy := req.Object.(*v1.ModelAccessPolicy)

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

		if alias, isAlias := resource.IsDefaultModelAliasRef(); isAlias {
			if types.DefaultModelAliasTypeFromString(alias) != types.DefaultModelAliasTypeUnknown {
				// Valid model alias type, keep
				resources = append(resources, resource)
			}

			continue
		}

		if !system.IsModelID(resource.ID) {
			// Prune invalid model ID
			continue
		}

		var model v1.Model
		if err := req.Get(&model, policy.Namespace, resource.ID); apierrors.IsNotFound(err) {
			// Prune missing model
			continue
		} else if err != nil {
			return err
		}

		resources = append(resources, resource)
	}

	if len(resources) == len(policy.Spec.Manifest.Models) {
		// Nothing was pruned, no update required
		return nil
	}

	// Update the models with the pruned resources
	policy.Spec.Manifest.Models = resources

	return req.Client.Update(req.Ctx, policy)
}
