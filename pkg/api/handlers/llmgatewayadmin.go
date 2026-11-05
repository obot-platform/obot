package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/llmengine"
)

const gatewayAdminPrefix = "/api/llm-gateway/admin/"

// RegisterAdminRoutes uses Obot's API mux for method selection and named path values.
// Each operation shares the same request restrictions and durable-write follow-up.
func (h *LLMGatewayHandler) RegisterAdminRoutes(handle func(string, api.HandlerFunc)) {
	paths := map[string]bool{}
	register := func(pattern string, operation func(api.Context, []byte) (any, error)) {
		handler := h.admin(operation)
		handle(pattern, handler)
		// The previous wildcard handler accepted a trailing slash on every operation.
		handle(pattern+"/{$}", handler)

		_, path, _ := strings.Cut(pattern, " ")
		if !paths[path] {
			paths[path] = true
			// Obot's catch-all /api/ handler would otherwise turn unsupported methods into 404s.
			notAllowed := func(api.Context) error { return types.NewErrHTTP(http.StatusMethodNotAllowed, "Method Not Allowed") }
			handle(path, notAllowed)
			handle(path+"/{$}", notAllowed)
		}
	}

	register("GET /api/llm-gateway/admin/providers", func(req api.Context, body []byte) (any, error) {
		providers, err := h.engine.ListProviders(req.Context())
		return map[string]any{"providers": providers}, err
	})

	register("POST /api/llm-gateway/admin/providers", func(req api.Context, body []byte) (any, error) {
		return h.engine.CreateProvider(req.Context(), body)
	})

	register("GET /api/llm-gateway/admin/providers/{provider}", func(req api.Context, body []byte) (any, error) {
		return h.engine.GetProvider(req.Context(), req.PathValue("provider"))
	})

	register("PUT /api/llm-gateway/admin/providers/{provider}", func(req api.Context, body []byte) (any, error) {
		return h.engine.UpdateProvider(req.Context(), req.PathValue("provider"), body, req.URL.Query().Get("removeKeys") == "true")
	})

	register("DELETE /api/llm-gateway/admin/providers/{provider}", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "deleted"}, h.engine.DeleteProvider(req.Context(), req.PathValue("provider"))
	})

	register("POST /api/llm-gateway/admin/providers/{provider}/refresh-models", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "refreshed"}, h.engine.RefreshProviderModels(req.Context(), req.PathValue("provider"))
	})

	register("GET /api/llm-gateway/admin/keys", func(req api.Context, body []byte) (any, error) {
		keys, err := h.engine.ListKeys(req.Context())
		return map[string]any{"keys": keys}, err
	})

	register("GET /api/llm-gateway/admin/providers/{provider}/keys", func(req api.Context, body []byte) (any, error) {
		keys, err := h.engine.ListProviderKeys(req.Context(), req.PathValue("provider"))
		return map[string]any{"keys": keys}, err
	})

	register("POST /api/llm-gateway/admin/providers/{provider}/keys", func(req api.Context, body []byte) (any, error) {
		return h.engine.CreateKey(req.Context(), req.PathValue("provider"), body)
	})

	register("GET /api/llm-gateway/admin/providers/{provider}/keys/{key}", func(req api.Context, body []byte) (any, error) {
		return h.engine.GetKey(req.Context(), req.PathValue("provider"), req.PathValue("key"))
	})

	register("PUT /api/llm-gateway/admin/providers/{provider}/keys/{key}", func(req api.Context, body []byte) (any, error) {
		return h.engine.UpdateKey(req.Context(), req.PathValue("provider"), req.PathValue("key"), body)
	})

	register("DELETE /api/llm-gateway/admin/providers/{provider}/keys/{key}", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "deleted"}, h.engine.DeleteKey(req.Context(), req.PathValue("provider"), req.PathValue("key"))
	})

	register("GET /api/llm-gateway/admin/config", func(req api.Context, body []byte) (any, error) {
		return h.engine.GetClientConfig(req.Context())
	})

	register("PUT /api/llm-gateway/admin/config", func(req api.Context, body []byte) (any, error) {
		client, err := h.engine.UpdateClientConfig(req.Context(), body)
		return map[string]any{"client_config": client}, err
	})

	register("GET /api/llm-gateway/admin/proxy-config", func(req api.Context, body []byte) (any, error) {
		return h.engine.GetProxyConfig(req.Context())
	})

	register("PUT /api/llm-gateway/admin/proxy-config", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "success"}, h.engine.UpdateProxyConfig(req.Context(), body)
	})

	register("GET /api/llm-gateway/admin/version", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"version": llmengine.Version}, nil
	})

	register("GET /api/llm-gateway/admin/models", func(req api.Context, body []byte) (any, error) {
		models, err := h.engine.ListCatalogModels(req.Context(), req.URL.Query().Get("provider"), req.URL.Query().Get("query"))
		return map[string]any{"models": models}, err
	})

	register("POST /api/llm-gateway/admin/pricing/force-sync", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "success", "message": "pricing synced successfully"}, h.engine.RefreshPricing(req.Context())
	})

	register("GET /api/llm-gateway/admin/governance/routing-rules", func(req api.Context, body []byte) (any, error) {
		rules, err := h.engine.ListRoutingRules(req.Context())
		return map[string]any{"rules": rules}, err
	})

	register("POST /api/llm-gateway/admin/governance/routing-rules", func(req api.Context, body []byte) (any, error) {
		rule, err := h.engine.CreateRoutingRule(req.Context(), body)
		return map[string]any{"rule": rule}, err
	})

	register("GET /api/llm-gateway/admin/governance/routing-rules/{rule}", func(req api.Context, body []byte) (any, error) {
		rule, err := h.engine.GetRoutingRule(req.Context(), req.PathValue("rule"))
		return map[string]any{"rule": rule}, err
	})

	register("PUT /api/llm-gateway/admin/governance/routing-rules/{rule}", func(req api.Context, body []byte) (any, error) {
		rule, err := h.engine.UpdateRoutingRule(req.Context(), req.PathValue("rule"), body)
		return map[string]any{"rule": rule}, err
	})

	register("DELETE /api/llm-gateway/admin/governance/routing-rules/{rule}", func(req api.Context, body []byte) (any, error) {
		return map[string]any{"status": "deleted"}, h.engine.DeleteRoutingRule(req.Context(), req.PathValue("rule"))
	})

	register("GET /api/llm-gateway/admin/governance/pricing-overrides", func(req api.Context, body []byte) (any, error) {
		rows, err := h.engine.ListPricingOverrides(req.Context())
		return map[string]any{"pricing_overrides": rows}, err
	})

	register("POST /api/llm-gateway/admin/governance/pricing-overrides", func(req api.Context, body []byte) (any, error) {
		value, err := h.engine.CreatePricingOverride(req.Context(), body)
		return map[string]any{"pricing_override": value}, err
	})

	register("PUT /api/llm-gateway/admin/governance/pricing-overrides/{override}", func(req api.Context, body []byte) (any, error) {
		value, err := h.engine.UpdatePricingOverride(req.Context(), req.PathValue("override"), body)
		return map[string]any{"pricing_override": value}, err
	})

	register("DELETE /api/llm-gateway/admin/governance/pricing-overrides/{override}", func(req api.Context, body []byte) (any, error) {
		value, err := h.engine.DeletePricingOverride(req.Context(), req.PathValue("override"))
		return map[string]any{"pricing_override": value}, err
	})
}

func gatewayAdminError(err error) error {
	if err == nil {
		return nil
	}
	code := http.StatusBadRequest
	if apiErr, ok := errors.AsType[*llmengine.APIError](err); ok {
		code = apiErr.StatusCode
	}
	return types.NewErrHTTP(code, llmengine.ScrubEngineName(err.Error()))
}
