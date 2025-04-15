package providers

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type ProviderMeta struct {
	types.CommonProviderMetadata
	EnvVars         []types.ProviderConfigurationParameter `json:"envVars"`
	OptionalEnvVars []types.ProviderConfigurationParameter `json:"optionalEnvVars"`
}

func ConvertModelProviderToolRef(toolRef v1.ToolReference, cred map[string]string) (*types.ModelProviderStatus, error) {
	commonProviderStatus, err := ConvertProviderTooRef(toolRef, cred)
	if err != nil {
		return nil, err
	}

	var modelsPopulated *bool
	if commonProviderStatus.Configured {
		modelsPopulated = new(bool)
		*modelsPopulated = toolRef.Status.ObservedGeneration == toolRef.Generation
	}

	return &types.ModelProviderStatus{
		CommonProviderStatus: *commonProviderStatus,
		ModelsBackPopulated:  modelsPopulated,
	}, nil
}
