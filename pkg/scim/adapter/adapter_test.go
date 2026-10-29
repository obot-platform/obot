package adapter

import (
	"errors"
	"reflect"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

const (
	choiceAuthProviderName = "choice-auth-provider"
	choiceAdapterType      = "choice"
	choiceParameterName    = "OBOT_CHOICE_AUTH_PROVIDER_SCIM"
	choiceDescription      = "Set to true to provision users and groups with SCIM."
)

// choiceAdapter has a SCIM choice parameter and no directory parameters, as the adapter of a provider whose login
// and directory reads share credentials does. Its other rules are Okta's.
type choiceAdapter struct {
	okta
}

// choiceWithDirectoryAdapter has Okta's directory parameters and a SCIM choice parameter.
type choiceWithDirectoryAdapter struct {
	choiceAdapter
}

func (choiceAdapter) Type() string {
	return choiceAdapterType
}

func (choiceAdapter) DirectoryParameters() []DirectoryParameter {
	return nil
}

func (choiceAdapter) SCIMChoiceParameter() SCIMChoiceParameter {
	return SCIMChoiceParameter{
		Name:        choiceParameterName,
		Description: choiceDescription,
		Translations: map[string]string{
			"ja": "SCIM でプロビジョニングするには true に設定します。",
		},
	}
}

func (choiceWithDirectoryAdapter) DirectoryParameters() []DirectoryParameter {
	return oktaDirectoryParameters
}

// choiceManifest returns a manifest of a provider with a SCIM choice parameter.
func choiceManifest() types2.AuthProviderManifest {
	return types2.AuthProviderManifest{
		Name: "Choice",
		RequiredConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name: "OBOT_CHOICE_AUTH_PROVIDER_CLIENT_ID",
			},
		},
		OptionalConfigurationParameters: []types2.ProviderConfigurationParameter{
			{
				Name: "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
			},
		},
		GroupIDPrefix: "choice/",
	}
}

func TestOktaAndOneLoginKeepTheSharedBehavior(t *testing.T) {
	for _, name := range []string{oktaAuthProviderName, oneLoginAuthProviderName} {
		t.Run(name, func(t *testing.T) {
			a, ok := ForAuthProvider(name)
			if !ok {
				t.Fatalf("ForAuthProvider(%q) found no adapter", name)
			}
			if got := a.UserKey(); got != UserKeyProviderUserID {
				t.Errorf("UserKey() = %v, want UserKeyProviderUserID", got)
			}
			if got := a.GroupKey(); got != GroupKeyDisplayName {
				t.Errorf("GroupKey() = %v, want GroupKeyDisplayName", got)
			}
			if got := a.Rules(); got != (Rules{}) {
				t.Errorf("Rules() = %+v, want none", got)
			}
			if got := a.SCIMChoiceParameter(); got.Name != "" {
				t.Errorf("SCIMChoiceParameter() = %+v, want none", got)
			}
			if got := a.NewGroupID("prefix/", "scim-id", "native-id"); got != "prefix/scim-id" {
				t.Errorf("NewGroupID() = %q, want the SCIM ID after the prefix", got)
			}
			id, err := a.NativeGroupIDFromExternalID("00g1a2b3c4d5e6f7g8h9")
			var invalid *InvalidGroupError
			if id != "" || !errors.As(err, &invalid) {
				t.Errorf("NativeGroupIDFromExternalID() = %q, %v, want an *InvalidGroupError", id, err)
			}
		})
	}
}

func TestSCIMChosen(t *testing.T) {
	RegisterForTest(t, choiceAuthProviderName, choiceAdapter{})
	okta, _ := ForAuthProvider(oktaAuthProviderName)
	choice, _ := ForAuthProvider(choiceAuthProviderName)

	tests := []struct {
		name    string
		adapter Adapter
		config  map[string]string
		want    bool
	}{
		{
			name:    "omitting the directory parameters chooses SCIM",
			adapter: okta,
			config: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID": "client",
			},
			want: true,
		},
		{
			name:    "empty directory parameters count as omitted",
			adapter: okta,
			config: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID": "",
			},
			want: true,
		},
		{
			name:    "providing a directory parameter chooses directory synchronization",
			adapter: okta,
			config: map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID": "service",
			},
		},
		{
			name:    "the choice parameter set to true chooses SCIM",
			adapter: choice,
			config: map[string]string{
				choiceParameterName: "true",
			},
			want: true,
		},
		{
			name:    "omitting the choice parameter chooses directory synchronization",
			adapter: choice,
			config:  map[string]string{},
		},
		{
			name:    "any other value of the choice parameter chooses directory synchronization",
			adapter: choice,
			config: map[string]string{
				choiceParameterName: "false",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SCIMChosen(tt.adapter, tt.config); got != tt.want {
				t.Fatalf("SCIMChosen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveParametersWithAChoiceParameter(t *testing.T) {
	RegisterForTest(t, choiceAuthProviderName, choiceAdapter{})
	hiddenChoice := types2.ProviderConfigurationParameter{
		Name:        choiceParameterName,
		Description: choiceDescription,
		Hidden:      true,
	}
	tests := []struct {
		name            string
		configured      bool
		connAdapterType string
		// listed adds the choice parameter to the manifest's optional parameters, which it should never be.
		listed       bool
		wantOptional []types2.ProviderConfigurationParameter
	}{
		{
			name: "a provider being set up lists the choice as a hidden optional parameter",
			wantOptional: []types2.ProviderConfigurationParameter{
				{
					Name: "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				},
				hiddenChoice,
			},
		},
		{
			name:       "a configured provider without a connection does not list it",
			configured: true,
			wantOptional: []types2.ProviderConfigurationParameter{
				{
					Name: "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				},
			},
		},
		{
			name:            "a provider with a connection does not list it",
			configured:      true,
			connAdapterType: choiceAdapterType,
			wantOptional: []types2.ProviderConfigurationParameter{
				{
					Name: "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				},
			},
		},
		{
			name:            "a manifest that lists it does not keep it",
			configured:      true,
			connAdapterType: choiceAdapterType,
			listed:          true,
			wantOptional: []types2.ProviderConfigurationParameter{
				{
					Name: "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := choiceManifest()
			if tt.listed {
				manifest.OptionalConfigurationParameters = append(manifest.OptionalConfigurationParameters, types2.ProviderConfigurationParameter{
					Name: choiceParameterName,
				})
			}
			params := EffectiveParameters(manifest, ProviderState{
				AuthProviderName:      choiceAuthProviderName,
				Configured:            tt.configured,
				ConnectionAdapterType: tt.connAdapterType,
			})

			if got, want := parameterNames(params.Required), []string{"OBOT_CHOICE_AUTH_PROVIDER_CLIENT_ID"}; !reflect.DeepEqual(got, want) {
				t.Errorf("Required = %v, want %v", got, want)
			}
			if !reflect.DeepEqual(params.Optional, tt.wantOptional) {
				t.Errorf("Optional = %+v, want %+v", params.Optional, tt.wantOptional)
			}
			// The choice is never stored, whatever the provider's state.
			if want := []string{choiceParameterName}; !reflect.DeepEqual(params.Dropped, want) {
				t.Errorf("Dropped = %v, want %v", params.Dropped, want)
			}
			if len(params.Together) != 0 {
				t.Errorf("Together = %v, want none", params.Together)
			}
		})
	}

	// A provider without a group ID prefix does not support SCIM, so it has no choice to drop.
	manifest := choiceManifest()
	manifest.GroupIDPrefix = ""
	if params := EffectiveParameters(manifest, ProviderState{
		AuthProviderName: choiceAuthProviderName,
	}); len(params.Dropped) != 0 {
		t.Errorf("Dropped = %v for a provider that does not support SCIM, want none", params.Dropped)
	}
}

func TestEffectiveParametersDropBothTheChoiceAndUnusedDirectoryParameters(t *testing.T) {
	// An adapter with both is not expected, but neither list replaces the other.
	RegisterForTest(t, choiceAuthProviderName, choiceWithDirectoryAdapter{})

	manifest := oktaManifest()
	params := EffectiveParameters(manifest, ProviderState{
		AuthProviderName:      choiceAuthProviderName,
		Configured:            true,
		ConnectionAdapterType: choiceAdapterType,
	})
	want := []string{
		choiceParameterName,
		"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
		"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
	}
	if !reflect.DeepEqual(params.Dropped, want) {
		t.Fatalf("Dropped = %v, want %v", params.Dropped, want)
	}
}

func TestLocalizedChoiceDescription(t *testing.T) {
	RegisterForTest(t, choiceAuthProviderName, choiceAdapter{})
	tests := []struct {
		name           string
		parameterName  string
		description    string
		locale         string
		want           string
		wantRecognized bool
	}{
		{
			name:           "the adapter's description is translated",
			parameterName:  choiceParameterName,
			description:    choiceDescription,
			locale:         "ja",
			want:           "SCIM でプロビジョニングするには true に設定します。",
			wantRecognized: true,
		},
		{
			name:           "a locale without a translation keeps the adapter's description",
			parameterName:  choiceParameterName,
			description:    choiceDescription,
			locale:         "ko",
			want:           choiceDescription,
			wantRecognized: true,
		},
		{
			name:          "another description of the parameter is not the adapter's",
			parameterName: choiceParameterName,
			description:   "Something else.",
			locale:        "ja",
			want:          "Something else.",
		},
		{
			name:          "another parameter is not the choice",
			parameterName: "OBOT_CHOICE_AUTH_PROVIDER_CLIENT_ID",
			description:   choiceDescription,
			locale:        "ja",
			want:          choiceDescription,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, recognized := LocalizedDirectoryDescription(choiceAuthProviderName, tt.parameterName, tt.description, tt.locale)
			if got != tt.want || recognized != tt.wantRecognized {
				t.Fatalf("LocalizedDirectoryDescription() = %q, %v, want %q, %v", got, recognized, tt.want, tt.wantRecognized)
			}
		})
	}
}

func TestNativeUserIDFromExternalID(t *testing.T) {
	tests := []struct {
		name             string
		authProviderName string
		externalID       string
		want             string
		wantErr          bool
	}{
		{
			name:             "an Okta user ID",
			authProviderName: oktaAuthProviderName,
			externalID:       " 00u1a2b3c4d5e6f7g8h9 ",
			want:             "00u1a2b3c4d5e6f7g8h9",
		},
		{
			name:             "an Okta externalId with a space",
			authProviderName: oktaAuthProviderName,
			externalID:       "not an id",
			wantErr:          true,
		},
		{
			name:             "a OneLogin user ID, whatever the userName",
			authProviderName: oneLoginAuthProviderName,
			externalID:       "263801284",
			want:             "263801284",
		},
		{
			name:             "a OneLogin externalId that is not a user ID",
			authProviderName: oneLoginAuthProviderName,
			externalID:       "user@example.com",
			wantErr:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := ForAuthProvider(tt.authProviderName)
			got, err := a.NativeUserIDFromExternalID(tt.externalID)
			var invalid *InvalidUserError
			if tt.wantErr != errors.As(err, &invalid) || got != tt.want {
				t.Fatalf("NativeUserIDFromExternalID(%q) = %q, %v", tt.externalID, got, err)
			}
		})
	}
}
