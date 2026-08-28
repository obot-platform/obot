package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"uuid"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func modelProviderConfigurationChangeName(providerName string) string {
	return name.SafeConcatName(system.ProviderChangePrefix, providerName)
}

func stageProviderCredential(req api.Context, secrets map[string]string) (string, error) {
	stagedName := strings.ReplaceAll(uuid.NewV4().String(), "-", "")
	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: system.StagedProviderCredentialContext,
		Name:    stagedName,
		Secrets: secrets,
	}); err != nil {
		return "", fmt.Errorf("stage provider credential: %w", err)
	}
	return stagedName, nil
}

func submitProviderConfigurationChange(req api.Context, change *v1.ProviderConfigurationChange) error {
	if err := req.Create(change); err != nil {
		var cleanupErr error
		if change.Spec.StagedCredentialName != "" {
			_, cleanupErr = req.GatewayClient.DeleteCredential(
				context.WithoutCancel(req.Context()),
				system.StagedProviderCredentialContext,
				change.Spec.StagedCredentialName,
			)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("remove staged provider credential %q: %w", change.Spec.StagedCredentialName, cleanupErr)
			}
		}
		if apierrors.IsAlreadyExists(err) {
			return errors.Join(types.NewErrHTTP(http.StatusConflict, "another provider configuration change is already in progress"), cleanupErr)
		}
		return errors.Join(fmt.Errorf("create provider configuration change: %w", err), cleanupErr)
	}

	if err := waitForProviderConfigurationChangeDeletion(req.Context(), req.Storage, change); err != nil {
		return fmt.Errorf("wait for provider configuration change %q: %w", change.Name, err)
	}
	return nil
}

// waitForProviderConfigurationChangeDeletion deliberately has no internal
// timeout. The persisted change outlives an HTTP request, while this wait ends
// only when that request is canceled or the exact submitted object is deleted.
func waitForProviderConfigurationChangeDeletion(ctx context.Context, client kclient.WithWatch, change *v1.ProviderConfigurationChange) error {
	for {
		var current v1.ProviderConfigurationChange
		if err := client.Get(ctx, kclient.ObjectKeyFromObject(change), &current); apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}

		watcher, err := client.Watch(ctx, &v1.ProviderConfigurationChangeList{},
			kclient.InNamespace(change.Namespace),
			kclient.MatchingFields{"metadata.name": change.Name},
			&kclient.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: current.ResourceVersion}},
		)
		if err != nil {
			return err
		}

		deleted, err := waitForExactProviderConfigurationChangeDeletion(ctx, watcher, change)
		watcher.Stop()
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}
		// A watch can close during storage reconnects. Re-read and resume it.
	}
}

func waitForExactProviderConfigurationChangeDeletion(ctx context.Context, watcher watch.Interface, change *v1.ProviderConfigurationChange) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return false, context.Cause(ctx)
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return false, nil
			}
			switch event.Type {
			case watch.Deleted:
				deleted, ok := event.Object.(*v1.ProviderConfigurationChange)
				if ok && deleted.Namespace == change.Namespace && deleted.Name == change.Name &&
					(change.UID == "" || deleted.UID == change.UID) {
					return true, nil
				}
			case watch.Error:
				return false, apierrors.FromObject(event.Object)
			}
		}
	}
}
