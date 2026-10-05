package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	// SetupKept means the configuration keeps how the provider gets its users and groups: through its SCIM
	// connection, by synchronizing its directory at sign-in, or, for a provider without SCIM rules, as always.
	SetupKept Setup = iota
	// SetupSCIMFirst means the configuration omits the directory parameters of a provider that is not configured and
	// has no SCIM connection, so the connection must be created before the provider's credential exists.
	SetupSCIMFirst
)

// Setup is what a configuration submitted for an auth provider sets up.
type Setup int

// ConfigurationError reports a configuration that the SCIM rules of its auth provider refuse.
type ConfigurationError struct {
	Message string
}

func (e *ConfigurationError) Error() string {
	return e.Message
}

// CheckConfiguration applies the SCIM rules of an auth provider to a configuration submitted for it, and returns the
// provider's effective parameters and what the configuration sets up. Both the API, which validates a configuration
// before submitting it, and the provider configuration change, which applies it, check it here, so that they agree.
//
// It first drops from secrets the parameters that the provider no longer uses, so that a provider whose directory is
// managed through SCIM is configured again without them. It then refuses, with a *ConfigurationError:
//   - some but not all of the parameters that the provider needs together;
//   - a configuration of a configured provider that synchronizes its directory, without the directory parameters.
//     Moving it to SCIM is Enable and Enforce.
//
// It does not check the provider's other required parameters, which the API checks against the parameters it returns.
func CheckConfiguration(ctx context.Context, gateway *gclient.Client, authProvider v1.AuthProvider, configured func() (bool, error), stored func() (map[string]string, error), secrets map[string]string) (adapter.Parameters, Setup, error) {
	conn, err := gateway.SCIMConnectionForAuthProvider(ctx, authProvider.Namespace, authProvider.Name)
	if err != nil {
		return adapter.Parameters{}, SetupKept, err
	}

	a, ok := adapter.ForAuthProvider(authProvider.Name)
	hasRules := ok && adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest)
	state := adapter.ProviderState{
		AuthProviderName:      authProvider.Name,
		ConnectionAdapterType: adapter.ConnectionAdapterType(conn),
	}
	if conn != nil {
		if conn.Origin == types.SCIMConnectionOriginMigrated {
			if state.Stored, err = stored(); err != nil {
				return adapter.Parameters{}, SetupKept, err
			}
		}
	} else if hasRules {
		if state.Configured, err = configured(); err != nil {
			return adapter.Parameters{}, SetupKept, err
		}
	}

	params := adapter.EffectiveParameters(authProvider.Spec.AuthProviderManifest, state)
	for _, name := range params.Dropped {
		delete(secrets, name)
	}
	if missing := params.IncompleteGroup(secrets); len(missing) > 0 {
		return params, SetupKept, &ConfigurationError{
			Message: fmt.Sprintf("provide all of %s, or none of them; missing: %s", strings.Join(params.Together, " and "), strings.Join(missing, ", ")),
		}
	}
	if !hasRules || conn != nil {
		return params, SetupKept, nil
	}

	if state.Configured {
		var missing []string
		for _, d := range a.DirectoryParameters() {
			if secrets[d.Name] == "" {
				missing = append(missing, d.Name)
			}
		}
		if len(missing) > 0 {
			return params, SetupKept, &ConfigurationError{
				Message: fmt.Sprintf("%s synchronizes its directory at sign-in, so it requires %s. Moving it to SCIM provisioning is a separate, reviewed step",
					cmp.Or(authProvider.Spec.Name, authProvider.Name), strings.Join(missing, ", ")),
			}
		}
		return params, SetupKept, nil
	}
	if adapter.DirectoryParametersProvided(a, secrets) {
		return params, SetupKept, nil
	}
	return params, SetupSCIMFirst, nil
}

// StoredConfiguration returns the configuration that an auth provider has stored, active or staged, or nil when it has
// none.
func StoredConfiguration(ctx context.Context, gateway *gclient.Client, authProvider v1.AuthProvider) (map[string]string, error) {
	cred, err := gateway.RevealCredential(ctx, []string{
		authProvider.Name,
		system.GenericAuthProviderCredentialContext,
		system.ReplacementAuthProviderCredentialContext,
	}, authProvider.Name)
	if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return nil, fmt.Errorf("failed to reveal credential for auth provider %q: %w", authProvider.Name, err)
	}
	return cred.Secrets, nil
}
