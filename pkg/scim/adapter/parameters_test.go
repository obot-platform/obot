package adapter

import (
	"reflect"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
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
	conn := &types.SCIMConnection{
		AdapterType: "okta",
	}

	tests := []struct {
		name         string
		conn         *types.SCIMConnection
		stored       map[string]string
		wantRequired []string
		wantOptional []string
		wantDropped  []string
		// wantUnused means the optional directory parameters carry the adapter's unused description.
		wantUnused bool
	}{
		{
			name: "directory synchronization requires the directory parameters",
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
			name: "a connection whose credential still holds them makes them optional and unused",
			conn: conn,
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
			wantUnused: true,
		},
		{
			name: "a connection whose credential holds one of them still shows both",
			conn: conn,
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
			wantUnused: true,
		},
		{
			name: "a connection whose credential lacks them drops them",
			conn: conn,
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
			name: "a connection without a stored credential drops them",
			conn: conn,
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
			name: "a connection with an unknown adapter relaxes nothing",
			conn: &types.SCIMConnection{
				AdapterType: "unknown",
			},
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
			got := EffectiveParameters(manifest, tt.conn, tt.stored)

			if names := parameterNames(got.Required); !reflect.DeepEqual(names, tt.wantRequired) {
				t.Errorf("Required = %v, want %v", names, tt.wantRequired)
			}
			if names := parameterNames(got.Optional); !reflect.DeepEqual(names, tt.wantOptional) {
				t.Errorf("Optional = %v, want %v", names, tt.wantOptional)
			}
			if !reflect.DeepEqual(got.Dropped, tt.wantDropped) {
				t.Errorf("Dropped = %v, want %v", got.Dropped, tt.wantDropped)
			}

			for _, p := range got.Optional {
				for _, d := range directory {
					if p.Name != d.Name {
						continue
					}
					if tt.wantUnused && p.Description != d.UnusedDescription {
						t.Errorf("%s description = %q, want the unused description", p.Name, p.Description)
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
	params := EffectiveParameters(oktaManifest(), &types.SCIMConnection{AdapterType: "okta"}, stored)

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
	for _, conn := range []*types.SCIMConnection{nil, {AdapterType: "okta"}} {
		if got := EffectiveParameters(oktaManifest(), conn, nil).IncompleteGroup(map[string]string{
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
