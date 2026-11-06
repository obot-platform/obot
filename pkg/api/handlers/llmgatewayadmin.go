package handlers

import (
	"context"
	"errors"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/llmengine"
)

const (
	gatewayAdminPrefix = "/api/llm-gateway/admin/"
)

// RegisterAdminRoutes adds gateway administration handlers to Obot's API mux.
func (h *LLMGatewayHandler) RegisterAdminRoutes(mux *server.Server) {
	mux.HandleFunc("GET /api/llm-gateway/admin/providers", h.adminHandler(h.ListProviders))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers", h.adminHandler(h.CreateProvider))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}", h.adminHandler(h.GetProvider))
	mux.HandleFunc("PUT /api/llm-gateway/admin/providers/{provider}", h.adminHandler(h.UpdateProvider))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/providers/{provider}", h.adminHandler(h.DeleteProvider))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers/{provider}/refresh-models", h.adminHandler(h.RefreshProviderModels))
	mux.HandleFunc("GET /api/llm-gateway/admin/keys", h.adminHandler(h.ListKeys))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}/keys", h.adminHandler(h.ListProviderKeys))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers/{provider}/keys", h.adminHandler(h.CreateKey))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}/keys/{key}", h.adminHandler(h.GetKey))
	mux.HandleFunc("PUT /api/llm-gateway/admin/providers/{provider}/keys/{key}", h.adminHandler(h.UpdateKey))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/providers/{provider}/keys/{key}", h.adminHandler(h.DeleteKey))
	mux.HandleFunc("GET /api/llm-gateway/admin/config", h.adminHandler(h.GetClientConfig))
	mux.HandleFunc("PUT /api/llm-gateway/admin/config", h.adminHandler(h.UpdateClientConfig))
	mux.HandleFunc("GET /api/llm-gateway/admin/proxy-config", h.adminHandler(h.GetProxyConfig))
	mux.HandleFunc("PUT /api/llm-gateway/admin/proxy-config", h.adminHandler(h.UpdateProxyConfig))
	mux.HandleFunc("GET /api/llm-gateway/admin/version", h.adminHandler(h.Version))
	mux.HandleFunc("GET /api/llm-gateway/admin/models", h.adminHandler(h.ListCatalogModels))
	mux.HandleFunc("POST /api/llm-gateway/admin/pricing/force-sync", h.adminHandler(h.RefreshPricing))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/routing-rules", h.adminHandler(h.ListRoutingRules))
	mux.HandleFunc("POST /api/llm-gateway/admin/governance/routing-rules", h.adminHandler(h.CreateRoutingRule))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/routing-rules/{rule}", h.adminHandler(h.GetRoutingRule))
	mux.HandleFunc("PUT /api/llm-gateway/admin/governance/routing-rules/{rule}", h.adminHandler(h.UpdateRoutingRule))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/governance/routing-rules/{rule}", h.adminHandler(h.DeleteRoutingRule))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/pricing-overrides", h.adminHandler(h.ListPricingOverrides))
	mux.HandleFunc("POST /api/llm-gateway/admin/governance/pricing-overrides", h.adminHandler(h.CreatePricingOverride))
	mux.HandleFunc("PUT /api/llm-gateway/admin/governance/pricing-overrides/{override}", h.adminHandler(h.UpdatePricingOverride))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/governance/pricing-overrides/{override}", h.adminHandler(h.DeletePricingOverride))

	// Keep trailing-slash compatibility with the former gateway API.
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{$}", h.adminHandler(h.ListProviders))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers/{$}", h.adminHandler(h.CreateProvider))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}/{$}", h.adminHandler(h.GetProvider))
	mux.HandleFunc("PUT /api/llm-gateway/admin/providers/{provider}/{$}", h.adminHandler(h.UpdateProvider))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/providers/{provider}/{$}", h.adminHandler(h.DeleteProvider))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers/{provider}/refresh-models/{$}", h.adminHandler(h.RefreshProviderModels))
	mux.HandleFunc("GET /api/llm-gateway/admin/keys/{$}", h.adminHandler(h.ListKeys))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}/keys/{$}", h.adminHandler(h.ListProviderKeys))
	mux.HandleFunc("POST /api/llm-gateway/admin/providers/{provider}/keys/{$}", h.adminHandler(h.CreateKey))
	mux.HandleFunc("GET /api/llm-gateway/admin/providers/{provider}/keys/{key}/{$}", h.adminHandler(h.GetKey))
	mux.HandleFunc("PUT /api/llm-gateway/admin/providers/{provider}/keys/{key}/{$}", h.adminHandler(h.UpdateKey))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/providers/{provider}/keys/{key}/{$}", h.adminHandler(h.DeleteKey))
	mux.HandleFunc("GET /api/llm-gateway/admin/config/{$}", h.adminHandler(h.GetClientConfig))
	mux.HandleFunc("PUT /api/llm-gateway/admin/config/{$}", h.adminHandler(h.UpdateClientConfig))
	mux.HandleFunc("GET /api/llm-gateway/admin/proxy-config/{$}", h.adminHandler(h.GetProxyConfig))
	mux.HandleFunc("PUT /api/llm-gateway/admin/proxy-config/{$}", h.adminHandler(h.UpdateProxyConfig))
	mux.HandleFunc("GET /api/llm-gateway/admin/version/{$}", h.adminHandler(h.Version))
	mux.HandleFunc("GET /api/llm-gateway/admin/models/{$}", h.adminHandler(h.ListCatalogModels))
	mux.HandleFunc("POST /api/llm-gateway/admin/pricing/force-sync/{$}", h.adminHandler(h.RefreshPricing))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/routing-rules/{$}", h.adminHandler(h.ListRoutingRules))
	mux.HandleFunc("POST /api/llm-gateway/admin/governance/routing-rules/{$}", h.adminHandler(h.CreateRoutingRule))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/routing-rules/{rule}/{$}", h.adminHandler(h.GetRoutingRule))
	mux.HandleFunc("PUT /api/llm-gateway/admin/governance/routing-rules/{rule}/{$}", h.adminHandler(h.UpdateRoutingRule))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/governance/routing-rules/{rule}/{$}", h.adminHandler(h.DeleteRoutingRule))
	mux.HandleFunc("GET /api/llm-gateway/admin/governance/pricing-overrides/{$}", h.adminHandler(h.ListPricingOverrides))
	mux.HandleFunc("POST /api/llm-gateway/admin/governance/pricing-overrides/{$}", h.adminHandler(h.CreatePricingOverride))
	mux.HandleFunc("PUT /api/llm-gateway/admin/governance/pricing-overrides/{override}/{$}", h.adminHandler(h.UpdatePricingOverride))
	mux.HandleFunc("DELETE /api/llm-gateway/admin/governance/pricing-overrides/{override}/{$}", h.adminHandler(h.DeletePricingOverride))
}

func (h *LLMGatewayHandler) ListProviders(req api.Context) error {
	providers, err := h.engine.ListProviders(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"providers": providers})
}

func (h *LLMGatewayHandler) CreateProvider(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	providerType, err := h.validateProviderBody(req, body)
	if err != nil {
		return err
	}
	body, err = withProviderDefaults(body, true, providerType)
	if err != nil {
		return err
	}
	out, err := h.engine.CreateProvider(req.Context(), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	h.providerChanged(req)
	return req.Write(out)
}

func (h *LLMGatewayHandler) GetProvider(req api.Context) error {
	out, err := h.engine.GetProvider(req.Context(), req.PathValue("provider"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(out)
}

func (h *LLMGatewayHandler) UpdateProvider(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	providerType, err := h.validateProviderBody(req, body)
	if err != nil {
		return err
	}
	body, err = withProviderDefaults(body, false, providerType)
	if err != nil {
		return err
	}
	out, err := h.engine.UpdateProvider(req.Context(), req.PathValue("provider"), body, req.URL.Query().Get("removeKeys") == "true")
	if err != nil {
		return gatewayAdminError(err)
	}
	h.providerChanged(req)
	return req.Write(out)
}

func (h *LLMGatewayHandler) DeleteProvider(req api.Context) error {
	err := h.engine.DeleteProvider(req.Context(), req.PathValue("provider"))
	if err != nil {
		return gatewayAdminError(err)
	}
	// Do not silently re-enable subscriptions if Anthropic is added again.
	if req.PathValue("provider") == "anthropic" {
		disableSubscriptionPassthrough(context.WithoutCancel(req.Context()), req)
	}
	triggerGatewayModelSync(req)
	return req.Write(map[string]any{"status": "deleted"})
}

func (h *LLMGatewayHandler) RefreshProviderModels(req api.Context) error {
	_, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	err = h.engine.RefreshProviderModels(context.WithoutCancel(req.Context()), req.PathValue("provider"))
	if err != nil {
		return gatewayAdminError(err)
	}
	h.providerChanged(req)
	return req.Write(map[string]any{"status": "refreshed"})
}

func (h *LLMGatewayHandler) ListKeys(req api.Context) error {
	keys, err := h.engine.ListKeys(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"keys": keys})
}

func (h *LLMGatewayHandler) ListProviderKeys(req api.Context) error {
	keys, err := h.engine.ListProviderKeys(req.Context(), req.PathValue("provider"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"keys": keys})
}

func (h *LLMGatewayHandler) CreateKey(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	if _, err := h.validateProviderBody(req, body); err != nil {
		return err
	}
	out, err := h.engine.CreateKey(req.Context(), req.PathValue("provider"), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	h.providerChanged(req)
	return req.Write(out)
}

func (h *LLMGatewayHandler) GetKey(req api.Context) error {
	out, err := h.engine.GetKey(req.Context(), req.PathValue("provider"), req.PathValue("key"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(out)
}

func (h *LLMGatewayHandler) UpdateKey(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	if _, err := h.validateProviderBody(req, body); err != nil {
		return err
	}
	out, err := h.engine.UpdateKey(req.Context(), req.PathValue("provider"), req.PathValue("key"), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	h.providerChanged(req)
	return req.Write(out)
}

func (h *LLMGatewayHandler) DeleteKey(req api.Context) error {
	err := h.engine.DeleteKey(req.Context(), req.PathValue("provider"), req.PathValue("key"))
	if err != nil {
		return gatewayAdminError(err)
	}
	triggerGatewayModelSync(req)
	return req.Write(map[string]any{"status": "deleted"})
}

func (h *LLMGatewayHandler) GetClientConfig(req api.Context) error {
	out, err := h.engine.GetClientConfig(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(out)
}

func (h *LLMGatewayHandler) UpdateClientConfig(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	if err := rejectEngineAuthConfig(body); err != nil {
		return err
	}
	body, err = clientConfigOnly(body)
	if err != nil {
		return err
	}
	client, err := h.engine.UpdateClientConfig(req.Context(), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"client_config": client})
}

func (h *LLMGatewayHandler) GetProxyConfig(req api.Context) error {
	out, err := h.engine.GetProxyConfig(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(out)
}

func (h *LLMGatewayHandler) UpdateProxyConfig(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	if err := rejectGlobalProxySkipVerify(body); err != nil {
		return err
	}
	err = h.engine.UpdateProxyConfig(req.Context(), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"status": "success"})
}

func (h *LLMGatewayHandler) Version(req api.Context) error {
	return req.Write(map[string]any{"version": llmengine.Version})
}

func (h *LLMGatewayHandler) ListCatalogModels(req api.Context) error {
	models, err := h.engine.ListCatalogModels(req.Context(), req.URL.Query().Get("provider"), req.URL.Query().Get("query"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"models": models})
}

func (h *LLMGatewayHandler) RefreshPricing(req api.Context) error {
	_, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	err = h.engine.RefreshPricing(context.WithoutCancel(req.Context()))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"status": "success", "message": "pricing synced successfully"})
}

func (h *LLMGatewayHandler) ListRoutingRules(req api.Context) error {
	rules, err := h.engine.ListRoutingRules(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"rules": rules})
}

func (h *LLMGatewayHandler) CreateRoutingRule(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	rule, err := h.engine.CreateRoutingRule(req.Context(), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"rule": rule})
}

func (h *LLMGatewayHandler) GetRoutingRule(req api.Context) error {
	rule, err := h.engine.GetRoutingRule(req.Context(), req.PathValue("rule"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"rule": rule})
}

func (h *LLMGatewayHandler) UpdateRoutingRule(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	rule, err := h.engine.UpdateRoutingRule(req.Context(), req.PathValue("rule"), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"rule": rule})
}

func (h *LLMGatewayHandler) DeleteRoutingRule(req api.Context) error {
	err := h.engine.DeleteRoutingRule(req.Context(), req.PathValue("rule"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"status": "deleted"})
}

func (h *LLMGatewayHandler) ListPricingOverrides(req api.Context) error {
	rows, err := h.engine.ListPricingOverrides(req.Context())
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"pricing_overrides": rows})
}

func (h *LLMGatewayHandler) CreatePricingOverride(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	value, err := h.engine.CreatePricingOverride(req.Context(), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"pricing_override": value})
}

func (h *LLMGatewayHandler) UpdatePricingOverride(req api.Context) error {
	body, err := readGatewayAdminBody(req)
	if err != nil {
		return err
	}
	value, err := h.engine.UpdatePricingOverride(req.Context(), req.PathValue("override"), body)
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"pricing_override": value})
}

func (h *LLMGatewayHandler) DeletePricingOverride(req api.Context) error {
	value, err := h.engine.DeletePricingOverride(req.Context(), req.PathValue("override"))
	if err != nil {
		return gatewayAdminError(err)
	}
	return req.Write(map[string]any{"pricing_override": value})
}

func gatewayAdminError(err error) error {
	code := http.StatusBadRequest
	if apiErr, ok := errors.AsType[*apierrors.StatusError](err); ok {
		code = int(apiErr.ErrStatus.Code)
	}
	return types.NewErrHTTP(code, llmengine.ScrubEngineName(err.Error()))
}
