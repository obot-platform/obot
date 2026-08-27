package services

import (
	"context"
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AgentsEnabled returns the effective state of the agents feature.
//
// An explicitly configured value always wins. When the setting is unset,
// deployments that already contain agents are grandfathered in while new
// deployments keep the feature disabled.
func (s *Services) AgentsEnabled(ctx context.Context) (bool, error) {
	if s.EnableAgents != nil {
		return *s.EnableAgents, nil
	}

	// Only need to know whether at least one agent exists.
	var agents v1.NanobotAgentList
	if err := s.StorageClient.List(ctx, &agents, kclient.Limit(1)); err != nil {
		return false, fmt.Errorf("failed to list nanobot agents: %w", err)
	}

	// Cache the grandfathering decision so every startup consumer sees the same
	// feature state even if agents are added or removed while Obot is running.
	s.EnableAgents = new(len(agents.Items) > 0)
	return *s.EnableAgents, nil
}
