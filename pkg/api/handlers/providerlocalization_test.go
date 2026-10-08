package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
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

func TestProviderResponseLocaleSetsVary(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/model-providers", nil)
	request.Header.Set("Accept-Language", "ko")
	locale := providerResponseLocale(api.Context{Request: request, ResponseWriter: recorder})
	if locale != "ko" || recorder.Header().Get("Vary") != "Accept-Language" {
		t.Fatalf("locale = %q, Vary = %q", locale, recorder.Header().Get("Vary"))
	}
}

func TestProviderConversionsLocalizeCopies(t *testing.T) {
	metadata := types.CommonProviderMetadata{
		Name:        "Example",
		Description: "English description",
		Locales: map[string]types.ProviderTranslation{
			"ja": {Name: "例", Description: "日本語の説明"},
		},
		RequiredConfigurationParameters: []types.ProviderConfigurationParameter{{
			Name:         "EXAMPLE_KEY",
			FriendlyName: "Key",
			Description:  "English key description",
			Locales: map[string]types.ProviderParameterTranslation{
				"ja": {FriendlyName: "キー", Description: "日本語のキー説明"},
			},
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
		if response.Name != "例" || response.Description != "日本語の説明" {
			t.Fatalf("provider text was not localized: %#v", response)
		}
		parameter := response.RequiredConfigurationParameters[0]
		if parameter.FriendlyName != "キー" || parameter.Description != "日本語のキー説明" {
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
	if auth.Spec.Name != "Example" || auth.Spec.RequiredConfigurationParameters[0].Description != "English key description" {
		t.Fatal("auth provider resource changed while localizing")
	}
	if model.Spec.Name != "Example" || model.Spec.RequiredConfigurationParameters[0].Description != "English key description" {
		t.Fatal("model provider resource changed while localizing")
	}
	english := (&ModelProviderHandler{}).convertModelProvider(model, types.ModelProviderStatus{}, "en")
	if english.Name != "Example" || english.RequiredConfigurationParameters[0].FriendlyName != "Key" {
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
				got := adapter.LocalizedDirectoryDescription("okta-auth-provider", parameter.Name, description, locale)
				if got == "" || got == description {
					t.Fatalf("missing %s translation for %s", locale, parameter.Name)
				}
				manifest := types.AuthProviderManifest{CommonProviderMetadata: types.CommonProviderMetadata{
					OptionalConfigurationParameters: []types.ProviderConfigurationParameter{{
						Name: parameter.Name, Description: description,
						Locales: map[string]types.ProviderParameterTranslation{
							locale: {Description: "ordinary manifest description"},
						},
					}},
				}}
				localizeAuthProviderManifest(&manifest, "okta-auth-provider", locale)
				if manifest.OptionalConfigurationParameters[0].Description != got {
					t.Fatalf("%s SCIM description was replaced by manifest text", locale)
				}
			}
		}
	}
}
