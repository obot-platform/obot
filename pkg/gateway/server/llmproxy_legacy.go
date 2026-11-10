package server

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	legacyProxySources = map[string]string{
		"openai": system.OpenAIModelProvider, "anthropic": system.AnthropicModelProvider,
		"generic-responses": system.GenericResponsesModelProvider,
		"aws-bedrock":       system.AmazonBedrockModelProvider, "aws-bedrock-api-key": system.AmazonBedrockAPIKeyModelProvider,
		"azure": system.AzureModelProvider, "azure-entra": system.AzureEntraModelProvider,
	}
)

// proxyRouteRegistrar shares production route registration with HTTP tests.
type proxyRouteRegistrar interface{ HandleFunc(string, api.HandlerFunc) }

func normalizeLegacyProxyPath(provider, path string) (string, error) {
	path = strings.Trim(path, "/")
	if provider == "aws-bedrock" || provider == "aws-bedrock-api-key" {
		path = strings.TrimPrefix(strings.TrimPrefix(path, "anthropic/"), "openai/")
	}
	if (provider == "azure" || provider == "azure-entra") && path == "openai/v1/models" {
		path = "v1/models"
	}
	if !strings.HasPrefix(path, "v1/") {
		path = "v1/" + path
	}
	reqPath := strings.TrimPrefix(path, "v1/")
	switch reqPath {
	case "messages", "messages/count_tokens", "responses", "responses/input_tokens", "chat/completions", "embeddings", "models":
		return path, nil
	default:
		return "", types.NewErrHTTP(410, "this legacy operation is not supported; use a supported endpoint under /api/llm-proxy/v1")
	}
}

func scopedLegacyReference(ctx context.Context, reference string) string {
	scope := llmengine.LegacyProviderScope(ctx)
	if scope != "" && !strings.Contains(reference, "/") {
		return scope + "/" + reference
	}
	return reference
}

func (b *bifrostProviderBackend) resolveRequestModel(l *llmProviderProxy, req api.Context, reference string) (*v1.Model, error) {
	mapped, err := b.engine.MigratedReference(req.Context(), reference)
	if err != nil {
		return nil, err
	}
	if mapped != "" {
		// Sources can survive migration cleanup; their deactivation still governs explicit old-ID requests.
		var source v1.Model
		if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: strings.TrimSpace(reference)}, &source); err == nil {
			if !source.Spec.Manifest.Active {
				return nil, types.NewErrNotFound("model %q is not available through this endpoint", reference)
			}
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		reference = mapped
	}
	scope := llmengine.LegacyProviderScope(req.Context())
	var model *v1.Model
	if scope != "" && strings.Contains(reference, "/") {
		// A native ID may contain slashes and even match another provider's qualified target.
		model, err = b.resolveModel(l, req.User, scope+"/"+reference)
	}
	if model == nil && (err == nil || apierrors.IsNotFound(err)) {
		model, err = l.mapHelper.ResolveModelReference(req.Context(), req.Storage, system.DefaultNamespace, system.LLMGatewayModelProvider, reference)
	}
	if apierrors.IsNotFound(err) {
		model, err = b.resolveModel(l, req.User, scopedLegacyReference(req.Context(), reference))
	}
	if err != nil {
		if apierrors.IsNotFound(err) && (scope == "bedrock" || scope == "bedrock_mantle") {
			return nil, types.NewErrHTTP(http.StatusGone, "model is not available through this legacy AWS provider; use /api/llm-proxy/v1 with the appropriate model")
		}
		return nil, err
	}
	if scope != "" && !strings.HasPrefix(model.Spec.Manifest.TargetModel, scope+"/") {
		return nil, types.NewErrBadRequest("model is not available through this legacy provider; use /api/llm-proxy/v1 for other providers")
	}
	return model, nil
}

func (s *Server) legacyAWSProvider() (string, error) {
	if s.llmGateway == nil || !s.llmGateway.Ready() {
		return "", types.NewErrHTTP(http.StatusServiceUnavailable, "the LLM gateway is unavailable")
	}
	providers, err := s.llmGateway.Client().GetConfiguredProviders()
	if err != nil {
		return "", err
	}
	// Prefer existing v0.28 configuration; import targets do not determine request routing.
	for _, provider := range []schemas.ModelProvider{schemas.Bedrock, schemas.BedrockMantle} {
		if slices.Contains(providers, provider) {
			return string(provider), nil
		}
	}
	if err := s.llmGateway.Err(); err != nil {
		return "", err
	}
	return "", types.NewErrHTTP(http.StatusGone, "no AWS provider is configured for this legacy route; use /api/llm-proxy/v1 with a configured model")
}

func (s *Server) addLLMProxyRoutes(mux proxyRouteRegistrar) {
	// LLM proxy. The LLM gateway serves every model, in every API format, at the root of the
	// LLM proxy.
	gatewayProxy := s.newBifrostLLMProviderProxy()
	mux.HandleFunc("/api/llm-proxy/v1/{path...}", func(req api.Context) error {
		req.SetPathValue("path", "v1/"+req.PathValue("path"))
		return gatewayProxy.proxy(req)
	})
	// Claude Code sends some Anthropic API requests that name no model to its API base URL too.
	anthropic := newAnthropicProxy()
	mux.HandleFunc("/api/llm-proxy/v1/files", anthropic.files)
	mux.HandleFunc("/api/llm-proxy/v1/files/{path...}", anthropic.files)
	mux.HandleFunc("GET /api/llm-proxy/v1/code/agent-proxy/ca-cert", anthropic.agentProxyCACert)
	mux.HandleFunc("GET /api/llm-proxy/v1/code/agent-proxy/ws", anthropic.agentProxyWebSocket)

	// Subscription-backed, model-free Anthropic requests also retain their old URL.
	// These legacy URLs are planned for deprecation in an upcoming non-patch release.
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files", anthropic.files)
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files/{path...}", anthropic.files)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ca-cert", anthropic.agentProxyCACert)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ws", anthropic.agentProxyWebSocket)

	// Legacy provider routes are planned for deprecation in an upcoming non-patch release.
	// New clients should use /api/llm-proxy/v1.
	for prefix, source := range legacyProxySources {
		mux.HandleFunc("/api/llm-proxy/"+prefix+"/{path...}", func(req api.Context) error {
			normalized, err := normalizeLegacyProxyPath(prefix, req.PathValue("path"))
			if err != nil {
				return err
			}
			provider := llmengine.LegacyGatewayProvider(source)
			if prefix == "aws-bedrock" || prefix == "aws-bedrock-api-key" {
				provider, err = s.legacyAWSProvider()
				if err != nil {
					return err
				}
			}
			req.SetPathValue("path", normalized)
			req.Request = req.WithContext(llmengine.WithLegacyProviderScope(req.Context(), provider))
			return gatewayProxy.proxy(req)
		})
	}
}
