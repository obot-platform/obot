package dispatcher

import (
	"context"

	"github.com/obot-platform/obot/pkg/system"
)

// HasModelProvider reports whether models can be served: whether the LLM gateway has an
// upstream provider configured.
func (d *Dispatcher) HasModelProvider(ctx context.Context) (bool, error) {
	builtin, ok := d.BuiltinModelProvider(system.DefaultNamespace, system.LLMGatewayModelProvider)
	if !ok {
		return false, nil
	}
	return builtin.HasUpstreamProviders(ctx), nil
}
