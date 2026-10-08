package handlers

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"golang.org/x/text/language"
)

// providerLocale selects the first supported, positively weighted language range.
func providerLocale(header string) string {
	tags, weights, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return "en"
	}
	for i, tag := range tags {
		if weights[i] <= 0 {
			continue
		}
		value := strings.ToLower(tag.String())
		switch {
		case value == "en" || strings.HasPrefix(value, "en-"):
			return "en"
		case value == "ja" || strings.HasPrefix(value, "ja-"):
			return "ja"
		case value == "ko" || strings.HasPrefix(value, "ko-"):
			return "ko"
		case value == "zh-cn" || value == "zh-sg" || value == "zh-hans" ||
			strings.HasPrefix(value, "zh-hans-"):
			return "zh-CN"
		}
	}
	return "en"
}

func providerResponseLocale(req api.Context) string {
	req.ResponseWriter.Header().Add("Vary", "Accept-Language")
	return providerLocale(req.Request.Header.Get("Accept-Language"))
}

func localizeProviderMetadata(metadata *types.CommonProviderMetadata, locale string) {
	translation := metadata.Locales[locale]
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
	translation := parameter.Locales[locale]
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
		requiredDescriptions[i] = adapter.LocalizedDirectoryDescription(providerName, parameter.Name, parameter.Description, locale)
		requiredOverrides[i] = requiredDescriptions[i] != parameter.Description
	}
	optionalDescriptions := make([]string, len(manifest.OptionalConfigurationParameters))
	optionalOverrides := make([]bool, len(manifest.OptionalConfigurationParameters))
	for i := range manifest.OptionalConfigurationParameters {
		parameter := &manifest.OptionalConfigurationParameters[i]
		optionalDescriptions[i] = adapter.LocalizedDirectoryDescription(providerName, parameter.Name, parameter.Description, locale)
		optionalOverrides[i] = optionalDescriptions[i] != parameter.Description
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
