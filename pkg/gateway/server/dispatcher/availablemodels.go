package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	openai "github.com/obot-platform/chat-completion-client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (d *Dispatcher) ModelsForProvider(ctx context.Context, modelProvider v1.ModelProvider) (*openai.ModelsList, error) {
	u, err := d.urlForModelProvider(ctx, providerKeyForModelProvider(modelProvider.Namespace, modelProvider.Name), modelProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL for model provider %q: %w", modelProvider.Name, err)
	}

	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1"
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.JoinPath("models").String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to model provider %q: %w", modelProvider.Name, err)
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to model provider %q: %w", modelProvider.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get model list from model provider %q: %s", modelProvider.Name, message)
	}

	var oModels openai.ModelsList
	if err = json.NewDecoder(resp.Body).Decode(&oModels); err != nil {
		return nil, fmt.Errorf("failed to decode model list from model provider %q: %w", modelProvider.Name, err)
	}

	return &oModels, nil
}
