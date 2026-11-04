package llmcore

import (
	"maps"
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

type callerCredentialsKey struct{}

// credentialPlugin runs after request-rewriting plugins on every attempt. The
// proxy admits credentials for Anthropic only; routing and fallbacks must not
// carry them elsewhere or silently substitute a configured, paid gateway key.
type credentialPlugin struct{}

func (*credentialPlugin) GetName() string { return "obot-caller-credentials" }
func (*credentialPlugin) Cleanup() error  { return nil }
func (*credentialPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (*credentialPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	extra, _ := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	credentials, saved := ctx.Value(callerCredentialsKey{}).(map[string][]string)
	if !saved {
		credentials = make(map[string][]string)
		for name, values := range extra {
			switch strings.ToLower(name) {
			case "authorization", "x-api-key":
				if len(values) > 0 && strings.Join(values, "") != "" {
					credentials[strings.ToLower(name)] = append([]string(nil), values...)
				}
			}
		}
		// Retain the admitted credential across a refused fallback so a later
		// Anthropic attempt can still use it. Published header maps stay immutable.
		ctx.SetValue(callerCredentialsKey{}, credentials)
	}
	if len(credentials) == 0 {
		return req, nil, nil
	}
	headers := make(map[string][]string, len(extra)+len(credentials))
	maps.Copy(headers, extra)
	extra = headers
	for name := range extra {
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") {
			delete(extra, name)
		}
	}
	provider, _, _ := req.GetRequestFields()
	if provider == schemas.Anthropic {
		maps.Copy(extra, credentials)
	}
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	if provider != schemas.Anthropic {
		// SkipKeySelection is reserved by core and cannot be changed in hooks.
		// Refuse the attempt rather than bypassing that guard or using Obot's key.
		return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
			StatusCode:     schemas.Ptr(http.StatusForbidden),
			AllowFallbacks: schemas.Ptr(true),
			Error: &schemas.ErrorField{
				Type:    schemas.Ptr("permission_denied"),
				Message: "requests carrying caller credentials can only be served by Anthropic",
			},
		}}, nil
	}
	return req, nil, nil
}

func (*credentialPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}
