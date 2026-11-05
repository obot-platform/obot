package llmengine

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
)

func (e *Engine) gatewayModels(ctx context.Context, provider schemas.ModelProvider) ([]schemas.Model, error) {
	return e.listGatewayModels(ctx, provider, false)
}

func (e *Engine) listGatewayModels(ctx context.Context, provider schemas.ModelProvider, unfiltered bool) ([]schemas.Model, error) {
	bctx, cancel := schemas.NewBifrostContextWithCancel(ctx)
	defer cancel()
	// Preserve provider lifecycle fields through the per-provider post-hook.
	bctx.SetValue(schemas.BifrostContextKeyAllowPerRequestRawOverride, true)
	bctx.SetValue(schemas.BifrostContextKeySendBackRawResponse, true)
	req := &schemas.BifrostListModelsRequest{Provider: provider, PageSize: 10000, Unfiltered: unfiltered}
	var out *schemas.BifrostListModelsResponse
	var engineErr *schemas.BifrostError
	if provider == "" {
		out, engineErr = e.Client().ListAllModels(bctx, req)
	} else {
		out, engineErr = e.Client().ListModelsRequest(bctx, req)
	}
	if engineErr != nil {
		return nil, fmt.Errorf("model listing failed: %v", engineErr.Error)
	}
	for i := range out.Data {
		model := &out.Data[i]
		provider, id := schemas.ParseModelString(model.ID, "")
		entry := e.prices.GetPricingEntryForModel(id, provider)
		if entry == nil && model.Alias != nil {
			entry = e.prices.GetPricingEntryForModel(*model.Alias, provider)
		}
		// Core lists provider data; the catalog fills details formerly added by the transport.
		modelcatalog.ApplyModelInfo(model, entry)
	}
	out.Data = slices.DeleteFunc(out.Data, func(model schemas.Model) bool {
		return model.IsDeprecated || e.modelIsUnavailable(model.ID)
	})
	live := map[schemas.ModelProvider][]string{}
	if provider != "" {
		// A successful empty response must also replace the previous live listing.
		live[provider] = nil
	}
	for _, model := range out.Data {
		p, id, ok := strings.Cut(model.ID, "/")
		if ok {
			live[schemas.ModelProvider(p)] = append(live[schemas.ModelProvider(p)], id)
		}
	}
	for provider, models := range live {
		e.prices.UpsertLive(provider, "", unfiltered, models)
	}
	return out.Data, nil
}

func (e *Engine) serveModels(w http.ResponseWriter, r *http.Request) {
	provider := schemas.ModelProvider(r.URL.Query().Get("provider"))
	anthropic := strings.HasPrefix(r.URL.Path, "/anthropic/")
	if anthropic {
		provider = schemas.Anthropic
	}
	models, err := e.gatewayModels(r.Context(), provider)
	if err != nil {
		adminError(w, &APIError{StatusCode: 502, Message: err.Error()})
		return
	}
	if anthropic {
		data := []any{}
		for _, v := range models {
			_, id, ok := strings.Cut(v.ID, "/")
			if !ok {
				id = v.ID
			}
			data = append(data, map[string]any{"id": id, "type": "model", "display_name": v.Name, "created_at": "1970-01-01T00:00:00Z"})
		}
		adminJSON(w, 200, map[string]any{"data": data, "has_more": false})
		return
	}
	adminJSON(w, 200, map[string]any{"object": "list", "data": models})
}
