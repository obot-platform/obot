package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcptester"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

var (
	errMCPTesterLicenseRequired = errors.New("register an Obot license to use MCP Tester without a model provider")
)

type MCPTesterFallbackOptions struct {
	URL           *url.URL
	Providers     mcptester.ProviderConfigurationResolver
	License       mcptester.LicenseSource
	GatewayClient *client.Client
	Settings      mcptester.ModelProxySettingsReader
}

func (h *MCPTesterHandler) fallbackEnabled(ctx context.Context) (bool, error) {
	if h.fallback.URL == nil {
		return false, nil
	}

	availability, err := mcptester.ResolveFallbackAvailability(ctx, h.fallback.URL, h.fallback.Providers, h.fallback.Settings)
	return availability.Enabled, err
}

func (h *MCPTesterHandler) fallbackRequest(ctx context.Context, request types.MCPTesterChatRequest, server v1.MCPServer) (*http.Request, []byte, error) {
	body, err := mcptester.BuildFallbackRequest(request, testerSystemInstruction(server))
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if h.fallback.License == nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license is unavailable")
	}

	licenseKey, err := h.fallback.License.LicenseKey(ctx)
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license is unavailable")
	}

	if strings.TrimSpace(licenseKey) == "" {
		return nil, nil, errMCPTesterLicenseRequired
	}

	outbound, err := mcptester.NewFallbackRequest(ctx, h.fallback.URL, body, licenseKey, h.fallback.License.MachineFingerprint())
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license or machine fingerprint is unavailable")
	}

	return outbound, body, nil
}

func writeMCPTesterFallbackError(req api.Context, status int, input io.Reader) error {
	// Read only a bounded body and never forward proxy error text to the browser.
	var response struct {
		Error struct {
			Code  string `json:"code"`
			Quota struct {
				ResetAt time.Time `json:"reset_at"`
			} `json:"quota"`
		} `json:"error"`
	}

	_ = json.NewDecoder(io.LimitReader(input, mcpTesterProviderErrorLimit)).Decode(&response)

	switch status {
	case http.StatusUnauthorized:
		return writeMCPTesterError(req, http.StatusServiceUnavailable, types.MCPTesterErrorLicenseRequired, "Register an Obot license to use MCP Tester without a model provider.", false)
	case http.StatusForbidden:
		return writeMCPTesterError(req, http.StatusServiceUnavailable, types.MCPTesterErrorLicenseRequired, "The registered Obot license is invalid. Update the installation license to use MCP Tester.", false)
	case http.StatusTooManyRequests:
		if response.Error.Code == "daily_token_quota_exceeded" {
			message := "The installation's daily MCP Tester token budget is exhausted."
			if !response.Error.Quota.ResetAt.IsZero() {
				message += " It resets at " + response.Error.Quota.ResetAt.UTC().Format(time.RFC3339) + "."
			}

			return writeMCPTesterError(req, http.StatusTooManyRequests, types.MCPTesterErrorQuotaExceeded, message, false)
		}
	}

	return writeMCPTesterError(req, http.StatusBadGateway, types.MCPTesterErrorProvider, "The MCP Tester model service is temporarily unavailable. Try again later.", true)
}
