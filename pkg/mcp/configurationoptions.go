package mcp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

func validateServerConfigurationOptions(manifest types.MCPServerManifest) error {
	if err := validateConfigurationOptions(manifest.Config); err != nil {
		return err
	}
	return nil
}

func validateCatalogConfigurationOptions(manifest types.MCPServerCatalogEntryManifest) error {
	for i, config := range manifest.Config {
		if err := validateConfigurationFieldOptions(fmt.Sprintf("config[%d]", i), config.ToHeader()); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigurationOptions(config []types.MCPConfig) error {
	for i, field := range config {
		if field.Usage == types.Header && strings.TrimSpace(field.Key) == "" {
			return types.RuntimeValidationError{Runtime: types.RuntimeRemote, Field: fmt.Sprintf("config[%d].key", i), Message: "header key cannot be empty"}
		}
		if err := validateConfigurationFieldOptions(fmt.Sprintf("config[%d]", i), field.ToHeader()); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigurationFieldOptions(field string, config types.MCPHeader) error {
	if len(config.Options) == 0 {
		return nil
	}
	if config.Value != "" {
		return fmt.Errorf("%s.value and options are mutually exclusive", field)
	}
	if config.SecretBinding != nil {
		return fmt.Errorf("%s.secretBinding and options are mutually exclusive", field)
	}

	values := make(map[string]struct{}, len(config.Options))
	for i, option := range config.Options {
		if strings.TrimSpace(option.Name) == "" {
			return fmt.Errorf("%s.options[%d].name cannot be empty", field, i)
		}
		if strings.TrimSpace(option.Value) == "" {
			return fmt.Errorf("%s.options[%d].value cannot be empty", field, i)
		}
		if _, exists := values[option.Value]; exists {
			return fmt.Errorf("%s.options contains duplicate value %q", field, option.Value)
		}
		values[option.Value] = struct{}{}
	}
	return nil
}

// ValidateConfiguredOptions returns missing required selections and rejects values outside catalog-owned options.
// It does not return an error when returning missing configs.
func ValidateConfiguredOptions(config []types.MCPConfig, values map[string]string) ([]string, error) {
	var missing []string
	for _, field := range config {
		fieldMissing, err := validateConfiguredOption(string(field.Usage), field.ToHeader(), values)
		if err != nil {
			return nil, err
		}
		if fieldMissing {
			missing = append(missing, field.Key)
		}
	}
	return missing, nil
}

func validateConfiguredOption(kind string, field types.MCPHeader, values map[string]string) (bool, error) {
	if len(field.Options) == 0 {
		return false, nil
	}
	value := values[field.Key]
	if value == "" {
		return field.Required, nil
	}
	if slices.ContainsFunc(field.Options, func(option types.MCPConfigurationOption) bool {
		return option.Value == value
	}) {
		return false, nil
	}
	return false, fmt.Errorf("%s %q value %q is not one of the configured options", kind, field.Key, value)
}

// ConfigurationOptionValueValid reports whether a field's configured selection is allowed.
func ConfigurationOptionValueValid(field types.MCPHeader, values map[string]string) bool {
	missing, err := validateConfiguredOption("configuration", field, values)
	return !missing && err == nil
}
