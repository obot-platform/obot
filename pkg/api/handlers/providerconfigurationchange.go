package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	providerConfigurationChangeTimeout = 2 * time.Minute
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

	return waitForProviderConfigurationChange(req, change)
}

func waitForProviderConfigurationChange(req api.Context, change *v1.ProviderConfigurationChange) error {
	settled, err := wait.For(req.Context(), req.Storage, change, func(current *v1.ProviderConfigurationChange) (bool, error) {
		return current.Status.Applied || current.Status.Error != "", nil
	}, wait.Option{Timeout: providerConfigurationChangeTimeout})
	if apierrors.IsNotFound(err) {
		// The controller applied the change and cleaned it up before we observed
		// its status, so there is nothing left to report.
		return nil
	} else if err != nil {
		return fmt.Errorf("wait for provider configuration change %q: %w", change.Name, err)
	}

	if settled.Status.Error != "" {
		return types.NewErrBadRequest("%s", settled.Status.Error)
	}
	return nil
}
