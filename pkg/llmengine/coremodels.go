package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
)

func (e *Engine) gatewayModels(ctx context.Context, provider schemas.ModelProvider) ([]schemas.Model, error) {
	return e.listGatewayModels(ctx, provider, false)
}

func (e *Engine) listGatewayModels(ctx context.Context, provider schemas.ModelProvider, unfiltered bool) ([]schemas.Model, error) {
	if !e.Ready() {
		return nil, apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}

	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}

	// Bifrost fans out across providers; share one immutable configuration snapshot.
	ctx = context.WithValue(ctx, modelListingConfigKey{}, &cfg)
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
		// Catalog metadata describes the wire model, even if an alias reuses another model's name.
		if model.Alias != nil {
			id = *model.Alias
		}

		if base := azureBaseModel(*model); base != "" {
			id = base
		}

		entry := e.prices.GetPricingEntryForModel(id, provider)
		// Core lists provider data; the catalog fills details formerly added by the transport.
		modelcatalog.ApplyModelInfo(model, entry)
	}
	live := map[schemas.ModelProvider][]string{}
	if provider != "" {
		// A successful empty response must also replace the previous live listing.
		live[provider] = nil
	}
	// Record providers before filtering so removing their last unavailable model
	// also clears their previous live catalog entries.
	for _, model := range out.Data {
		p, _, ok := strings.Cut(model.ID, "/")
		if ok {
			live[schemas.ModelProvider(p)] = nil
		}
	}
	out.Data = slices.DeleteFunc(out.Data, func(model schemas.Model) bool {
		return model.IsDeprecated || e.modelMetadataIsUnavailable(model.ID, model.Alias, azureBaseModel(model))
	})
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
		adminError(w, &apierrors.StatusError{ErrStatus: metav1.Status{Code: http.StatusBadGateway, Message: err.Error()}})
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

func adminJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func adminError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if apiErr, ok := errors.AsType[*apierrors.StatusError](err); ok {
		code = int(apiErr.ErrStatus.Code)
	}
	adminJSON(w, code, map[string]any{"error": map[string]any{"message": ScrubEngineName(err.Error())}})
}
