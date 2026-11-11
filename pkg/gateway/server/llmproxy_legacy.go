package server

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	legacyProxySources = map[string]string{
		"openai":              system.OpenAIModelProvider,
		"anthropic":           system.AnthropicModelProvider,
		"generic-responses":   system.GenericResponsesModelProvider,
		"aws-bedrock":         system.AmazonBedrockModelProvider,
		"aws-bedrock-api-key": system.AmazonBedrockAPIKeyModelProvider,
		"azure":               system.AzureModelProvider,
		"azure-entra":         system.AzureEntraModelProvider,
	}
)

type proxyRouteRegistrar interface{ HandleFunc(string, api.HandlerFunc) }

func normalizeLegacyProxyPath(provider, path string) string {
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
	return path
}

func (s *Server) addLegacyLLMProxyRoutes(mux proxyRouteRegistrar, proxy *llmProviderProxy, anthropic *anthropicProxy) {
	for prefix, source := range legacyProxySources {
		mux.HandleFunc("/api/llm-proxy/"+prefix+"/{path...}", func(req api.Context) error {
			req.SetPathValue("path", normalizeLegacyProxyPath(prefix, req.PathValue("path")))
			req.Request = req.WithContext(llmengine.WithLegacySource(req.Context(), source))
			return proxy.proxy(req)
		})
	}
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files", anthropic.files)
	mux.HandleFunc("/api/llm-proxy/anthropic/v1/files/{path...}", anthropic.files)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ca-cert", anthropic.agentProxyCACert)
	mux.HandleFunc("GET /api/llm-proxy/anthropic/v1/code/agent-proxy/ws", anthropic.agentProxyWebSocket)
}

func resolveLegacyModel(l *llmProviderProxy, req api.Context, reference, scope string) (*v1.Model, error) {
	usable := func(m *v1.Model) bool {
		ok, err := l.mapHelper.UserHasAccessToModel(req.User, m.Name)
		return err == nil && ok
	}
	// Try the scoped native spelling first: native model IDs can themselves contain slashes.
	model, err := l.mapHelper.ResolveUpstreamModelName(system.LLMGatewayModelProvider, scope+"/"+reference, usable)
	if apierrors.IsNotFound(err) {
		model, err = l.mapHelper.ResolveModelReference(req.Context(), req.Storage, system.DefaultNamespace, system.LLMGatewayModelProvider, reference)
		if apierrors.IsNotFound(err) {
			model, err = l.mapHelper.ResolveUpstreamModelName(system.LLMGatewayModelProvider, reference, usable)
		}
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(model.Spec.Manifest.TargetModel, scope+"/") {
		return nil, types.NewErrBadRequest("model is not available through this legacy provider")
	}
	return model, nil
}
