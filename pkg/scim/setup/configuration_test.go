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

	entraTenantParam = "OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID"
	entraChoiceParam = "OBOT_ENTRA_AUTH_PROVIDER_SCIM"
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
	// Entra has no directory parameters. Its choice parameter chooses SCIM-first setup, and is never stored.
	entra := v1.AuthProvider{
		Name:      "entra-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Name: "Microsoft Entra",
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: entraTenantParam,
						},
					},
				},
				GroupIDPrefix: "entra/",
			},
		},
	}
	entraConfig := map[string]string{
		entraTenantParam: "00000000-0000-4000-8000-000000000001",
	}
	entraSCIM := map[string]string{
		entraTenantParam: "00000000-0000-4000-8000-000000000001",
		entraChoiceParam: "true",
	}
	entraNotSCIM := map[string]string{
		entraTenantParam: "00000000-0000-4000-8000-000000000001",
		entraChoiceParam: "false",
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
		{
			name:         "an Entra provider that is not configured, choosing SCIM",
			authProvider: entra,
			secrets:      entraSCIM,
			wantSetup:    SetupSCIMFirst,
			wantSecrets:  entraConfig,
		},
		{
			name:         "an Entra provider that is not configured, without the choice, which synchronizes its directory",
			authProvider: entra,
			secrets:      entraConfig,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
		{
			name:         "an Entra provider that is not configured, with a choice other than true",
			authProvider: entra,
			secrets:      entraNotSCIM,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
		{
			name:         "a configured Entra provider that synchronizes its directory, which has no parameters to require",
			authProvider: entra,
			configured:   true,
			secrets:      entraConfig,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
		{
			name:         "a configured Entra provider that synchronizes its directory cannot choose SCIM by configuration",
			authProvider: entra,
			configured:   true,
			secrets:      entraSCIM,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
		{
			name:         "an Entra provider with a connection",
			authProvider: entra,
			connection:   types.SCIMConnectionOriginSCIMFirst,
			secrets:      entraSCIM,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
		{
			name:         "an Entra provider whose connection replaced directory synchronization",
			authProvider: entra,
			connection:   types.SCIMConnectionOriginMigrated,
			stored:       entraConfig,
			secrets:      entraSCIM,
			wantSetup:    SetupKept,
			wantSecrets:  entraConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newTestGateway(t)
			if tt.connection != "" {
				// A connection that matches users by group lookup ID waits for the backfill of their hashes.
				if err := gateway.BackfillIdentityGroupLookupIDHashes(t.Context()); err != nil {
					t.Fatal(err)
				}
				if _, _, err := gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
					AuthProviderNamespace: tt.authProvider.Namespace,
					AuthProviderName:      tt.authProvider.Name,
					GroupIDPrefix:         tt.authProvider.Spec.GroupIDPrefix,
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
