package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcptester"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	mcpTesterLicenseInvalidMessage = "The registered Obot license is invalid. Update the installation license to use MCP Tester."
)

var (
	errMCPTesterLicenseRequired = errors.New("register an Obot license to use MCP Tester without a model provider")
	errMCPTesterLicenseInvalid  = errors.New("the registered Obot license is invalid")
)

// MCPTesterModelProxyOptions configures the model proxy used when no model provider
// is configured and the proxy is enabled. It is not used when a configured provider fails.
type MCPTesterModelProxyOptions struct {
	URL           *url.URL
	Providers     mcptester.ProviderConfigurationResolver
	License       mcptester.LicenseSource
	GatewayClient *client.Client
	Settings      mcptester.ModelProxySettingsReader
}

func (h *MCPTesterHandler) modelProxyEnabled(ctx context.Context) (bool, error) {
	if h.modelProxy.URL == nil {
		return false, nil
	}

	availability, err := mcptester.ResolveModelProxyAvailability(ctx, h.modelProxy.URL, h.modelProxy.Providers, h.modelProxy.Settings)
	return availability.Enabled, err
}

func (h *MCPTesterHandler) modelProxyRequest(ctx context.Context, request types.MCPTesterChatRequest, server v1.MCPServer, inbound http.Header) (*http.Request, []byte, error) {
	body, err := mcptester.BuildModelProxyRequest(request, testerSystemInstruction(server))
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if h.modelProxy.License == nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license is unavailable")
	}

	licenseKey, err := h.modelProxy.License.LicenseKey(ctx)
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license is unavailable")
	}

	if licenseKey == "" {
		return nil, nil, errMCPTesterLicenseRequired
	}

	machineID, err := h.modelProxy.License.MachineID(ctx)
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license is unavailable")
	}

	// Only a valid license has an activated machine.
	if machineID == "" {
		return nil, nil, errMCPTesterLicenseInvalid
	}

	outbound, err := mcptester.NewModelProxyRequest(ctx, h.modelProxy.URL, body, licenseKey, machineID, inbound)
	if err != nil {
		return nil, nil, types.NewErrHTTP(http.StatusServiceUnavailable, "installation license or machine ID is unavailable")
	}

	return outbound, body, nil
}

func writeMCPTesterModelProxyError(req api.Context, status int, input io.Reader) error {
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
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorLicenseRequired, "Register an Obot license to use MCP Tester without a model provider.", false)
	case http.StatusForbidden:
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorLicenseRequired, mcpTesterLicenseInvalidMessage, false)
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
