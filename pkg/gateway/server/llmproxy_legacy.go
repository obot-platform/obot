package server

import (
	"context"
	"errors"
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		reference = mapped
	}
	model, err := l.mapHelper.ResolveModelReference(req.Context(), req.Storage, system.DefaultNamespace, system.LLMGatewayModelProvider, reference)
	if apierrors.IsNotFound(err) {
		model, err = b.resolveModel(l, req.User, scopedLegacyReference(req.Context(), reference))
	}
	if err != nil {
		return nil, err
	}
	if scope := llmengine.LegacyProviderScope(req.Context()); scope != "" && !strings.HasPrefix(model.Spec.Manifest.TargetModel, scope+"/") {
		return nil, types.NewErrBadRequest("model is not available through this legacy provider; use /api/llm-proxy/v1 for other providers")
	}
	return model, nil
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
	// These legacy URLs are planned for deprecation in the next non-patch release.
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files", anthropic.files)
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files/{path...}", anthropic.files)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ca-cert", anthropic.agentProxyCACert)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ws", anthropic.agentProxyWebSocket)

	// Legacy provider routes are planned for deprecation in the next non-patch release.
	// New clients should use /api/llm-proxy/v1.
	for prefix, source := range legacyProxySources {
		mux.HandleFunc("/api/llm-proxy/"+prefix+"/{path...}", func(req api.Context) error {
			provider, err := s.llmGateway.LegacyGatewayProvider(req.Context(), source)
			if err != nil {
				if ae, ok := errors.AsType[*apierrors.StatusError](err); ok {
					return types.NewErrHTTP(int(ae.ErrStatus.Code), ae.ErrStatus.Message)
				}
				return err
			}
			normalized, err := normalizeLegacyProxyPath(prefix, req.PathValue("path"))
			if err != nil {
				return err
			}
			req.SetPathValue("path", normalized)
			req.Request = req.WithContext(llmengine.WithLegacyProviderScope(req.Context(), provider))
			return gatewayProxy.proxy(req)
		})
	}
}
