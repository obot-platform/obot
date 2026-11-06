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

func (*providerTransitionPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	// Bifrost only logs errors from this phase. PreLLMHook enforces admission
	// with a short-circuit response for each attempt, including fallbacks.
	return nil
}

func (p *providerTransitionPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	// Recheck after trusted plugins have had their opportunity to rewrite the attempt.
	if denied := prepareSubscription(ctx, req, p.account); denied != nil {
		return req, denied, nil
	}

	if err := p.unavailable(req); err != nil {
		return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
			StatusCode: schemas.Ptr(http.StatusServiceUnavailable),
			Error:      &schemas.ErrorField{Type: new("api_error"), Message: err.Error()},
		}}, nil
	}
	return req, nil, nil
}

func (*providerTransitionPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

// Core creates the queue before PreLLMHook. A blocked account read fails there
// with no status, and core classifies it as caller input. Its %v wrapping erases
// the cause, so match only this local gate's complete message and provider stamp.
// Classify after attempts finish so routing and fallbacks still get to recover;
// explicit upstream statuses (including 400) always retain their meaning.
func isProviderConfigGateError(err *schemas.BifrostError) bool {
	if err.StatusCode != nil || err.Error == nil {
		return false
	}

	provider := err.ExtraFields.RoutingInfo.Provider
	if provider == "" {
		// Core v1.12.0 only fills the legacy stamp for failures before dispatch.
		provider = err.ExtraFields.Provider //nolint:staticcheck // Preserve early configuration-gate classification until core populates RoutingInfo.
	}
	if provider == "" {
		return false
	}

	return err.Error.Message == fmt.Sprintf("failed to get config for provider %s: provider %s is being reconfigured", provider, provider)
}
