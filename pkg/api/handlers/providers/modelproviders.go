package providers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func ModelProviderStatus(ctx context.Context, modelProvider v1.ModelProvider, cred map[string]string, entitlementProvider gclient.EntitlementProvider) (*types.ModelProviderStatus, error) {
	var (
		modelsPopulated *bool
		missingEnvVars  []string
	)

	if cred != nil {
		for _, envVar := range modelProvider.Spec.RequiredConfigurationParameters {
			if _, ok := cred[envVar.Name]; !ok {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	} else {
		missingEnvVars = modelProvider.Status.MissingConfigurationParameters
		if !modelProvider.Status.Configured && len(missingEnvVars) == 0 {
			for _, envVar := range modelProvider.Spec.RequiredConfigurationParameters {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	}

	if len(missingEnvVars) == 0 {
		modelsPopulated = new(modelProvider.Status.ObservedGeneration == modelProvider.Generation)
	}

	entitlementsMissing, err := license.EntitlementsMissing(ctx, entitlementProvider, modelProvider.Spec.RequiredEntitlements)
	if err != nil {
		return nil, err
	}

	return &types.ModelProviderStatus{
		Configured:                     len(missingEnvVars) == 0,
		EntitlementsMissing:            entitlementsMissing,
		MissingConfigurationParameters: missingEnvVars,
		ModelsBackPopulated:            modelsPopulated,
	}, nil
}
