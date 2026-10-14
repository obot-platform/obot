package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestProviderLocale(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{header: "", want: "en"},
		{header: "ja-JP,ko;q=0.8,en;q=0.5", want: "ja"},
		{header: "en;q=0.5,ja;q=0.9", want: "ja"},
		{header: "fr,ko-KR;q=0.7,en;q=0.5", want: "ko"},
		{header: "zh-Hans-CN,ja;q=0.8", want: "zh-CN"},
		{header: "zh-SG", want: "zh-CN"},
		{header: "zh-TW,ja;q=0.5", want: "ja"},
		{header: "zh-HK", want: "en"},
		{header: "ja;q=0,en;q=0.5", want: "en"},
		{header: "not a language", want: "en"},
	}
	for _, test := range tests {
		t.Run(test.header, func(t *testing.T) {
			if got := providerLocale(test.header); got != test.want {
				t.Fatalf("providerLocale(%q) = %q, want %q", test.header, got, test.want)
			}
		})
	}
}

func TestProviderResponseLocaleFromContext(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/model-providers", nil)
	request = request.WithContext(i18n.WithLocale(request.Context(), "ko"))
	locale := providerResponseLocale(api.Context{Request: request})
	if locale != "ko" {
		t.Fatalf("locale = %q, want ko", locale)
	}
}

func TestProviderConversionsLocalizeCopies(t *testing.T) {
	metadata := types.CommonProviderMetadata{
		Name:        "${google_auth_provider_name}",
		Description: "${google_auth_provider_description}",
		RequiredConfigurationParameters: []types.ProviderConfigurationParameter{{
			Name:         "EXAMPLE_KEY",
			FriendlyName: "${google_auth_provider_client_id_friendly_name}",
			Description:  "${google_auth_provider_client_id_description}",
		}},
	}
	auth := v1.AuthProvider{
		Name: "example-auth",
		Spec: v1.AuthProviderSpec{AuthProviderManifest: types.AuthProviderManifest{
			CommonProviderMetadata: metadata,
		}},
	}
	model := v1.ModelProvider{
		Name: "example-model",
		Spec: v1.ModelProviderSpec{ModelProviderManifest: types.ModelProviderManifest{
			CommonProviderMetadata: metadata,
		}},
	}
	params := adapter.Parameters{Required: auth.Spec.RequiredConfigurationParameters}
	authResponse := (&AuthProviderHandler{}).convertAuthProvider(auth, types.AuthProviderStatus{}, params, "ja")
	modelResponse := (&ModelProviderHandler{}).convertModelProvider(model, types.ModelProviderStatus{}, "ja")
	for _, response := range []types.CommonProviderMetadata{authResponse.CommonProviderMetadata, modelResponse.CommonProviderMetadata} {
		if response.Name != "Google" || response.Description != i18n.Text("ja", "google_auth_provider_description") {
			t.Fatalf("provider text was not localized: %#v", response)
		}
		parameter := response.RequiredConfigurationParameters[0]
		if parameter.FriendlyName != i18n.Text("ja", "google_auth_provider_client_id_friendly_name") ||
			parameter.Description != i18n.Text("ja", "google_auth_provider_client_id_description") {
			t.Fatalf("parameter text was not localized: %#v", parameter)
		}
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"locales"`) || strings.Contains(string(data), `"translations"`) {
			t.Fatalf("locale map leaked into response: %s", data)
		}
	}
	if auth.Spec.Name != "${google_auth_provider_name}" ||
		auth.Spec.RequiredConfigurationParameters[0].Description != "${google_auth_provider_client_id_description}" {
		t.Fatal("auth provider resource changed while localizing")
	}
	if model.Spec.Name != "${google_auth_provider_name}" ||
		model.Spec.RequiredConfigurationParameters[0].Description != "${google_auth_provider_client_id_description}" {
		t.Fatal("model provider resource changed while localizing")
	}
	english := (&ModelProviderHandler{}).convertModelProvider(model, types.ModelProviderStatus{}, "en")
	if english.Name != "Google" || english.RequiredConfigurationParameters[0].FriendlyName != "Client ID" {
		t.Fatal("English fallback changed after a localized conversion")
	}
}

func TestOktaDirectoryDescriptions(t *testing.T) {
	provider, ok := adapter.ForAuthProvider("okta-auth-provider")
	if !ok {
		t.Fatal("Okta adapter missing")
	}
	for _, parameter := range provider.DirectoryParameters() {
		for _, description := range []string{parameter.SetupDescription, parameter.UnusedDescription} {
			for _, locale := range []string{"ja", "ko", "zh-CN"} {
				got, recognized := adapter.LocalizedDirectoryDescription("okta-auth-provider", parameter.Name, description, locale)
				if !recognized || got == "" || got == description {
					t.Fatalf("missing %s translation for %s", locale, parameter.Name)
				}
				manifest := types.AuthProviderManifest{
					OptionalConfigurationParameters: []types.ProviderConfigurationParameter{{
						Name:        parameter.Name,
						Description: description,
					}},
				}
				localizeAuthProviderManifest(&manifest, "okta-auth-provider", locale)
				if manifest.OptionalConfigurationParameters[0].Description != got {
					t.Fatalf("%s SCIM description was replaced by manifest text", locale)
				}
			}
		}
	}
}

func TestOktaDirectoryDescriptionFallback(t *testing.T) {
	provider, ok := adapter.ForAuthProvider("okta-auth-provider")
	if !ok {
		t.Fatal("Okta adapter missing")
	}
	for _, parameter := range provider.DirectoryParameters() {
		tests := []struct {
			name        string
			description string
			want        string
		}{
			{
				name:        "setup",
				description: parameter.SetupDescription,
				want:        parameter.SetupDescription,
			},
			{
				name:        "unused",
				description: parameter.UnusedDescription,
				want:        parameter.UnusedDescription,
			},
			{
				name:        "ordinary manifest description",
				description: "ordinary setup description",
				want:        "ordinary setup description",
			},
		}
		for _, tt := range tests {
			for _, locale := range []string{"en", "fr"} {
				t.Run(parameter.Name+"/"+tt.name+"/"+locale, func(t *testing.T) {
					definition := types.ProviderConfigurationParameter{
						Name:        parameter.Name,
						Description: tt.description,
					}
					manifest := types.AuthProviderManifest{
						RequiredConfigurationParameters: []types.ProviderConfigurationParameter{definition},
						OptionalConfigurationParameters: []types.ProviderConfigurationParameter{definition},
					}
					localizeAuthProviderManifest(&manifest, "okta-auth-provider", locale)
					for _, parameters := range [][]types.ProviderConfigurationParameter{manifest.RequiredConfigurationParameters, manifest.OptionalConfigurationParameters} {
						if got := parameters[0].Description; got != tt.want {
							t.Errorf("description = %q, want %q", got, tt.want)
						}
					}
				})
			}
		}
	}
}

func TestLocalProviderReferences(t *testing.T) {
	provider := localauth.AuthProvider()
	params := adapter.Parameters{Required: provider.Spec.RequiredConfigurationParameters}
	for _, locale := range []string{"ja", "en", "ko", "zh-CN", "en"} {
		response := (&AuthProviderHandler{}).convertAuthProvider(*provider, types.AuthProviderStatus{}, params, locale)
		if response.Name != i18n.Text(locale, "local_provider_name") ||
			response.RequiredConfigurationParameters[0].FriendlyName != i18n.Text(locale, "local_provider_domains_name") {
			t.Fatalf("incorrect Local provider response for %s: %#v", locale, response)
		}
	}
	if provider.Spec.Name != "${local_provider_name}" {
		t.Fatal("stored Local provider name changed")
	}
}

func TestProviderOptionalParameterReferences(t *testing.T) {
	manifest := types.CommonProviderMetadata{
		Name: "Literal provider",
		OptionalConfigurationParameters: []types.ProviderConfigurationParameter{{
			Name:         "ENV_NAME",
			FriendlyName: "${google_auth_provider_postgres_connection_dsn_friendly_name}",
			Description:  "${google_auth_provider_postgres_connection_dsn_description}",
		}},
	}
	for _, locale := range []string{"ko", "en", "unavailable"} {
		response := *manifest.DeepCopy()
		localizeProviderMetadata(&response, locale)
		parameter := response.OptionalConfigurationParameters[0]
		if parameter.Name != "ENV_NAME" || response.Name != "Literal provider" ||
			parameter.FriendlyName != i18n.Text(locale, "google_auth_provider_postgres_connection_dsn_friendly_name") ||
			parameter.Description != i18n.Text(locale, "google_auth_provider_postgres_connection_dsn_description") {
			t.Fatalf("incorrect optional parameter for %s: %#v", locale, parameter)
		}
	}
}

func TestModelProviderCatalogReferences(t *testing.T) {
	provider := v1.ModelProvider{
		Name: "openai-model-provider",
		Spec: v1.ModelProviderSpec{ModelProviderManifest: types.ModelProviderManifest{
			CommonProviderMetadata: types.CommonProviderMetadata{
				Name:        "${openai_model_provider_name}",
				Description: "${openai_model_provider_description}",
				Command:     "bin/provider",
				RequiredConfigurationParameters: []types.ProviderConfigurationParameter{{
					Name:         "OBOT_OPENAI_MODEL_PROVIDER_API_KEY",
					FriendlyName: "${openai_model_provider_api_key_friendly_name}",
					Description:  "${openai_model_provider_api_key_description}",
					Sensitive:    true,
				}},
			},
		}},
	}
	for _, locale := range []string{"ja", "en", "ko", "zh-CN"} {
		response := (&ModelProviderHandler{}).convertModelProvider(provider, types.ModelProviderStatus{}, locale)
		parameter := response.RequiredConfigurationParameters[0]
		if response.Name != "OpenAI" || response.Description != i18n.Text(locale, "openai_model_provider_description") ||
			parameter.FriendlyName != i18n.Text(locale, "openai_model_provider_api_key_friendly_name") ||
			parameter.Description != i18n.Text(locale, "openai_model_provider_api_key_description") {
			t.Fatalf("incorrect model provider text for %s: %#v", locale, response)
		}
		if response.ID != provider.Name || response.Command != "bin/provider" ||
			parameter.Name != "OBOT_OPENAI_MODEL_PROVIDER_API_KEY" || !parameter.Sensitive {
			t.Fatal("operational fields changed")
		}
	}
}
