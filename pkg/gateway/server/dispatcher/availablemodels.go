package dispatcher

import (
	"context"
	"fmt"

	openai "github.com/obot-platform/chat-completion-client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// ModelsForProvider lists the models a built-in model provider, the LLM gateway, serves.
func (d *Dispatcher) ModelsForProvider(ctx context.Context, modelProvider v1.ModelProvider) (*openai.ModelsList, error) {
	builtin, ok := d.BuiltinModelProvider(modelProvider.Namespace, modelProvider.Name)
	if !ok {
		return nil, fmt.Errorf("model provider %q was replaced by the LLM gateway", modelProvider.Name)
	}
	return builtin.ListModels(ctx)
}

// FreshModelsForProvider is ModelsForProvider; a built-in provider always lists its current
// models.
func (d *Dispatcher) FreshModelsForProvider(ctx context.Context, modelProvider v1.ModelProvider) (*openai.ModelsList, error) {
	return d.ModelsForProvider(ctx, modelProvider)
}
