package providers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func AuthProviderStatus(ctx context.Context, authProvider v1.AuthProvider, cred map[string]string, licenseProvider *license.Provider) (*types.AuthProviderStatus, error) {
	var missingEnvVars []string

	if cred != nil {
		for _, envVar := range authProvider.Spec.RequiredConfigurationParameters {
			if _, ok := cred[envVar.Name]; !ok {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	} else {
		missingEnvVars = authProvider.Status.MissingConfigurationParameters
		if !authProvider.Status.Configured && len(missingEnvVars) == 0 {
			for _, envVar := range authProvider.Spec.RequiredConfigurationParameters {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	}

	missingEntitlements, err := licenseProvider.MissingEntitlements(ctx, authProvider.Spec.RequiredEntitlements)
	if err != nil {
		return nil, err
	}

	return &types.AuthProviderStatus{
		CommonProviderStatus: types.CommonProviderStatus{
			Configured:                     len(missingEnvVars) == 0,
			MissingEntitlements:            missingEntitlements,
			MissingConfigurationParameters: missingEnvVars,
		},
		Namespace: authProvider.Namespace,
	}, nil
}
