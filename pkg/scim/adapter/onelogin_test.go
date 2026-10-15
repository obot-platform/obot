package adapter

import (
	"errors"
	"slices"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestOneLoginRegistry(t *testing.T) {
	a, ok := ForAuthProvider("onelogin-auth-provider")
	if !ok || a.Type() != "onelogin" {
		t.Fatalf("ForAuthProvider(onelogin-auth-provider) = %v, %v", a, ok)
	}
	if byType, ok := Lookup("onelogin"); !ok || byType.Type() != "onelogin" {
		t.Fatalf("Lookup(onelogin) = %v, %v", byType, ok)
	}
	if !SupportsSCIM("onelogin-auth-provider", oneLoginManifest()) {
		t.Error("the OneLogin provider does not support SCIM")
	}
}

func TestOneLoginNativeUserID(t *testing.T) {
	tests := []struct {
		name       string
		userName   string
		externalID string
		want       string
		// wantError is a fragment of the refusal's message, which names the OneLogin setting to change.
		wantError string
	}{
		{
			name:       "numeric user ID",
			userName:   "263801284",
			externalID: "263801284",
			want:       "263801284",
		},
		{
			name:       "surrounding whitespace",
			userName:   " 263801284",
			externalID: "263801284 ",
			want:       "263801284",
		},
		{
			name:       "the longest ID",
			userName:   "123456789012345678",
			externalID: "123456789012345678",
			want:       "123456789012345678",
		},
		{
			name:      "no externalId, as the connector's default template sends",
			userName:  "263801284",
			wantError: `Add "externalId": "{$user.id}" to the SCIM JSON Template`,
		},
		{
			name:       "whitespace externalId",
			userName:   "263801284",
			externalID: "  ",
			wantError:  `Add "externalId": "{$user.id}" to the SCIM JSON Template`,
		},
		{
			name:       "an unrendered template variable",
			userName:   "263801284",
			externalID: "{$user.id}",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "email",
			userName:   "263801284",
			externalID: "user@example.com",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "too long",
			userName:   "1234567890123456789",
			externalID: "1234567890123456789",
			wantError:  "externalId and userName must both be the numeric OneLogin user ID",
		},
		{
			name:       "externalId too long",
			userName:   "263801284",
			externalID: "1234567890123456789",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "negative",
			userName:   "263801284",
			externalID: "-263801284",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "leading zero",
			userName:   "263801284",
			externalID: "0263801284",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "zero",
			userName:   "263801284",
			externalID: "0",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:       "non-ASCII digits",
			userName:   "263801284",
			externalID: "２６３８０１２８４",
			wantError:  "externalId must be the numeric OneLogin user ID",
		},
		{
			name:      "no externalId and a username as userName",
			userName:  "jdoe",
			wantError: `send "externalId": "{$user.id}", and on the Parameters tab, set SCIM Username to OneLogin ID`,
		},
		{
			name:       "non-numeric externalId and a username as userName",
			userName:   "jdoe",
			externalID: "jdoe@example.com",
			wantError:  `send "externalId": "{$user.id}", and on the Parameters tab, set SCIM Username to OneLogin ID`,
		},
		{
			name:       "userName from SCIM Username's default, Username",
			userName:   "jdoe",
			externalID: "263882404",
			wantError:  "set SCIM Username to OneLogin ID. OneLogin finds existing users",
		},
		{
			name:       "userName that is another user's ID",
			userName:   "263882371",
			externalID: "263882404",
			wantError:  "set SCIM Username to OneLogin ID. OneLogin finds existing users",
		},
		{
			name:       "no userName",
			externalID: "263801284",
			wantError:  "set SCIM Username to OneLogin ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (oneLogin{}).NativeUserID(User{
				UserName:   tt.userName,
				ExternalID: tt.externalID,
			})
			if tt.wantError == "" {
				if err != nil || got != tt.want {
					t.Fatalf("NativeUserID() = %q, %v, want %q", got, err, tt.want)
				}
				return
			}
			invalid, ok := errors.AsType[*InvalidUserError](err)
			if !ok || !strings.Contains(invalid.Message, tt.wantError) {
				t.Fatalf("NativeUserID() = %q, %v, want an InvalidUserError containing %q", got, err, tt.wantError)
			}
		})
	}
}

func TestOneLoginNativeGroupID(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		groupID string
		want    string
	}{
		{
			name:    "role ID",
			prefix:  "onelogin/",
			groupID: "onelogin/1113742",
			want:    "1113742",
		},
		{
			name:    "group created through SCIM",
			prefix:  "onelogin/",
			groupID: "onelogin/8eaf7175-12c1-4f88-8ddb-dd6b9cb24928",
		},
		{
			name:    "role name",
			prefix:  "onelogin/",
			groupID: "onelogin/obot-scim-test",
		},
		{
			name:    "no ID after the prefix",
			prefix:  "onelogin/",
			groupID: "onelogin/",
		},
		{
			name:    "too long",
			prefix:  "onelogin/",
			groupID: "onelogin/1234567890123456789",
		},
		{
			name:    "leading zero",
			prefix:  "onelogin/",
			groupID: "onelogin/01113742",
		},
		{
			name:    "zero",
			prefix:  "onelogin/",
			groupID: "onelogin/0",
		},
		{
			name:    "another provider's prefix",
			prefix:  "onelogin/",
			groupID: "okta/1113742",
		},
		{
			name:    "no prefix",
			prefix:  "",
			groupID: "1113742",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (oneLogin{}).NativeGroupID(tt.prefix, tt.groupID); got != tt.want {
				t.Fatalf("NativeGroupID(%q, %q) = %q, want %q", tt.prefix, tt.groupID, got, tt.want)
			}
		})
	}
}

func TestOneLoginGroupConsoleURL(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		groupID string
		want    string
	}{
		{
			name:    "issuer",
			issuer:  "https://acme.onelogin.com/oidc/2",
			groupID: "1113742",
			want:    "https://acme.onelogin.com/roles/1113742/edit",
		},
		{
			name:    "issuer with a trailing slash and an upper-case host",
			issuer:  "https://ACME.onelogin.com/oidc/2/",
			groupID: "1113742",
			want:    "https://acme.onelogin.com/roles/1113742/edit",
		},
		{
			name:    "no native group ID",
			issuer:  "https://acme.onelogin.com/oidc/2",
			groupID: "",
		},
		{
			name:    "not a role ID",
			issuer:  "https://acme.onelogin.com/oidc/2",
			groupID: "../users",
		},
		{
			name:    "not HTTPS",
			issuer:  "http://acme.onelogin.com/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "port",
			issuer:  "https://acme.onelogin.com:8443/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "custom domain",
			issuer:  "https://login.acme.com/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "nested subdomain",
			issuer:  "https://evil.acme.onelogin.com/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "no subdomain",
			issuer:  "https://onelogin.com/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "IPv4 address",
			issuer:  "https://192.0.2.1/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "IPv6 address",
			issuer:  "https://[2001:db8::1]/oidc/2",
			groupID: "1113742",
		},
		{
			name:    "leading zero in the role ID",
			issuer:  "https://acme.onelogin.com/oidc/2",
			groupID: "01113742",
		},
		{
			name:    "not a URL",
			issuer:  "acme.onelogin.com",
			groupID: "1113742",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (oneLogin{}).GroupConsoleURL(tt.issuer, tt.groupID); got != tt.want {
				t.Fatalf("GroupConsoleURL(%q, %q) = %q, want %q", tt.issuer, tt.groupID, got, tt.want)
			}
		})
	}
}

// TestOneLoginEffectiveParameters checks that a OneLogin provider being set up may omit its API credential, and that
// the API base URL, which directory synchronization alone uses but which has a default, stays optional as the
// manifest lists it.
func TestOneLoginEffectiveParameters(t *testing.T) {
	params := EffectiveParameters(oneLoginManifest(), ProviderState{
		AuthProviderName: "onelogin-auth-provider",
	})
	if got, want := parameterNames(params.Required), []string{
		"OBOT_ONELOGIN_AUTH_PROVIDER_CLIENT_ID",
		"OBOT_ONELOGIN_AUTH_PROVIDER_ISSUER_URL",
	}; !slices.Equal(got, want) {
		t.Errorf("required = %v, want %v", got, want)
	}
	if got, want := parameterNames(params.Optional), []string{
		"OBOT_ONELOGIN_AUTH_PROVIDER_API_BASE_URL",
		"OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_ID",
		"OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_SECRET",
	}; !slices.Equal(got, want) {
		t.Errorf("optional = %v, want %v", got, want)
	}
	if got, want := params.Together, []string{
		"OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_ID",
		"OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_SECRET",
	}; !slices.Equal(got, want) {
		t.Errorf("together = %v, want %v", got, want)
	}
}

// oneLoginManifest returns a manifest like the OneLogin auth provider's, trimmed to what the tests need.
func oneLoginManifest() types2.AuthProviderManifest {
	return types2.AuthProviderManifest{
		Name: "OneLogin",
		RequiredConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_ONELOGIN_AUTH_PROVIDER_CLIENT_ID",
				FriendlyName: "Client ID",
			},
			{
				Name:         "OBOT_ONELOGIN_AUTH_PROVIDER_ISSUER_URL",
				FriendlyName: "Issuer URL",
			},
			{
				Name:         "OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_ID",
				FriendlyName: "API Client ID",
			},
			{
				Name:         "OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_SECRET",
				FriendlyName: "API Client Secret",
				Sensitive:    true,
			},
		},
		OptionalConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_ONELOGIN_AUTH_PROVIDER_API_BASE_URL",
				FriendlyName: "API Base URL",
			},
		},
		GroupIDPrefix: "onelogin/",
	}
}
