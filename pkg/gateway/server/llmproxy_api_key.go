package server

import (
	"fmt"
	"net/http"
	"net/url"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type apiKeyLLMProviderBackend struct {
	u            url.URL
	providerName string
}

func (a apiKeyLLMProviderBackend) modelProviderName() string {
	return a.providerName
}

func (a apiKeyLLMProviderBackend) prepare(req api.Context, l *llmProviderProxy, modelProvider *v1.ModelProvider, body []byte) (*preparedLLMProxyRequest, error) {
	var tokenUsageTracker *threadSafeTokenUsageTracker
	targetModel := extractModelFromBody(body)
	if targetModel != "" {
		model, err := getModelFromReference(req.Context(), req.Storage, modelProvider.Namespace, targetModel)
		if apierrors.IsNotFound(err) {
			model, err = l.mapHelper.ResolveTargetModel(modelProvider.Name, targetModel)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get model: %w", err)
		}
		if model.Spec.Manifest.ModelProvider != modelProvider.Name {
			return nil, types2.NewErrBadRequest("requested model does not match configured provider %q", targetModel)
		}

		hasAccess, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check user access to model %q: %w", model.Name, err)
		}
		if !hasAccess {
			return nil, types2.NewErrForbidden("user does not have permission to use model %q", targetModel)
		}

		tokenUsageTracker = newTokenUsageTracker(*model)
		targetModel = model.Spec.Manifest.TargetModel
		rewritten, err := rewriteModelInBody(body, targetModel)
		if err != nil {
			return nil, fmt.Errorf("failed to rewrite model in request body: %w", err)
		}
		body = rewritten
	}

	return &preparedLLMProxyRequest{
		body:              body,
		model:             targetModel,
		modelProvider:     modelProvider.Name,
		tokenUsageTracker: tokenUsageTracker,
	}, nil
}

func (a apiKeyLLMProviderBackend) upstreamURL(*preparedLLMProxyRequest, map[string]string) (url.URL, error) {
	return a.u, nil
}

func (a apiKeyLLMProviderBackend) transport(modelProvider v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	credEnvKey, err := envVarForModelProvider(modelProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential environment key for model provider: %w", err)
	}
	return apiKeyTransport{providerName: modelProvider.Name, key: credEnv[credEnvKey], next: http.DefaultTransport}, nil
}

type apiKeyTransport struct {
	providerName string
	key          string
	next         http.RoundTripper
}

func (a apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch a.providerName {
	case system.AnthropicModelProvider:
		req.Header.Del("Authorization")
		req.Header.Set("X-Api-Key", a.key)
	case system.OpenAIModelProvider:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.key))
		req.Header.Del("X-Api-Key")
	default:
		if bearer := req.Header.Get("Authorization"); bearer != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.key))
		} else if token := req.Header.Get("X-Api-Key"); token != "" {
			req.Header.Set("X-Api-Key", a.key)
		}
	}
	next := a.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

func (a apiKeyLLMProviderBackend) proxyModelsList(api.Context, *llmProviderProxy, *v1.ModelProvider, map[string]string) (bool, error) {
	return false, nil
}
