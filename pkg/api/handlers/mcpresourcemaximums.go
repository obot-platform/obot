package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func validateK8sSettingsResourceMaximums(sessionManager *mcp.SessionManager, settings v1.K8sSettingsSpec) error {
	maximums := sessionManager.KubernetesResourceMaximums()
	if maximums.Empty() {
		return nil
	}
	if err := mcp.ValidateK8sSettingsResourceMaximums(settings, maximums); err != nil {
		return types.NewErrBadRequest("resource maximum validation failed: %v", err)
	}
	return nil
}
