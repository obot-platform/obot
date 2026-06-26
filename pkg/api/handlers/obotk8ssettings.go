package handlers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/obothelmvalues"
)

func (h *K8sSettingsHandler) buildObotK8sSettings(ctx context.Context) (types.ObotK8sSettings, error) {
	if !mcp.IsKubernetesBackend(h.mcpRuntimeBackend) {
		return types.ObotK8sSettings{Available: false}, nil
	}

	values, err := obothelmvalues.MaskedValuesFromSecret(ctx, h.localK8sClient, h.serviceNamespace, h.serviceName)
	if err != nil {
		return types.ObotK8sSettings{}, err
	}
	if len(values) == 0 {
		return types.ObotK8sSettings{Available: false}, nil
	}

	settings, err := obothelmvalues.ParseObotK8sSettings(values)
	if err != nil {
		return types.ObotK8sSettings{}, err
	}
	return settings, nil
}
