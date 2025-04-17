package providers

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	CookieSecretEnvVar       = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"
	PostgresConnectionEnvVar = "OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"
)

func ConvertAuthProviderToolRef(toolRef v1.ToolReference, cred map[string]string) (*types.AuthProviderStatus, error) {
	providerStatus, err := ConvertProviderToolRef(toolRef, cred)
	if err != nil {
		return nil, err
	}

	return &types.AuthProviderStatus{CommonProviderStatus: *providerStatus}, nil
}
