package mcp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

type optionConstraint struct {
	key         string
	value       string
	prefix      string
	required    bool
	sensitive   bool
	usage       types.Usage
	userAllowed bool
	options     []types.MCPConfigurationOption
}

func validateServerConfigurationOptions(manifest types.MCPServerManifest) error {
	if err := validateConfigurationOptions(manifest.Config, ""); err != nil {
		return err
	}
	return nil
}

func validateCatalogConfigurationOptions(manifest types.MCPServerCatalogEntryManifest, prefix string) error {
	for i, config := range manifest.Config {
		if err := validateConfigurationFieldOptions(fmt.Sprintf("%sconfig[%d]", prefix, i), config.ToHeader()); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigurationOptions(config []types.MCPConfig, prefix string) error {
	for i, field := range config {
		if field.Usage == types.Header && strings.TrimSpace(field.Key) == "" {
			return types.RuntimeValidationError{Runtime: types.RuntimeRemote, Field: fmt.Sprintf("%sconfig[%d].key", prefix, i), Message: "header key cannot be empty"}
		}
		if err := validateConfigurationFieldOptions(fmt.Sprintf("%sconfig[%d]", prefix, i), field.ToHeader()); err != nil {
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

// ManifestHasConfigurationOptions reports whether a server manifest defines catalog-owned options.
func ManifestHasConfigurationOptions(manifest types.MCPServerManifest) bool {
	return fieldsHaveConfigurationOptions(manifest.Config)
}

func fieldsHaveConfigurationOptions(config []types.MCPConfig) bool {
	for _, field := range config {
		if len(field.Options) > 0 {
			return true
		}
	}
	return false
}

// ValidateCatalogConfigurationConstraints ensures catalog-owned option fields cannot be changed on a deployed server.
func ValidateCatalogConfigurationConstraints(manifest types.MCPServerManifest, catalog types.MCPServerCatalogEntryManifest) error {
	return validateOptionConstraints("config", configOptionConstraints(manifest.Config), configOptionConstraints(catalog.Config))
}

func configOptionConstraints(fields []types.MCPConfig) []optionConstraint {
	constraints := make([]optionConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, optionConstraint{
			key:         field.Key,
			value:       field.Value,
			prefix:      field.Prefix,
			required:    field.Required,
			sensitive:   field.Sensitive,
			usage:       field.Usage,
			userAllowed: field.UserAllowed,
			options:     field.Options,
		})
	}
	return constraints
}

func validateOptionConstraints(kind string, runtimeFields, catalogFields []optionConstraint) error {
	for _, catalogField := range catalogFields {
		if len(catalogField.options) > 0 && !allMatchingOptionConstraintsMatch(runtimeFields, catalogField) {
			return fmt.Errorf("%s %q configuration must match the source catalog entry", kind, catalogField.key)
		}
	}
	for _, runtimeField := range runtimeFields {
		if len(runtimeField.options) > 0 && !allMatchingOptionConstraintsMatch(catalogFields, runtimeField) {
			return fmt.Errorf("%s %q configuration must match the source catalog entry", kind, runtimeField.key)
		}
	}
	return nil
}

func allMatchingOptionConstraintsMatch(fields []optionConstraint, expected optionConstraint) bool {
	found := false
	for _, field := range fields {
		if field.key != expected.key {
			continue
		}
		found = true
		if field.value != expected.value || field.prefix != expected.prefix || field.required != expected.required || field.sensitive != expected.sensitive || field.usage != expected.usage || field.userAllowed != expected.userAllowed || !slices.Equal(field.options, expected.options) {
			return false
		}
	}
	return found
}
