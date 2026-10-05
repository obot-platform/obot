package providers

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// AuthProviderStatus reports whether the auth provider's stored configuration is complete, checking its effective
// required parameters, which depend on its SCIM connection. conn is the provider's SCIM connection, or nil when it has
// none. When cred is nil, the status the controller computed is reported instead of checking a credential.
func AuthProviderStatus(ctx context.Context, authProvider v1.AuthProvider, cred map[string]string, conn *gatewaytypes.SCIMConnection, licenseProvider *license.Provider) (*types.AuthProviderStatus, error) {
	var missingEnvVars []string

	// A stored configuration is the provider's active one, so without a connection it synchronizes its directory.
	required := adapter.EffectiveParameters(authProvider.Spec.AuthProviderManifest, adapter.ProviderState{
		AuthProviderName:      authProvider.Name,
		Configured:            true,
		ConnectionAdapterType: adapter.ConnectionAdapterType(conn),
	}).Required
	if cred != nil {
		for _, envVar := range required {
			if _, ok := cred[envVar.Name]; !ok {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	} else {
		missingEnvVars = authProvider.Status.MissingConfigurationParameters
		if !authProvider.Status.Configured && len(missingEnvVars) == 0 {
			for _, envVar := range required {
				missingEnvVars = append(missingEnvVars, envVar.Name)
			}
		}
	}

	missingEntitlements, err := licenseProvider.MissingEntitlements(ctx, authProvider.Spec.RequiredEntitlements)
	if err != nil {
		return nil, err
	}

	return &types.AuthProviderStatus{
		Configured:                     len(missingEnvVars) == 0,
		MissingEntitlements:            missingEntitlements,
		MissingConfigurationParameters: missingEnvVars,
		Namespace:                      authProvider.Namespace,
	}, nil
}
