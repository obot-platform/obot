package adapter

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

const (
	// entraTestObjectID is an Entra object ID, in the lower-case form Microsoft Graph writes.
	entraTestObjectID = "00000000-0000-4000-8000-0000000000c1"
)

func TestEntraRegistry(t *testing.T) {
	a, ok := ForAuthProvider("entra-auth-provider")
	if !ok || a.Type() != "entra" {
		t.Fatalf("ForAuthProvider(entra-auth-provider) = %v, %v", a, ok)
	}
	if byType, ok := Lookup("entra"); !ok || byType.Type() != "entra" {
		t.Fatalf("Lookup(entra) = %v, %v", byType, ok)
	}
	if !SupportsSCIM("entra-auth-provider", entraManifest()) {
		t.Error("the Entra provider does not support SCIM")
	}
}

func TestEntraRules(t *testing.T) {
	a := entra{}
	if got := a.UserKey(); got != UserKeyGroupLookupID {
		t.Errorf("UserKey() = %v, want UserKeyGroupLookupID", got)
	}
	if got := a.GroupKey(); got != GroupKeyExternalID {
		t.Errorf("GroupKey() = %v, want GroupKeyExternalID", got)
	}
	if got, want := a.PatchRules(), (PatchRules{ReplaceAddsUnmatched: true}); got != want {
		t.Errorf("PatchRules() = %+v, want %+v", got, want)
	}
	if got, want := a.Rules(), (Rules{DeletesUsers: true}); got != want {
		t.Errorf("Rules() = %+v, want %+v", got, want)
	}
	if got := a.DirectoryParameters(); len(got) != 0 {
		t.Errorf("DirectoryParameters() = %v, want none", got)
	}
	if got := a.IssuerConfigurationParameter(); got != "OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID" {
		t.Errorf("IssuerConfigurationParameter() = %q", got)
	}

	choice := a.SCIMChoiceParameter()
	if choice.Name != "OBOT_ENTRA_AUTH_PROVIDER_SCIM" || choice.Description == "" {
		t.Fatalf("SCIMChoiceParameter() = %+v", choice)
	}
	for _, locale := range []string{"ja", "ko", "zh-CN"} {
		if translated := choice.Translations[locale]; translated == "" || translated == choice.Description {
			t.Errorf("the choice parameter has no %s translation", locale)
		}
	}
}

func TestEntraNativeUserID(t *testing.T) {
	tests := []struct {
		name       string
		userName   string
		externalID string
		want       string
		wantError  bool
	}{
		{
			name:       "object ID",
			userName:   "user@example.com",
			externalID: entraTestObjectID,
			want:       entraTestObjectID,
		},
		{
			name:       "surrounding whitespace",
			userName:   "user@example.com",
			externalID: " " + entraTestObjectID + "\t",
			want:       entraTestObjectID,
		},
		{
			name:       "upper case, which is the same object ID",
			userName:   "user@example.com",
			externalID: strings.ToUpper(entraTestObjectID),
			want:       entraTestObjectID,
		},
		{
			name:       "a userName mangled by a soft delete does not matter",
			userName:   strings.ReplaceAll(entraTestObjectID, "-", "") + "user@example.com",
			externalID: entraTestObjectID,
			want:       entraTestObjectID,
		},
		{
			name:       "mailNickname, as the default mapping sends",
			userName:   "user@example.com",
			externalID: "jdoe",
			wantError:  true,
		},
		{
			name:       "empty",
			userName:   "user@example.com",
			externalID: "",
			wantError:  true,
		},
		{
			name:       "whitespace",
			userName:   "user@example.com",
			externalID: "  ",
			wantError:  true,
		},
		{
			name:       "no hyphens",
			userName:   "user@example.com",
			externalID: strings.ReplaceAll(entraTestObjectID, "-", ""),
			wantError:  true,
		},
		{
			name:       "braces",
			userName:   "user@example.com",
			externalID: "{" + entraTestObjectID + "}",
			wantError:  true,
		},
		{
			name:       "URN",
			userName:   "user@example.com",
			externalID: "urn:uuid:" + entraTestObjectID,
			wantError:  true,
		},
		{
			name:       "a hyphen out of place",
			userName:   "user@example.com",
			externalID: "000000000-000-4000-8000-0000000000c1",
			wantError:  true,
		},
		{
			name:       "not hexadecimal",
			userName:   "user@example.com",
			externalID: "00000000-0000-4000-8000-0000000000cg",
			wantError:  true,
		},
		{
			name:       "the userName's object ID does not stand in for externalId",
			userName:   entraTestObjectID,
			externalID: "jdoe",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (entra{}).NativeUserID(User{
				UserName:   tt.userName,
				ExternalID: tt.externalID,
			})
			if !tt.wantError {
				if err != nil || got != tt.want {
					t.Fatalf("NativeUserID() = %q, %v, want %q", got, err, tt.want)
				}
				return
			}
			// Entra shows the message verbatim, so it names the mapping to change.
			invalid, ok := errors.AsType[*InvalidUserError](err)
			if !ok || !strings.Contains(invalid.Message, "map objectId to externalId (the default maps mailNickname)") {
				t.Fatalf("NativeUserID() = %q, %v, want an InvalidUserError that names the mapping", got, err)
			}

			// An externalId filter refuses the same values.
			if got, err := (entra{}).NativeUserIDFromExternalID(tt.externalID); err == nil {
				t.Fatalf("NativeUserIDFromExternalID(%q) = %q, want an error", tt.externalID, got)
			}
		})
	}
}

func TestEntraNewUserIdentity(t *testing.T) {
	got := (entra{}).NewUserIdentity(entraTestObjectID)
	want := Identity{
		Username:              entraTestObjectID,
		ProviderUsername:      "OBOT_SCIM_PENDING/" + entraTestObjectID,
		ProviderUserID:        "OBOT_SCIM_PENDING/" + entraTestObjectID,
		ProviderGroupLookupID: entraTestObjectID,
	}
	if got != want {
		t.Fatalf("NewUserIdentity() = %+v, want %+v", got, want)
	}
}

func TestEntraGroupIDs(t *testing.T) {
	a := entra{}

	nativeID, err := a.NativeGroupIDFromExternalID(" " + strings.ToUpper(entraTestObjectID) + " ")
	if err != nil || nativeID != entraTestObjectID {
		t.Fatalf("NativeGroupIDFromExternalID() = %q, %v, want %q", nativeID, err, entraTestObjectID)
	}
	groupID := a.NewGroupID("entra/", "8eaf7175-12c1-4f88-8ddb-dd6b9cb24928", nativeID)
	if want := "entra/" + entraTestObjectID; groupID != want {
		t.Fatalf("NewGroupID() = %q, want %q", groupID, want)
	}
	if got := a.NativeGroupID("entra/", groupID); got != nativeID {
		t.Fatalf("NativeGroupID(%q) = %q, want %q", groupID, got, nativeID)
	}

	for _, externalID := range []string{"", "Engineering", strings.ReplaceAll(entraTestObjectID, "-", "")} {
		_, err := a.NativeGroupIDFromExternalID(externalID)
		invalid, ok := errors.AsType[*InvalidGroupError](err)
		if !ok || !strings.Contains(invalid.Message, "Provision Microsoft Entra ID Groups, map objectId to externalId") {
			t.Errorf("NativeGroupIDFromExternalID(%q) = %v, want an InvalidGroupError that names the mapping", externalID, err)
		}
	}
}

func TestEntraNativeGroupID(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		groupID string
		want    string
	}{
		{
			name:    "object ID",
			prefix:  "entra/",
			groupID: "entra/" + entraTestObjectID,
			want:    entraTestObjectID,
		},
		{
			name:    "upper case, which no Entra group ID has",
			prefix:  "entra/",
			groupID: "entra/" + strings.ToUpper(entraTestObjectID),
		},
		{
			name:    "group name",
			prefix:  "entra/",
			groupID: "entra/engineering",
		},
		{
			name:    "no ID after the prefix",
			prefix:  "entra/",
			groupID: "entra/",
		},
		{
			name:    "another provider's prefix",
			prefix:  "entra/",
			groupID: "okta/" + entraTestObjectID,
		},
		{
			name:    "no prefix",
			prefix:  "",
			groupID: entraTestObjectID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (entra{}).NativeGroupID(tt.prefix, tt.groupID); got != tt.want {
				t.Fatalf("NativeGroupID(%q, %q) = %q, want %q", tt.prefix, tt.groupID, got, tt.want)
			}
		})
	}
}

func TestEntraGroupConsoleURL(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		groupID string
		want    string
	}{
		{
			name:    "object ID",
			issuer:  "00000000-0000-4000-8000-000000000001",
			groupID: entraTestObjectID,
			want:    "https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupDetailsMenuBlade/~/Overview/groupId/" + entraTestObjectID,
		},
		{
			name:    "no issuer is needed",
			groupID: entraTestObjectID,
			want:    "https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupDetailsMenuBlade/~/Overview/groupId/" + entraTestObjectID,
		},
		{
			name:    "no native group ID",
			issuer:  "00000000-0000-4000-8000-000000000001",
			groupID: "",
		},
		{
			name:    "not an object ID",
			issuer:  "00000000-0000-4000-8000-000000000001",
			groupID: "../users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (entra{}).GroupConsoleURL(tt.issuer, tt.groupID); got != tt.want {
				t.Fatalf("GroupConsoleURL(%q, %q) = %q, want %q", tt.issuer, tt.groupID, got, tt.want)
			}
		})
	}
}

func TestEntraErrorHints(t *testing.T) {
	hints := (entra{}).ErrorHints()
	if !strings.Contains(hints.Retry, "Restart provisioning") {
		t.Errorf("Retry = %q, want it to name Restart provisioning", hints.Retry)
	}
	if !strings.Contains(hints.ExternalIDChanged, "Map objectId to externalId") {
		t.Errorf("ExternalIDChanged = %q, want it to name the mapping", hints.ExternalIDChanged)
	}
	// Entra users are deleted, and Entra's PATCH requests never add a member with a value filter.
	if hints.UserDelete != "" || hints.ValuelessFilteredAdd != "" {
		t.Errorf("hints = %+v, want no UserDelete or ValuelessFilteredAdd hint", hints)
	}
}

// TestEntraEffectiveParameters checks that every Entra parameter stays as the manifest lists it, and that the choice
// parameter is offered only while the provider is being set up without a connection, and is dropped every time.
func TestEntraEffectiveParameters(t *testing.T) {
	choice := (entra{}).SCIMChoiceParameter()
	tests := []struct {
		name            string
		configured      bool
		connAdapterType string
		stored          map[string]string
		wantChoice      bool
	}{
		{
			name:       "a provider being set up offers the choice",
			wantChoice: true,
		},
		{
			name:       "a configured provider synchronizes its directory",
			configured: true,
		},
		{
			name:            "a provider with a connection",
			configured:      true,
			connAdapterType: "entra",
			stored: map[string]string{
				"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID": "client",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := EffectiveParameters(entraManifest(), ProviderState{
				AuthProviderName:      "entra-auth-provider",
				Configured:            tt.configured,
				ConnectionAdapterType: tt.connAdapterType,
				Stored:                tt.stored,
			})
			if got, want := parameterNames(params.Required), []string{
				"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID",
				"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET",
				"OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID",
			}; !slices.Equal(got, want) {
				t.Errorf("Required = %v, want %v", got, want)
			}

			wantOptional := []types2.ProviderConfigurationParameter{
				{
					Name:         "OBOT_AUTH_PROVIDER_ENABLE_LOGGING",
					FriendlyName: "Enable Logging",
				},
			}
			if tt.wantChoice {
				wantOptional = append(wantOptional, types2.ProviderConfigurationParameter{
					Name:        choice.Name,
					Description: choice.Description,
					Hidden:      true,
				})
			}
			if !reflect.DeepEqual(params.Optional, wantOptional) {
				t.Errorf("Optional = %+v, want %+v", params.Optional, wantOptional)
			}
			if want := []string{choice.Name}; !slices.Equal(params.Dropped, want) {
				t.Errorf("Dropped = %v, want %v", params.Dropped, want)
			}
			if len(params.Together) != 0 {
				t.Errorf("Together = %v, want none", params.Together)
			}
		})
	}
}

func TestEntraSCIMChosen(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   bool
	}{
		{
			name: "true chooses SCIM",
			config: map[string]string{
				"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID": "client",
				"OBOT_ENTRA_AUTH_PROVIDER_SCIM":      "true",
			},
			want: true,
		},
		{
			name: "absent chooses directory synchronization, as every existing configuration does",
			config: map[string]string{
				"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID": "client",
			},
		},
		{
			name: "false chooses directory synchronization",
			config: map[string]string{
				"OBOT_ENTRA_AUTH_PROVIDER_SCIM": "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SCIMChosen(entra{}, tt.config); got != tt.want {
				t.Fatalf("SCIMChosen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntraLocalizedChoiceDescription(t *testing.T) {
	choice := (entra{}).SCIMChoiceParameter()
	got, recognized := LocalizedDirectoryDescription("entra-auth-provider", choice.Name, choice.Description, "ja")
	if !recognized || got != choice.Translations["ja"] {
		t.Fatalf("LocalizedDirectoryDescription() = %q, %v, want the Japanese translation", got, recognized)
	}
}

// entraManifest returns a manifest like the Entra auth provider's, trimmed to what the tests need.
func entraManifest() types2.AuthProviderManifest {
	return types2.AuthProviderManifest{
		Name: "Microsoft Entra",
		RequiredConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID",
				FriendlyName: "Client ID",
			},
			{
				Name:         "OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET",
				FriendlyName: "Client Secret",
				Sensitive:    true,
			},
			{
				Name:         "OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID",
				FriendlyName: "Tenant ID",
			},
		},
		OptionalConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name:         "OBOT_AUTH_PROVIDER_ENABLE_LOGGING",
				FriendlyName: "Enable Logging",
			},
		},
		GroupIDPrefix: "entra/",
	}
}
