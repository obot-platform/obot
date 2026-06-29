package handlers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/k8ssettings"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AppK8sSettingsHandler struct {
	localK8sClient   kclient.Client
	configSecret     string
	serviceNamespace string
}

func NewAppK8sSettingsHandler(configSecret, serviceNamespace string, localK8sClient kclient.Client) *AppK8sSettingsHandler {
	return &AppK8sSettingsHandler{
		localK8sClient:   localK8sClient,
		configSecret:     configSecret,
		serviceNamespace: serviceNamespace,
	}
}

func (h *AppK8sSettingsHandler) Get(req api.Context) error {
	settings, err := h.buildAppK8sSettings(req.Context())
	if err != nil {
		return err
	}
	return req.Write(settings)
}

func (h *AppK8sSettingsHandler) buildAppK8sSettings(ctx context.Context) (types.AppK8sSettings, error) {
	if h.localK8sClient == nil || h.configSecret == "" || h.serviceNamespace == "" {
		return types.AppK8sSettings{Available: false}, nil
	}

	values, err := k8ssettings.AppK8sSettingsValuesFromSecret(ctx, h.localK8sClient, h.serviceNamespace, h.configSecret)
	if err != nil {
		return types.AppK8sSettings{}, err
	}
	if len(values) == 0 {
		return types.AppK8sSettings{Available: false}, nil
	}

	settings, err := k8ssettings.ParseAppK8sSettings(values)
	if err != nil {
		return types.AppK8sSettings{}, err
	}
	return settings, nil
}
