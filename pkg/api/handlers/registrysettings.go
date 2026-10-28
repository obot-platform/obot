package handlers

import (
	"context"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type registrySettingsStore interface {
	AnonymousRegistryAccessEnabled(context.Context) (bool, error)
	SetAnonymousRegistryAccessEnabled(context.Context, bool) error
}

type RegistrySettingsHandler struct {
	settings registrySettingsStore
}

func NewRegistrySettingsHandler(settings registrySettingsStore) *RegistrySettingsHandler {
	return &RegistrySettingsHandler{settings: settings}
}

func (h *RegistrySettingsHandler) Get(req api.Context) error {
	enabled, err := h.settings.AnonymousRegistryAccessEnabled(req.Context())
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "registry settings unavailable")
	}
	return req.Write(types.RegistrySettings{AnonymousAccessEnabled: enabled})
}

func (h *RegistrySettingsHandler) Update(req api.Context) error {
	var input types.RegistrySettings
	if err := req.Read(&input); err != nil {
		return types.NewErrHTTP(http.StatusBadRequest, "invalid registry settings request")
	}
	if err := h.settings.SetAnonymousRegistryAccessEnabled(req.Context(), input.AnonymousAccessEnabled); err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "registry settings unavailable")
	}
	return req.Write(input)
}
