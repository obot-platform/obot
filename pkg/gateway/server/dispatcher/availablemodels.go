package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	openai "github.com/gptscript-ai/chat-completion-client"
)

func (d *Dispatcher) ModelsForProvider(ctx context.Context, modelProviderNamespace, modelProviderName string) (*openai.ModelsList, error) {
	return d.ModelsForProviderWithEnv(ctx, modelProviderNamespace, modelProviderName, nil)
}

func (d *Dispatcher) ModelsForProviderWithEnv(ctx context.Context, modelProviderNamespace, modelProviderName string, env map[string]string) (*openai.ModelsList, error) {
	u, err := d.URLForModelProvider(ctx, modelProviderNamespace, modelProviderName)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL for model provider %q: %w", modelProviderName, err)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to model provider %q: %w", modelProviderName, err)
	}

	addCredHeaders(r, env)

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

func addCredHeaders(r *http.Request, credEnv map[string]string) {
	for k, v := range credEnv {
		r.Header.Set(fmt.Sprintf("X-Obot-%s", k), v)
	}
}
