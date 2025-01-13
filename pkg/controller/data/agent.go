package data

import (
	"context"
	_ "embed"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed agent.yaml
var agentBytes []byte

func addAgent(ctx context.Context, k kclient.Client) error {
	var agent v1.Agent
	if err := yaml.Unmarshal(agentBytes, &agent); err != nil {
		return err
	}

	var existing v1.Agent
	if err := k.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: agent.Name}, &existing); apierrors.IsNotFound(err) {
		if err := k.Create(ctx, &agent); err != nil {
			return err
		}
		if err := k.Create(ctx, &v1.AgentAuthorization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      handlers.AgentAuthorizationName(agent.Name, "*"),
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.AgentAuthorizationSpec{
				AuthorizationManifest: types.AuthorizationManifest{
					UserID:  "*",
					AgentID: agent.Name,
				},
			},
		}); kclient.IgnoreAlreadyExists(err) != nil {
			return err
		}
		existing = agent
	} else if err != nil {
		return err
	}

	var modified bool
	modified, existing.Spec.Manifest.Tools = addTool(modified, &existing, existing.Spec.Manifest.Tools, agent.Spec.Manifest.Tools)
	modified, existing.Spec.Manifest.DefaultThreadTools = addTool(modified, &existing, existing.Spec.Manifest.DefaultThreadTools, agent.Spec.Manifest.DefaultThreadTools)
	modified, existing.Spec.Manifest.AvailableThreadTools = addTool(modified, &existing, existing.Spec.Manifest.AvailableThreadTools, agent.Spec.Manifest.AvailableThreadTools)

	if modified {
		return k.Update(ctx, &existing)
	}

	return nil
}

func addTool(alreadyModified bool, agent *v1.Agent, existing, newTools []string) (modified bool, result []string) {
	if alreadyModified {
		modified = true
	}

	result = existing
	for _, tool := range newTools {
		if agent.Annotations[tool] == "added" {
			continue
		}

		modified = true
		if agent.Annotations == nil {
			agent.Annotations = make(map[string]string)
		}
		agent.Annotations[tool] = "added"
		if !slices.Contains(result, tool) {
			result = append(result, tool)
		}
	}
	return
}
