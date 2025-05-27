package thread

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetModelAndModelProviderForProject(ctx context.Context, c kclient.Client, project *v1.Thread) (string, string, error) {
	if !project.Spec.Project {
		return "", "", fmt.Errorf("thread %s is not a project", project.Name)
	}

	if project.Spec.DefaultModelProvider != "" && project.Spec.DefaultModel != "" {
		return project.Spec.DefaultModel, project.Spec.DefaultModelProvider, nil
	}

	var (
		agent v1.Agent
		model string
	)

	// Check the base agent for a default model.
	if project.Spec.AgentName != "" {
		if err := c.Get(ctx, router.Key(project.Namespace, project.Spec.AgentName), &agent); err != nil {
			return "", "", err
		}

		model = agent.Spec.Manifest.Model
	}

	// If there wasn't one, just use the system default model.
	if model == "" {
		model = string(types.DefaultModelAliasTypeLLM)
	}

	return model, "", nil
}

func GetModelAndModelProviderForThread(ctx context.Context, c kclient.Client, thread *v1.Thread) (string, string, error) {
	modelProvider := thread.Spec.Manifest.ModelProvider
	model := thread.Spec.Manifest.Model

	// If it wasn't set on the thread, try to find a parent project that has it set.
	if modelProvider == "" || model == "" {
		project, err := projects.GetFirst(ctx, c, thread, func(thread *v1.Thread) (bool, error) {
			return thread.Spec.DefaultModelProvider != "" && thread.Spec.DefaultModel != "", nil
		})
		if err != nil {
			return "", "", err
		}

		if project != nil && project.Spec.DefaultModelProvider != "" && project.Spec.DefaultModel != "" {
			modelProvider = project.Spec.DefaultModelProvider
			model = project.Spec.DefaultModel
		} else {
			// If it wasn't set on the project, try to find a default on the agent.
			var agent v1.Agent
			if thread.Spec.AgentName != "" {
				if err := c.Get(ctx, router.Key(thread.Namespace, thread.Spec.AgentName), &agent); err != nil {
					return "", "", err
				}
				model = agent.Spec.Manifest.Model
				modelProvider = ""
			}

			// If it wasn't set on the agent, or if there is no agent, use the system-level default model.
			if model == "" {
				model = string(types.DefaultModelAliasTypeLLM)
				modelProvider = ""
			}
		}
	} else {
		// Make sure this model is allowed on the project.
		project, err := projects.GetRoot(ctx, c, thread)
		if err != nil {
			return "", "", err
		}
		if project != nil {
			if _, ok := project.Spec.Models[modelProvider]; !ok || !slices.Contains(project.Spec.Models[modelProvider], model) {
				return "", "", fmt.Errorf("model %q is not allowed on project", model)
			}
		} else {
			// Shouldn't happen
			return "", "", fmt.Errorf("project not found")
		}
	}

	return model, modelProvider, nil
}
