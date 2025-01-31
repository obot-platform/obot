package providers

import (
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type ProviderMeta struct {
	types.CommonProviderMetadata
	EnvVars         []types.ProviderConfigurationParameter `json:"envVars"`
	OptionalEnvVars []types.ProviderConfigurationParameter `json:"optionalEnvVars"`
}

func ConvertModelProviderToolRef(toolRef v1.ToolReference, cred map[string]string) (*types.ModelProviderStatus, error) {
	var (
		providerMeta   ProviderMeta
		missingEnvVars []string
	)
	if toolRef.Status.Tool != nil {
		if toolRef.Status.Tool.Metadata["providerMeta"] != "" {
			if err := json.Unmarshal([]byte(toolRef.Status.Tool.Metadata["providerMeta"]), &providerMeta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider meta for %s: %v", toolRef.Name, err)
			}
		}

		for _, envVar := range providerMeta.EnvVars {
			if _, ok := cred[envVar.Name]; !ok {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	}

	var modelsPopulated *bool
	configured := toolRef.Status.Tool != nil && len(missingEnvVars) == 0
	if configured {
		modelsPopulated = new(bool)
		*modelsPopulated = toolRef.Status.ObservedGeneration == toolRef.Generation
	}

	return &types.ModelProviderStatus{
		CommonProviderMetadata:          providerMeta.CommonProviderMetadata,
		Configured:                      configured,
		ModelsBackPopulated:             modelsPopulated,
		RequiredConfigurationParameters: providerMeta.EnvVars,
		OptionalConfigurationParameters: providerMeta.OptionalEnvVars,
		MissingConfigurationParameters:  missingEnvVars,
	}, nil
}
