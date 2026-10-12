package setup

import (
	"errors"
	"maps"
	"strings"
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	issuerParam   = "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
	clientIDParam = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"
	keyParam      = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"
)

func TestCheckConfiguration(t *testing.T) {
	okta := v1.AuthProvider{
		Name:      "okta-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Name: "Okta",
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: issuerParam,
						},
						{
							Name: clientIDParam,
						},
						{
							Name: keyParam,
						},
					},
				},
				GroupIDPrefix: "okta/",
			},
		},
	}
	withoutRules := v1.AuthProvider{
		Name:      "github-auth-provider",
		Namespace: system.DefaultNamespace,
	}
	directory := map[string]string{
		issuerParam:   "https://example.okta.com",
		clientIDParam: "client",
		keyParam:      "key",
	}
	oidcOnly := map[string]string{
		issuerParam: "https://example.okta.com",
	}
	partial := map[string]string{
		issuerParam:   "https://example.okta.com",
		clientIDParam: "client",
	}

	tests := []struct {
		name         string
		authProvider v1.AuthProvider
		// connection is the origin of the provider's SCIM connection, or empty for none.
		connection types.SCIMConnectionOrigin
		configured bool
		// stored is the provider's stored configuration. A nil one must not be read.
		stored      map[string]string
		secrets     map[string]string
		wantSetup   Setup
		wantSecrets map[string]string
		// wantRefusal is part of the refusal, if the configuration is refused.
		wantRefusal string
	}{
		{
			name:         "a provider without SCIM rules",
			authProvider: withoutRules,
			secrets:      oidcOnly,
			wantSetup:    SetupKept,
			wantSecrets:  oidcOnly,
		},
		{
			name:         "a provider that is not configured, without directory parameters",
			authProvider: okta,
			secrets:      oidcOnly,
			wantSetup:    SetupSCIMFirst,
			wantSecrets:  oidcOnly,
		},
		{
			name:         "a provider that is not configured, with directory parameters",
			authProvider: okta,
			secrets:      directory,
			wantSetup:    SetupKept,
			wantSecrets:  directory,
		},
		{
			name:         "a provider that is not configured, with some directory parameters",
			authProvider: okta,
			secrets:      partial,
			wantRefusal:  "provide all of " + clientIDParam + " and " + keyParam + ", or none of them; missing: " + keyParam,
		},
		{
			name:         "a configured provider that synchronizes its directory, with directory parameters",
			authProvider: okta,
			configured:   true,
			secrets:      directory,
			wantSetup:    SetupKept,
			wantSecrets:  directory,
		},
		{
			name:         "a configured provider that synchronizes its directory, without directory parameters",
			authProvider: okta,
			configured:   true,
			secrets:      partial,
			wantRefusal:  "Okta synchronizes its directory at sign-in, so it requires " + keyParam,
		},
		{
			name:         "a provider whose connection was created without directory credentials",
			authProvider: okta,
			connection:   types.SCIMConnectionOriginSCIMFirst,
			secrets:      directory,
			wantSetup:    SetupKept,
			wantSecrets:  oidcOnly,
		},
		{
			name:         "a provider whose connection replaced directory synchronization, which still stores them",
			authProvider: okta,
			connection:   types.SCIMConnectionOriginMigrated,
			stored:       directory,
			secrets:      directory,
			wantSetup:    SetupKept,
			wantSecrets:  directory,
		},
		{
			name:         "a provider whose connection replaced directory synchronization, configured with some of them",
			authProvider: okta,
			connection:   types.SCIMConnectionOriginMigrated,
			stored:       directory,
			secrets:      partial,
			wantRefusal:  "provide all of",
		},
		{
			name:         "a provider whose connection replaced directory synchronization, which no longer stores them",
			authProvider: okta,
			connection:   types.SCIMConnectionOriginMigrated,
			stored:       oidcOnly,
			secrets:      directory,
			wantSetup:    SetupKept,
			wantSecrets:  oidcOnly,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newTestGateway(t)
			if tt.connection != "" {
				if _, _, err := gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
					AuthProviderNamespace: tt.authProvider.Namespace,
					AuthProviderName:      tt.authProvider.Name,
					GroupIDPrefix:         "okta/",
					Origin:                tt.connection,
				}); err != nil {
					t.Fatal(err)
				}
			}
			configured := func() (bool, error) {
				if tt.connection != "" {
					t.Error("asked whether a provider with a connection is configured")
				}
				return tt.configured, nil
			}
			stored := func() (map[string]string, error) {
				if tt.stored == nil {
					t.Error("read a stored configuration that does not matter")
				}
				return tt.stored, nil
			}
			secrets := maps.Clone(tt.secrets)

			_, setup, err := CheckConfiguration(t.Context(), gateway, tt.authProvider, configured, stored, secrets)
			if tt.wantRefusal != "" {
				refused, ok := errors.AsType[*ConfigurationError](err)
				if !ok || !strings.Contains(refused.Message, tt.wantRefusal) {
					t.Fatalf("CheckConfiguration() error = %v, want a refusal containing %q", err, tt.wantRefusal)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckConfiguration() error = %v", err)
			}
			if setup != tt.wantSetup {
				t.Errorf("setup = %v, want %v", setup, tt.wantSetup)
			}
			if !maps.Equal(secrets, tt.wantSecrets) {
				t.Errorf("secrets = %v, want %v", secrets, tt.wantSecrets)
			}
		})
	}
}
