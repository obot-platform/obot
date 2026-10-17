// Package modelinfosource removes the models.dev model info source that earlier versions
// synced model prices from. Gateway models get their prices from the LLM gateway's model
// catalog instead, and the source only ever priced the model providers the gateway replaced.
package modelinfosource

import (
	"context"
	"fmt"
	"log/slog"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteDefaultSource deletes the default model info source left by an earlier version, if
// there is one. The model info it synced is deleted with it.
func DeleteDefaultSource(ctx context.Context, c kclient.Client) error {
	err := c.Delete(ctx, &v1.ModelInfoSource{Name: system.DefaultModelInfoSource, Namespace: system.DefaultNamespace})
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete the model info source: %w", err)
	}
	slog.Info("Deleted the models.dev model info source; gateway models are priced by the LLM gateway")
	return nil
}
