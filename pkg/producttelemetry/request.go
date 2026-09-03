package producttelemetry

import (
	"context"
	"fmt"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/upgrade"
	"github.com/obot-platform/obot/pkg/version"
)

type installationPropertyClient interface {
	GetOrCreateProperty(context.Context, string, string) (gatewaytypes.Property, error)
}

// buildRequest currently populates the stable installation ID and Obot version.
// Additional report fields will be added incrementally.
func buildRequest(ctx context.Context, gatewayClient installationPropertyClient) (clienttypes.ProductTelemetryRequest, error) {
	installationID, err := upgrade.GetInstallationID(ctx, gatewayClient)
	if err != nil {
		return clienttypes.ProductTelemetryRequest{}, fmt.Errorf("get installation ID: %w", err)
	}

	return clienttypes.ProductTelemetryRequest{
		InstallationID: installationID,
		CurrentVersion: version.Get().String(),
	}, nil
}
