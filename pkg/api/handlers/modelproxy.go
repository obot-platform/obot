package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcptester"
)

type modelProxySettingsStore interface {
	mcptester.ModelProxySettingsReader
	SetModelProxyEnabled(context.Context, bool) error
}

type ModelProxyHandler struct {
	settings      modelProxySettingsStore
	configuredURL string
	responsesURL  *url.URL
	license       mcptester.LicenseSource
	client        mcpTesterHTTPClient
}

func NewModelProxyHandler(settings modelProxySettingsStore, configuredURL string, responsesURL *url.URL, license mcptester.LicenseSource) *ModelProxyHandler {
	return &ModelProxyHandler{
		settings:      settings,
		configuredURL: configuredURL,
		responsesURL:  responsesURL,
		license:       license,
		client:        mcptester.NewModelProxyUsageHTTPClient(),
	}
}

func (h *ModelProxyHandler) Get(req api.Context) error {
	enabled, err := h.settings.ModelProxyEnabled(req.Context())
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "model proxy settings unavailable")
	}

	return req.Write(types.ModelProxySettings{Enabled: enabled, URL: h.configuredURL})
}

func (h *ModelProxyHandler) Update(req api.Context) error {
	var input types.ModelProxySettingsUpdate
	if err := req.Read(&input); err != nil || input.Enabled == nil {
		return types.NewErrHTTP(http.StatusBadRequest, "request must contain an enabled boolean")
	}

	if err := h.settings.SetModelProxyEnabled(req.Context(), *input.Enabled); err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "model proxy settings unavailable")
	}

	return req.Write(types.ModelProxySettings{Enabled: *input.Enabled, URL: h.configuredURL})
}

func (h *ModelProxyHandler) Usage(req api.Context) error {
	if h.responsesURL == nil {
		return types.NewErrHTTP(http.StatusConflict, "model proxy is disabled")
	}

	enabled, err := h.settings.ModelProxyEnabled(req.Context())
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "model proxy settings unavailable")
	}

	if !enabled {
		return types.NewErrHTTP(http.StatusConflict, "model proxy is disabled")
	}

	ctx, cancel := context.WithTimeout(req.Context(), mcptester.ModelProxyUsageTimeout)
	defer cancel()

	if h.license == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "installation license unavailable")
	}

	key, err := h.license.LicenseKey(ctx)
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "installation license unavailable")
	}
	if key == "" {
		return types.NewErrHTTP(http.StatusForbidden, "a valid installation license is required to read model proxy usage")
	}

	outbound, err := mcptester.NewModelProxyUsageRequest(ctx, h.responsesURL, key, h.license.MachineFingerprint(), req.Request.Header)
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "installation license or machine fingerprint unavailable")
	}

	response, err := h.client.Do(outbound)
	if err != nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "model proxy usage is temporarily unavailable")
	}

	defer response.Body.Close()

	usage, err := mcptester.ReadModelProxyUsage(response)
	if err != nil {
		if failure, ok := errors.AsType[*mcptester.ModelProxyUsageError](err); ok {
			if failure.RetryAfter != "" {
				req.ResponseWriter.Header().Set("Retry-After", failure.RetryAfter)
			}

			return types.NewErrHTTP(failure.Status, failure.Message)
		}

		return types.NewErrHTTP(http.StatusBadGateway, "invalid model proxy usage response")
	}

	return req.Write(usage)
}
