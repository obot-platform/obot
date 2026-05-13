package providers

import (
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func ConvertProviderToolRef(toolRef v1.ToolReference, cred map[string]string, licenseProvider *license.KeygenProvider) (*types.CommonProviderStatus, error) {
	var (
		providerMeta        ProviderMeta
		missingEnvVars      []string
		missingEntitlements []string
	)
	if toolRef.Status.Tool != nil {
		if toolRef.Status.Tool.Metadata["providerMeta"] != "" {
			if err := json.Unmarshal([]byte(toolRef.Status.Tool.Metadata["providerMeta"]), &providerMeta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider meta for %s: %v", toolRef.Name, err)
			}
		}

		if cred != nil {
			for _, envVar := range providerMeta.EnvVars {
				if _, ok := cred[envVar.Name]; !ok {
					missingEnvVars = append(missingEnvVars, envVar.Name)
				}
			}
		} else if !toolRef.Status.Configured {
			missingEnvVars = toolRef.Status.MissingConfigurationParameters
			if len(missingEnvVars) == 0 {
				for _, envVar := range providerMeta.EnvVars {
					missingEnvVars = append(missingEnvVars, envVar.Name)
				}
			}
		}

		missingEntitlements = licenseProvider.Missing(providerMeta.RequiredEntitlements)
	}

	configured := toolRef.Status.Tool != nil && toolRef.Status.Configured
	if cred != nil {
		configured = toolRef.Status.Tool != nil && len(missingEnvVars) == 0
	}

	return &types.CommonProviderStatus{
		CommonProviderMetadata:          providerMeta.CommonProviderMetadata,
		Configured:                      configured,
		RequiredConfigurationParameters: providerMeta.EnvVars,
		OptionalConfigurationParameters: providerMeta.OptionalEnvVars,
		MissingConfigurationParameters:  missingEnvVars,
		MissingEntitlements:             missingEntitlements,
		Error:                           toolRef.Status.Error,
	}, nil
}
