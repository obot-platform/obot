package providers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func ModelProviderStatus(ctx context.Context, modelProvider v1.ModelProvider, cred map[string]string, licenseProvider *license.Provider) (*types.ModelProviderStatus, error) {
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

	missingEntitlements, err := licenseProvider.MissingEntitlements(ctx, modelProvider.Spec.RequiredEntitlements)
	if err != nil {
		return nil, err
	}

	return &types.ModelProviderStatus{
		CommonProviderStatus: types.CommonProviderStatus{
			Configured:                     len(missingEnvVars) == 0,
			MissingEntitlements:            missingEntitlements,
			MissingConfigurationParameters: missingEnvVars,
		},
		ModelsBackPopulated: modelsPopulated,
	}, nil
}
