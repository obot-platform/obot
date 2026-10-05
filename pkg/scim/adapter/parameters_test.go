package adapter

import (
	"reflect"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestRegistry(t *testing.T) {
	a, ok := ForAuthProvider("okta-auth-provider")
	if !ok || a.Type() != "okta" {
		t.Fatalf("ForAuthProvider(okta-auth-provider) = %v, %v", a, ok)
	}
	if byType, ok := Lookup(a.Type()); !ok || byType.Type() != a.Type() {
		t.Fatalf("Lookup(%q) = %v, %v", a.Type(), byType, ok)
	}

	// Only the registry decides which providers support SCIM. A name or a prefix that merely looks like Okta's does
	// not.
	for _, name := range []string{"okta", "okta-auth-provider-2", "entra-auth-provider", ""} {
		if _, ok := ForAuthProvider(name); ok {
			t.Errorf("ForAuthProvider(%q) found an adapter", name)
		}
	}
	if _, ok := Lookup("okta-auth-provider"); ok {
		t.Error("Lookup found an adapter by auth provider name")
	}

	manifest := oktaManifest()
	if !SupportsSCIM("okta-auth-provider", manifest) {
		t.Error("the Okta provider does not support SCIM")
	}
	manifest.GroupIDPrefix = ""
	if SupportsSCIM("okta-auth-provider", manifest) {
		t.Error("a provider without a group ID prefix supports SCIM")
	}
}

func TestEffectiveParameters(t *testing.T) {
	a, _ := ForAuthProvider("okta-auth-provider")
	directory := a.DirectoryParameters()
	tests := []struct {
		name             string
		authProviderName string
		configured       bool
		// connAdapterType is the adapter type of the provider's SCIM connection, or empty when it has none.
		connAdapterType string
		stored          map[string]string
		wantRequired    []string
		wantOptional    []string
		wantDropped     []string
		wantTogether    []string
		// wantDescription selects the adapter's description of the optional directory parameters: "setup" or
		// "unused".
		wantDescription string
	}{
		{
			name:             "a provider being set up may omit the directory parameters",
			authProviderName: "okta-auth-provider",
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantTogether: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantDescription: "setup",
		},
		{
			name:             "a provider being set up without an adapter requires everything",
			authProviderName: "okta-auth-provider-2",
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
		},
		{
			name:             "directory synchronization requires the directory parameters",
			authProviderName: "okta-auth-provider",
			configured:       true,
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
		},
		{
			name:             "a connection whose credential still holds them makes them optional and unused",
			authProviderName: "okta-auth-provider",
			configured:       true,
			connAdapterType:  "okta",
			stored: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID":   "client",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY": "key",
			},
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantTogether: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantDescription: "unused",
		},
		{
			name:             "a connection whose credential holds one of them still shows both",
			authProviderName: "okta-auth-provider",
			connAdapterType:  "okta",
			stored: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID": "client",
			},
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantTogether: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantDescription: "unused",
		},
		{
			name:             "a connection whose credential lacks them drops them",
			authProviderName: "okta-auth-provider",
			configured:       true,
			connAdapterType:  "okta",
			stored: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID": "client",
			},
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
			wantDropped: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
		},
		{
			name:             "a connection without a stored credential drops them, even while being set up",
			authProviderName: "okta-auth-provider",
			connAdapterType:  "okta",
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
			wantDropped: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
		},
		{
			name:             "a connection with an unknown adapter relaxes nothing",
			authProviderName: "okta-auth-provider",
			connAdapterType:  "unknown",
			wantRequired: []string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
			wantOptional: []string{
				"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := oktaManifest()
			got := EffectiveParameters(manifest, ProviderState{
				AuthProviderName:      tt.authProviderName,
				Configured:            tt.configured,
				ConnectionAdapterType: tt.connAdapterType,
				Stored:                tt.stored,
			})

			if names := parameterNames(got.Required); !reflect.DeepEqual(names, tt.wantRequired) {
				t.Errorf("Required = %v, want %v", names, tt.wantRequired)
			}
			if names := parameterNames(got.Optional); !reflect.DeepEqual(names, tt.wantOptional) {
				t.Errorf("Optional = %v, want %v", names, tt.wantOptional)
			}
			if !reflect.DeepEqual(got.Dropped, tt.wantDropped) {
				t.Errorf("Dropped = %v, want %v", got.Dropped, tt.wantDropped)
			}
			if !reflect.DeepEqual(got.Together, tt.wantTogether) {
				t.Errorf("Together = %v, want %v", got.Together, tt.wantTogether)
			}

			for _, p := range got.Optional {
				for _, d := range directory {
					if p.Name != d.Name {
						continue
					}
					want := d.SetupDescription
					if tt.wantDescription == "unused" {
						want = d.UnusedDescription
					}
					if p.Description != want {
						t.Errorf("%s description = %q, want the %s description", p.Name, p.Description, tt.wantDescription)
					}
					// The rest of the manifest's definition is kept.
					if p.FriendlyName == "" {
						t.Errorf("%s lost its friendly name", p.Name)
					}
				}
			}

			// The manifest itself is never changed.
			if !reflect.DeepEqual(manifest, oktaManifest()) {
				t.Error("EffectiveParameters changed the manifest")
			}
		})
	}
}

func TestIncompleteGroup(t *testing.T) {
	stored := map[string]string{
		"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID":   "client",
		"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY": "key",
	}
	params := EffectiveParameters(oktaManifest(), ProviderState{
		AuthProviderName:      "okta-auth-provider",
		Configured:            true,
		ConnectionAdapterType: "okta",
		Stored:                stored,
	})

	tests := []struct {
		name   string
		config map[string]string
		want   []string
	}{
		{
			name:   "both",
			config: stored,
		},
		{
			name:   "neither",
			config: map[string]string{},
		},
		{
			name: "only the client ID",
			config: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID": "client",
			},
			want: []string{"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"},
		},
		{
			name: "the private key with an empty client ID",
			config: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID":   "",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY": "key",
			},
			want: []string{"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := params.IncompleteGroup(tt.config); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("IncompleteGroup() = %v, want %v", got, tt.want)
			}
		})
	}

	// Parameters that are required, or dropped, are never checked together.
	for _, connAdapterType := range []string{"", "okta"} {
		if got := EffectiveParameters(oktaManifest(), ProviderState{
			AuthProviderName:      "okta-auth-provider",
			Configured:            true,
			ConnectionAdapterType: connAdapterType,
		}).IncompleteGroup(map[string]string{
			"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID": "client",
		}); got != nil {
			t.Fatalf("IncompleteGroup() without optional directory parameters = %v", got)
		}
	}
}

func parameterNames(params []types2.ProviderConfigurationParameter) []string {
	var names []string
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

// oktaManifest returns a manifest like the Okta auth provider's, trimmed to what the tests need.
func oktaManifest() types2.AuthProviderManifest {
	return types2.AuthProviderManifest{
		Name: "Okta",
		RequiredConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID",
				FriendlyName: "Client ID",
			},
			{
				Name:         "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
				FriendlyName: "Org URL",
			},
			{
				Name:         "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				FriendlyName: "API Services Client ID",
				Description:  "Client ID for the Okta API Services app.",
			},
			{
				Name:         "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
				FriendlyName: "API Services Private Key",
				Description:  "PEM-encoded RSA private key for your Okta API Services app.",
				Sensitive:    true,
				Multiline:    true,
			},
		},
		OptionalConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				FriendlyName: "Token Refresh Duration",
			},
		},
		GroupIDPrefix: "okta/",
	}
}
