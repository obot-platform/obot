package upgrade

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	DefaultServerBaseURL      = "https://upgrade-server.obot.ai"
	InstallationIDPropertyKey = "installation_id"
)

type propertyClient interface {
	GetOrCreateProperty(context.Context, string, string) (gatewaytypes.Property, error)
}

func ServerBaseURL() string {
	if baseURL := strings.TrimSpace(os.Getenv("OBOT_UPGRADE_SERVER_URL")); baseURL != "" {
		return baseURL
	}
	return DefaultServerBaseURL
}

func EndpointURL(baseURL, endpoint string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
}

func GetInstallationID(ctx context.Context, gatewayClient propertyClient) (string, error) {
	if gatewayClient == nil {
		return "", fmt.Errorf("gateway client is required")
	}

	property, err := gatewayClient.GetOrCreateProperty(ctx, InstallationIDPropertyKey, uuid.NewString())
	if err != nil {
		return "", fmt.Errorf("failed to ensure installation ID: %w", err)
	}
	return property.Value, nil
}
