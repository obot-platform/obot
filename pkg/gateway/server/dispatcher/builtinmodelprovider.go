package dispatcher

import (
	"context"

	openai "github.com/obot-platform/chat-completion-client"
)

// BuiltinModelProvider is a model provider that runs inside the Obot process rather than
// as a daemon launched from the provider registry.
type BuiltinModelProvider interface {
	// ListModels returns the models the provider can serve.
	ListModels(ctx context.Context) (*openai.ModelsList, error)
	// HasUpstreamProviders reports whether at least one upstream provider is configured,
	// which is what makes the built-in provider usable.
	HasUpstreamProviders(ctx context.Context) bool
	// Ready reports whether the provider has finished starting.
	Ready() bool
}

// RegisterBuiltinModelProvider registers a model provider that runs inside the Obot process.
func (d *Dispatcher) RegisterBuiltinModelProvider(namespace, modelProviderName string, p BuiltinModelProvider) {
	d.builtinLock.Lock()
	defer d.builtinLock.Unlock()
	if d.builtinModelProvider == nil {
		d.builtinModelProvider = map[string]BuiltinModelProvider{}
	}
	d.builtinModelProvider[providerKeyForModelProvider(namespace, modelProviderName)] = p
}

// BuiltinModelProvider returns the built-in model provider registered under the given name.
func (d *Dispatcher) BuiltinModelProvider(namespace, modelProviderName string) (BuiltinModelProvider, bool) {
	if d == nil {
		return nil, false
	}
	d.builtinLock.RLock()
	defer d.builtinLock.RUnlock()
	p, ok := d.builtinModelProvider[providerKeyForModelProvider(namespace, modelProviderName)]
	return p, ok
}
