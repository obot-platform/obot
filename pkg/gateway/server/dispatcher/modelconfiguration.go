package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HasModelProvider reads installation-wide configuration without starting a
// provider daemon or considering model access, licenses, or upstream health.
// Only a confirmed absence permits the Tester's external fallback.
func (d *Dispatcher) HasModelProvider(ctx context.Context) (bool, error) {
	return hasModelProvider(ctx, d.client, func(ctx context.Context, provider v1.ModelProvider) (map[string]string, error) {
		env, err := CredentialEnvForModelProvider(ctx, d.gatewayClient, provider)
		if errors.As(err, &client.CredentialNotFoundError{}) {
			return map[string]string{}, nil
		}

		return env, err
	})
}

func hasModelProvider(ctx context.Context, storage kclient.Client, credentials func(context.Context, v1.ModelProvider) (map[string]string, error)) (bool, error) {
	checkPending := func() error {
		var changes v1.ProviderConfigurationChangeList
		if err := storage.List(ctx, &changes, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return fmt.Errorf("read provider configuration changes: %w", err)
		}

		for _, change := range changes.Items {
			if change.Spec.ProviderType == v1.ProviderTypeModel && !change.Status.Applied && change.Status.Error == "" {
				return fmt.Errorf("model provider configuration change is pending")
			}
		}

		return nil
	}

	if err := checkPending(); err != nil {
		return false, err
	}

	var providers v1.ModelProviderList
	if err := storage.List(ctx, &providers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return false, fmt.Errorf("read model providers: %w", err)
	}

	var configured bool
	for _, provider := range providers.Items {
		if !provider.DeletionTimestamp.IsZero() {
			return false, fmt.Errorf("model provider deletion is pending")
		}

		// A provider with no required parameters is configured even without a
		// credential record or a controller status update.
		if len(provider.Spec.RequiredConfigurationParameters) == 0 {
			configured = true
			continue
		}

		env, err := credentials(ctx, provider)
		if err != nil {
			return false, fmt.Errorf("read model provider configuration: %w", err)
		}

		var missing []string
		for _, parameter := range provider.Spec.RequiredConfigurationParameters {
			// Match the existing computed status: presence, including an empty
			// value, satisfies a required parameter.
			if _, ok := env[parameter.Name]; !ok {
				missing = append(missing, parameter.Name)
			}
		}

		if len(missing) == 0 {
			configured = true
			continue
		}

		// Partial credentials, stale configured status, or unobserved manifests
		// are not evidence that the installation has no configured provider.
		expectedMissing := slices.Clone(provider.Status.MissingConfigurationParameters)
		slices.Sort(expectedMissing)
		slices.Sort(missing)
		if len(env) != 0 || provider.Status.Configured || provider.Status.Error != "" ||
			provider.Status.ObservedGeneration != provider.Generation || !slices.Equal(missing, expectedMissing) {
			return false, fmt.Errorf("model provider configuration is incomplete or inconsistent")
		}
	}

	// A change may have begun while credentials were being read.
	if err := checkPending(); err != nil {
		return false, err
	}

	return configured, nil
}
