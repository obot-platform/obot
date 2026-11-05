package llmcore

import (
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// credentialPlugin also covers direct Client calls and plugin-rewritten attempts.
// Refuse caller credentials rather than silently substituting a paid provider key.
type credentialPlugin struct{}

func (*credentialPlugin) GetName() string { return "obot-caller-credentials" }
func (*credentialPlugin) Cleanup() error  { return nil }
func (*credentialPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (*credentialPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	headers, _ := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	if !hasCallerCredentials(headers) {
		return req, nil, nil
	}
	return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
		StatusCode:     new(http.StatusForbidden),
		AllowFallbacks: new(false),
		Error:          &schemas.ErrorField{Type: new("permission_denied"), Message: "Caller credentials are not supported; configure provider API keys in Obot"},
	}}, nil
}

func (*credentialPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func hasCallerCredentials(headers map[string][]string) bool {
	for name, values := range headers {
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") || strings.EqualFold(name, "api-key") || strings.EqualFold(name, "x-goog-api-key") {
			if strings.Join(values, "") != "" {
				return true
			}
		}
	}
	return false
}
