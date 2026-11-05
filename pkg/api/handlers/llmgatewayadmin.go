package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/llmengine"
)

// adminMethods keeps HTTP operation shapes at the public boundary. Prefix validation
// alone cannot prevent an unsupported method or extra path segment from writing data.
func adminMethods(path string) []string {
	parts := strings.Split(path, "/")
	switch {
	case path == "providers", path == "governance/routing-rules", path == "governance/pricing-overrides":
		return []string{http.MethodGet, http.MethodPost}
	case path == "config", path == "proxy-config":
		return []string{http.MethodGet, http.MethodPut}
	case path == "keys", path == "models", path == "version":
		return []string{http.MethodGet}
	case path == "pricing/force-sync":
		return []string{http.MethodPost}
	case len(parts) == 2 && parts[0] == "providers" && parts[1] != "":
		return []string{http.MethodGet, http.MethodPut, http.MethodDelete}
	case len(parts) == 3 && parts[0] == "providers" && parts[1] != "" && parts[2] == "keys":
		return []string{http.MethodGet, http.MethodPost}
	case len(parts) == 3 && parts[0] == "providers" && parts[1] != "" && parts[2] == "refresh-models":
		return []string{http.MethodPost}
	case len(parts) == 4 && parts[0] == "providers" && parts[1] != "" && parts[2] == "keys" && parts[3] != "":
		return []string{http.MethodGet, http.MethodPut, http.MethodDelete}
	case len(parts) == 3 && parts[0] == "governance" && parts[2] != "" && parts[1] == "routing-rules":
		return []string{http.MethodGet, http.MethodPut, http.MethodDelete}
	case len(parts) == 3 && parts[0] == "governance" && parts[2] != "" && parts[1] == "pricing-overrides":
		return []string{http.MethodPut, http.MethodDelete}
	default:
		return nil
	}
}

func validateAdminOperation(method, path string) error {
	methods := adminMethods(path)
	if len(methods) == 0 {
		return types.NewErrNotFound("gateway API not found")
	}
	// GET routes also accepted HEAD through the former engine ServeMux.
	if method == http.MethodHead {
		method = http.MethodGet
	}
	if !slices.Contains(methods, method) {
		return types.NewErrHTTP(http.StatusMethodNotAllowed, "Method Not Allowed")
	}
	return nil
}

// adminOperation translates the established HTTP envelopes to named Engine calls.
// The engine owns validation and persistence, and has no administration HTTP surface.
func (h *LLMGatewayHandler) adminOperation(ctx context.Context, method, path string, query url.Values, body []byte) (any, error) {
	if err := validateAdminOperation(method, path); err != nil {
		return nil, err
	}
	if method == http.MethodHead {
		method = http.MethodGet
	}
	parts := strings.Split(path, "/")
	deleted := map[string]any{"status": "deleted"}

	switch parts[0] {
	case "providers":
		switch len(parts) {
		case 1:
			if method == http.MethodPost {
				return h.engine.CreateProvider(ctx, body)
			}
			providers, err := h.engine.ListProviders(ctx)
			return map[string]any{"providers": providers}, err
		case 2:
			switch method {
			case http.MethodPut:
				return h.engine.UpdateProvider(ctx, parts[1], body, query.Get("removeKeys") == "true")
			case http.MethodDelete:
				return deleted, h.engine.DeleteProvider(ctx, parts[1])
			default:
				return h.engine.GetProvider(ctx, parts[1])
			}
		case 3:
			if parts[2] == "refresh-models" {
				return map[string]any{"status": "refreshed"}, h.engine.RefreshProviderModels(ctx, parts[1])
			}
			if method == http.MethodPost {
				return h.engine.CreateKey(ctx, parts[1], body)
			}
			keys, err := h.engine.ListProviderKeys(ctx, parts[1])
			return map[string]any{"keys": keys}, err
		case 4:
			switch method {
			case http.MethodPut:
				return h.engine.UpdateKey(ctx, parts[1], parts[3], body)
			case http.MethodDelete:
				return deleted, h.engine.DeleteKey(ctx, parts[1], parts[3])
			default:
				return h.engine.GetKey(ctx, parts[1], parts[3])
			}
		}
	case "keys":
		keys, err := h.engine.ListKeys(ctx)
		return map[string]any{"keys": keys}, err
	case "config":
		if method == http.MethodPut {
			client, err := h.engine.UpdateClientConfig(ctx, body)
			return map[string]any{"client_config": client}, err
		}
		return h.engine.GetClientConfig(ctx)
	case "proxy-config":
		if method == http.MethodPut {
			return map[string]any{"status": "success"}, h.engine.UpdateProxyConfig(ctx, body)
		}
		return h.engine.GetProxyConfig(ctx)
	case "version":
		return map[string]any{"version": llmengine.Version}, nil
	case "models":
		models, err := h.engine.ListCatalogModels(ctx, query.Get("provider"), query.Get("query"))
		return map[string]any{"models": models}, err
	case "pricing":
		return map[string]any{"status": "success", "message": "pricing synced successfully"}, h.engine.RefreshPricing(ctx)
	case "governance":
		if parts[1] == "routing-rules" {
			if len(parts) == 2 {
				if method == http.MethodPost {
					value, err := h.engine.CreateRoutingRule(ctx, body)
					return map[string]any{"rule": value}, err
				}
				rules, err := h.engine.ListRoutingRules(ctx)
				return map[string]any{"rules": rules}, err
			}
			switch method {
			case http.MethodPut:
				value, err := h.engine.UpdateRoutingRule(ctx, parts[2], body)
				return map[string]any{"rule": value}, err
			case http.MethodDelete:
				return deleted, h.engine.DeleteRoutingRule(ctx, parts[2])
			default:
				rule, err := h.engine.GetRoutingRule(ctx, parts[2])
				return map[string]any{"rule": rule}, err
			}
		}
		if len(parts) == 2 {
			if method == http.MethodPost {
				value, err := h.engine.CreatePricingOverride(ctx, body)
				return map[string]any{"pricing_override": value}, err
			}
			rows, err := h.engine.ListPricingOverrides(ctx)
			return map[string]any{"pricing_overrides": rows}, err
		}
		if method == http.MethodPut {
			value, err := h.engine.UpdatePricingOverride(ctx, parts[2], body)
			return map[string]any{"pricing_override": value}, err
		}
		value, err := h.engine.DeletePricingOverride(ctx, parts[2])
		return map[string]any{"pricing_override": value}, err
	}
	return nil, types.NewErrNotFound("gateway API not found")
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
