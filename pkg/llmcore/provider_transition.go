package llmcore

import (
	"fmt"
	"net/http"

	"github.com/maximhq/bifrost/core/schemas"
)

// providerTransitionPlugin gates the core pipeline, not just HTTP admission, so
// direct Client calls and fallback attempts cannot use a transitioning provider.
// Register it last so it checks the provider after other plugins rewrite a request.
// Account key reads also gate workers that passed admission before a transition.
// Requests that already captured old credentials may finish while RemoveProvider
// drains them; the account does not publish new keys until that drain completes.
type providerTransitionPlugin struct {
	account *account
}

func (*providerTransitionPlugin) GetName() string { return "obot-provider-transition" }
func (*providerTransitionPlugin) Cleanup() error  { return nil }

func (p *providerTransitionPlugin) unavailable(req *schemas.BifrostRequest) error {
	provider, _, _ := req.GetRequestFields()
	if _, blocked := p.account.transitioning.Load(provider); blocked {
		return fmt.Errorf("provider %s is being reconfigured", provider)
	}
	return nil
}

func (p *providerTransitionPlugin) PreRequestHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	return p.unavailable(req)
}

func (p *providerTransitionPlugin) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if err := p.unavailable(req); err != nil {
		return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
			StatusCode: schemas.Ptr(http.StatusServiceUnavailable),
			Error:      &schemas.ErrorField{Type: schemas.Ptr("api_error"), Message: err.Error()},
		}}, nil
	}
	return req, nil, nil
}

func (*providerTransitionPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}
