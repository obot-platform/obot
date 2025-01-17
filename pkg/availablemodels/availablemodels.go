package availablemodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
)

func ForProvider(ctx context.Context, dispatcher *dispatcher.Dispatcher, modelProviderNamespace, modelProviderName string) (*openai.ModelsList, error) {
	u, token, err := dispatcher.URLForModelProvider(ctx, modelProviderNamespace, modelProviderName)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL for model provider %q: %w", modelProviderName, err)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to model provider %q: %w", modelProviderName, err)
	}

	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to model provider %q: %w", modelProviderName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get model list from model provider %q: %s", modelProviderName, message)
	}

	var oModels openai.ModelsList
	if err = json.NewDecoder(resp.Body).Decode(&oModels); err != nil {
		return nil, fmt.Errorf("failed to decode model list from model provider %q: %w", modelProviderName, err)
	}

	return &oModels, nil
}
