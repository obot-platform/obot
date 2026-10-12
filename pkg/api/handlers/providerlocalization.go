package handlers

import (
	"maps"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"golang.org/x/text/language"
)

// providerLocale selects the first supported, positively weighted language range.
func providerLocale(header string) string {
	return i18n.Locale(header)
}

func providerResponseLocale(req api.Context, metadata ...types.CommonProviderMetadata) string {
	if req.ResponseWriter != nil {
		req.ResponseWriter.Header().Add("Vary", "Accept-Language")
	}
	if req.Request == nil {
		return "en"
	}
	var locales []string
	for _, provider := range metadata {
		locales = append(locales, slices.Collect(maps.Keys(provider.Locales))...)
		for _, parameters := range [][]types.ProviderConfigurationParameter{provider.RequiredConfigurationParameters, provider.OptionalConfigurationParameters} {
			for _, parameter := range parameters {
				locales = append(locales, slices.Collect(maps.Keys(parameter.Locales))...)
			}
		}
	}
	return i18n.Locale(req.Request.Header.Get("Accept-Language"), locales...)
}

func providerTranslation[T any](translations map[string]T, locale string) T {
	if value, ok := translations[locale]; ok {
		return value
	}
	for _, key := range slices.Sorted(maps.Keys(translations)) {
		if tag, err := language.Parse(key); err == nil && tag.String() == locale {
			return translations[key]
		}
	}
	var zero T
	return zero
}

func apiMessage(req api.Context, key string, values ...string) string {
	if len(values)%2 != 0 {
		panic("API message arguments must be key/value pairs")
	}
	args := make(map[string]string, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		args[values[i]] = values[i+1]
	}
	return i18n.Message(providerResponseLocale(req), key, args)
}

func localizeProviderMetadata(metadata *types.CommonProviderMetadata, locale string) {
	translation := providerTranslation(metadata.Locales, locale)
	if translation.Name != "" {
		metadata.Name = translation.Name
	}
	if translation.Description != "" {
		metadata.Description = translation.Description
	}
	metadata.Locales = nil
	for i := range metadata.RequiredConfigurationParameters {
		localizeProviderParameter(&metadata.RequiredConfigurationParameters[i], locale)
	}
	for i := range metadata.OptionalConfigurationParameters {
		localizeProviderParameter(&metadata.OptionalConfigurationParameters[i], locale)
	}
}

func localizeProviderParameter(parameter *types.ProviderConfigurationParameter, locale string) {
	translation := providerTranslation(parameter.Locales, locale)
	if translation.FriendlyName != "" {
		parameter.FriendlyName = translation.FriendlyName
	}
	if translation.Description != "" {
		parameter.Description = translation.Description
	}
	parameter.Locales = nil
}

func copyProviderParameters(parameters []types.ProviderConfigurationParameter) []types.ProviderConfigurationParameter {
	if parameters == nil {
		return nil
	}
	result := make([]types.ProviderConfigurationParameter, len(parameters))
	for i := range parameters {
		parameters[i].DeepCopyInto(&result[i])
	}
	return result
}

func localizeAuthProviderManifest(manifest *types.AuthProviderManifest, providerName, locale string) {
	requiredDescriptions := make([]string, len(manifest.RequiredConfigurationParameters))
	requiredOverrides := make([]bool, len(manifest.RequiredConfigurationParameters))
	for i := range manifest.RequiredConfigurationParameters {
		parameter := &manifest.RequiredConfigurationParameters[i]
		requiredDescriptions[i], requiredOverrides[i] = adapter.LocalizedDirectoryDescription(providerName, parameter.Name, parameter.Description, locale)
	}
	optionalDescriptions := make([]string, len(manifest.OptionalConfigurationParameters))
	optionalOverrides := make([]bool, len(manifest.OptionalConfigurationParameters))
	for i := range manifest.OptionalConfigurationParameters {
		parameter := &manifest.OptionalConfigurationParameters[i]
		optionalDescriptions[i], optionalOverrides[i] = adapter.LocalizedDirectoryDescription(providerName, parameter.Name, parameter.Description, locale)
	}
	localizeProviderMetadata(&manifest.CommonProviderMetadata, locale)
	for i, description := range requiredDescriptions {
		if requiredOverrides[i] {
			manifest.RequiredConfigurationParameters[i].Description = description
		}
	}
	for i, description := range optionalDescriptions {
		if optionalOverrides[i] {
			manifest.OptionalConfigurationParameters[i].Description = description
		}
	}
}
