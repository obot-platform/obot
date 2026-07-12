package modelinfosource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// providerIDToObotProvider maps source provider IDs to Obot provider names.
var providerIDToObotProvider = map[string]string{
	"anthropic": system.AnthropicModelProvider,
	"openai":    system.OpenAIModelProvider,
}

// fetchModelInfos GETs the models.dev document at the source URL and parses it
// into ModelInfo objects.
func (h *Handler) fetchModelInfos(ctx context.Context, source *v1.ModelInfoSource) ([]kclient.Object, error) {
	url := source.Spec.Manifest.URL
	if url == "" {
		return nil, fmt.Errorf("model info source has no source URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}

	var doc modelsDevDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to decode model info source: %w", err)
	}
	return parseModelInfos(source.Namespace, source.Name, doc)
}

// modelsDevDocument is the source response shape consumed by the sync.
type modelsDevDocument map[string]struct {
	Models map[string]struct {
		Cost modelsDevCost `json:"cost"`
	} `json:"models"`
}

// modelsDevCost is one source model cost entry.
type modelsDevCost struct {
	Input      float64         `json:"input"`
	Output     float64         `json:"output"`
	CacheRead  float64         `json:"cache_read"`
	CacheWrite float64         `json:"cache_write"`
	Tiers      []modelsDevTier `json:"tiers"`
}

type modelsDevTier struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Tier       struct {
		Type string `json:"type"`
		Size int    `json:"size"`
	} `json:"tier"`
}

// parseModelInfos converts a models.dev document into ModelInfo objects in the
// given namespace, owned by the named source.
func parseModelInfos(namespace, sourceName string, doc modelsDevDocument) ([]kclient.Object, error) {
	var infos []kclient.Object
	for providerID, obotProvider := range providerIDToObotProvider {
		provider, ok := doc[providerID]
		if !ok {
			continue
		}

		for modelID, m := range provider.Models {
			infos = append(infos, &v1.ModelInfo{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      v1.ModelInfoName(obotProvider, modelID),
				},
				Spec: v1.ModelInfoSpec{
					ModelInfoSourceName: sourceName,
					Provider:            obotProvider,
					Model:               modelID,
					Cost:                modelCost(m.Cost, obotProvider),
				},
			})
		}
	}

	if len(infos) < 1 {
		return nil, fmt.Errorf("no models found for known providers")
	}

	return infos, nil
}

// modelCost converts a models.dev cost entry to a ModelCost.
func modelCost(c modelsDevCost, obotProvider string) types.ModelCost {
	cost := types.ModelCost{
		TokenUsageCost: tokenUsageCost(c.Input, c.Output, c.CacheRead, c.CacheWrite, obotProvider),
	}
	for _, tr := range c.Tiers {
		tierType := types.ModelCostTierType(tr.Tier.Type)
		if tierType != types.ModelCostTierTypeContext || tr.Tier.Size <= 0 {
			continue
		}
		cost.Tiers = append(cost.Tiers, types.ModelCostTier{
			TokenUsageCost: tokenUsageCost(tr.Input, tr.Output, tr.CacheRead, tr.CacheWrite, obotProvider),
			Type:           tierType,
			Size:           new(tr.Tier.Size),
		})
	}
	return cost
}

func tokenUsageCost(input, output, cacheRead, cacheWrite float64, obotProvider string) types.TokenUsageCost {
	cost := types.TokenUsageCost{
		Input:      input,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
		Output:     output,
	}
	if obotProvider == system.AnthropicModelProvider {
		cost.CacheWrite1h = input * 2
	}
	return cost
}
