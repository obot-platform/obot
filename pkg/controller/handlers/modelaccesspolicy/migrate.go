package modelaccesspolicy

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MigrateAgentAllowedModels ensures that the default agent's AllowedModels fields are cleared.
// This enables owners and admins to control model access for threads with ModelAccessPolicies.
// This handler also preserves the existing AllowedModels as a "user wildcard" MAP,
// ensuring that users retain access to models they were previously allowed to use.
// Custom agents are not migrated since they have minimal user support and non-standard configurations.
// Once the migration is complete, clearing AllowedModels prevents this handler from running again and allows
// threads descended from the agent to use models granted via ModelAccessPolicies.
func MigrateAgentAllowedModels(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.Agent)

	if !agent.Spec.Manifest.Default || len(agent.Spec.Manifest.AllowedModels) == 0 {
		// Migration complete, bail out
		return nil
	}

	// Preserve any existing chat configuration as an MAP before clearing it from the agent's manifest.
	var (
		policy         v1.ModelAccessPolicy
		policyName     = name.SafeConcatName(system.ModelAccessPolicyPrefix, agent.Name)
		policySubjects = []types.Subject{
			{
				Type: types.SubjectTypeSelector,
				ID:   "*",
			},
		}
		policyModels = convertToModelResources(agent.Spec.Manifest.AllowedModels)
	)

	// Make sure there are actually policies to copy before attempting to avoid producing an invalid MAP.
	if len(policyModels) > 0 {
		if err := req.Get(&policy, agent.Namespace, policyName); err == nil {
			// Rule already exists, determine if we should add new models.
			if !utils.SlicesEqualIgnoreOrder(policyModels, policy.Spec.Manifest.Models) ||
				!utils.SlicesEqualIgnoreOrder(policySubjects, policy.Spec.Manifest.Subjects) {
				// Replace the policy's subjects and models
				policy.Spec.Manifest.Subjects = policySubjects
				policy.Spec.Manifest.Models = policyModels
				if err := req.Client.Update(req.Ctx, &policy); err != nil {
					return fmt.Errorf("failed to update model access policy for agent %s: %w", agent.Name, err)
				}
			}
		} else if apierrors.IsNotFound(err) {
			// Create a new policy
			if err := req.Client.Create(req.Ctx, &v1.ModelAccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: agent.Namespace,
				},
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						DisplayName: "Migrated Policy",
						Subjects:    policySubjects,
						Models:      policyModels,
					},
				},
			}); err != nil {
				return fmt.Errorf("failed to create model access policy for agent %s: %w", agent.Name, err)
			}
		} else {
			return err
		}
	}

	// Clear AllowedModels from the agent.
	// This does three things:
	// 1. Prevent this migration handler from running again
	// 2. Allows threads descended from the agent to run invoke using models granted to the user via MAPs
	// 3. Allows the default model to be set on threads
	agent.Spec.Manifest.AllowedModels = nil

	return req.Client.Update(req.Ctx, agent)
}

// MigrateAgentDefaultModel ensures that the default agent's default model field is cleared.
// This handler will attempt to update the "LLM" DefaultModelAlias to point
// to the default model specified by the agent (if any). In most cases, this should preserve the default model
// used when creating and invoking threads.
func MigrateAgentDefaultModel(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.Agent)

	if !agent.Spec.Manifest.Default || agent.Spec.Manifest.Model == "" {
		return nil
	}

	var alias v1.DefaultModelAlias
	if err := req.Get(&alias, agent.Namespace, string(types.DefaultModelAliasTypeLLM)); err != nil {
		return fmt.Errorf("failed to get default model alias for agent %s: %w", agent.Name, err)
	}

	// Get the model to determine if we want to update the alias to point to it
	var agentModel v1.Model
	if err := kclient.IgnoreNotFound(req.Get(&agentModel, agent.Namespace, agent.Spec.Manifest.Model)); err == nil {
		if agentModel.Spec.Manifest.Active && agentModel.Name != alias.Spec.Manifest.Model {
			// The model is active and is different than the model the alias, update the alias
			alias.Spec.Manifest.Model = agentModel.Name
			if err := req.Client.Update(req.Ctx, &alias); err != nil {
				return fmt.Errorf("failed to update default model alias for agent %s: %w", agent.Name, err)
			}
		}
	} else {
		// This is an error other than NotFound
		return fmt.Errorf("failed to get model %s for agent %s: %w", agent.Spec.Manifest.Model, agent.Name, err)
	}

	// Clear out the agent's default model.
	// This will:
	// 1. Prevent this migration handler from running again
	// 2. Ensure that, when invoked, threads without an explicit default set on their manifest or on
	//    their root project will use the global default model after migration
	agent.Spec.Manifest.Model = ""

	return req.Client.Update(req.Ctx, agent)
}

// convertToModelResources converts an Agent's allowed models to a slice of ModelResource.
// Allowed models that are not model IDs -- i.e don't have the `m1` prefix -- and duplicates are omitted.
// This ensures the result doesn't reference ambiguous target model names; e.g. `gpt-5.2`.
func convertToModelResources(allowedModels []string) []types.ModelResource {
	var (
		resources = make([]types.ModelResource, 0, len(allowedModels))
		included  = make(map[string]struct{}, len(allowedModels))
	)
	for _, model := range allowedModels {
		if _, ok := included[model]; !ok && system.IsModelID(model) {
			resources = append(resources, types.ModelResource{
				ID: model,
			})
			included[model] = struct{}{}
		}
	}

	return resources
}
