package mcp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

func remoteHeaders(config *types.RemoteRuntimeConfig) []types.MCPHeader {
	if config == nil {
		return nil
	}
	return config.Headers
}

func remoteCatalogHeaders(config *types.RemoteCatalogConfig) []types.MCPHeader {
	if config == nil {
		return nil
	}
	return config.Headers
}

func multiUserHeaders(config *types.MultiUserConfig) []types.MCPHeader {
	if config == nil {
		return nil
	}
	return config.UserDefinedHeaders
}

func validateServerConfigurationOptions(manifest types.MCPServerManifest) error {
	if err := validateConfigurationOptions(manifest.Env, remoteHeaders(manifest.RemoteConfig), multiUserHeaders(manifest.MultiUserConfig), ""); err != nil {
		return err
	}
	if manifest.CompositeConfig != nil {
		for i, component := range manifest.CompositeConfig.ComponentServers {
			if err := validateServerConfigurationOptions(component.Manifest); err != nil {
				return fmt.Errorf("compositeConfig.componentServers[%d].manifest: %w", i, err)
			}
		}
	}
	return nil
}

func validateCatalogConfigurationOptions(manifest types.MCPServerCatalogEntryManifest, prefix string) error {
	if err := validateConfigurationOptions(manifest.Env, remoteCatalogHeaders(manifest.RemoteConfig), multiUserHeaders(manifest.MultiUserConfig), prefix); err != nil {
		return err
	}
	if manifest.CompositeConfig != nil {
		for i, component := range manifest.CompositeConfig.ComponentServers {
			componentPrefix := fmt.Sprintf("%scompositeConfig.componentServers[%d].manifest.", prefix, i)
			if err := validateCatalogConfigurationOptions(component.Manifest, componentPrefix); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConfigurationOptions(envs []types.MCPEnv, remote, multiUser []types.MCPHeader, prefix string) error {
	for i, env := range envs {
		if err := validateConfigurationFieldOptions(fmt.Sprintf("%senv[%d]", prefix, i), env.MCPHeader); err != nil {
			return err
		}
	}
	for i, header := range remote {
		if err := validateConfigurationFieldOptions(fmt.Sprintf("%sremoteConfig.headers[%d]", prefix, i), header); err != nil {
			return err
		}
	}
	for i, header := range multiUser {
		if err := validateConfigurationFieldOptions(fmt.Sprintf("%smultiUserConfig.userDefinedHeaders[%d]", prefix, i), header); err != nil {
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
func ValidateConfiguredOptions(envs []types.MCPEnv, headers []types.MCPHeader, values map[string]string) ([]string, error) {
	var missing []string
	for _, env := range envs {
		fieldMissing, err := validateConfiguredOption("env", env.MCPHeader, values)
		if err != nil {
			return nil, err
		}
		if fieldMissing {
			missing = append(missing, env.Key)
		}
	}
	for _, header := range headers {
		fieldMissing, err := validateConfiguredOption("header", header, values)
		if err != nil {
			return nil, err
		}
		if fieldMissing {
			missing = append(missing, header.Key)
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
	if fieldsHaveConfigurationOptions(manifest.Env, remoteHeaders(manifest.RemoteConfig), multiUserHeaders(manifest.MultiUserConfig)) {
		return true
	}
	if manifest.CompositeConfig != nil {
		for _, component := range manifest.CompositeConfig.ComponentServers {
			if ManifestHasConfigurationOptions(component.Manifest) {
				return true
			}
		}
	}
	return false
}

func catalogManifestHasConfigurationOptions(manifest types.MCPServerCatalogEntryManifest) bool {
	if fieldsHaveConfigurationOptions(manifest.Env, remoteCatalogHeaders(manifest.RemoteConfig), multiUserHeaders(manifest.MultiUserConfig)) {
		return true
	}
	if manifest.CompositeConfig != nil {
		for _, component := range manifest.CompositeConfig.ComponentServers {
			if catalogManifestHasConfigurationOptions(component.Manifest) {
				return true
			}
		}
	}
	return false
}

func fieldsHaveConfigurationOptions(envs []types.MCPEnv, headerGroups ...[]types.MCPHeader) bool {
	for _, env := range envs {
		if len(env.Options) > 0 {
			return true
		}
	}
	for _, headers := range headerGroups {
		for _, header := range headers {
			if len(header.Options) > 0 {
				return true
			}
		}
	}
	return false
}

type optionConstraint struct {
	key         string
	value       string
	prefix      string
	required    bool
	sensitive   bool
	file        bool
	dynamicFile bool
	options     []types.MCPConfigurationOption
}

// ValidateCatalogConfigurationConstraints ensures catalog-owned option fields cannot be changed on a deployed server.
func ValidateCatalogConfigurationConstraints(manifest types.MCPServerManifest, catalog types.MCPServerCatalogEntryManifest) error {
	if err := validateOptionConstraints("env", envOptionConstraints(manifest.Env), envOptionConstraints(catalog.Env)); err != nil {
		return err
	}
	if err := validateOptionConstraints("header", headerOptionConstraints(remoteHeaders(manifest.RemoteConfig)), headerOptionConstraints(remoteCatalogHeaders(catalog.RemoteConfig))); err != nil {
		return err
	}
	if err := validateOptionConstraints("multi-user header", headerOptionConstraints(multiUserHeaders(manifest.MultiUserConfig)), headerOptionConstraints(multiUserHeaders(catalog.MultiUserConfig))); err != nil {
		return err
	}

	var runtimeComponents []types.ComponentServer
	if manifest.CompositeConfig != nil {
		runtimeComponents = manifest.CompositeConfig.ComponentServers
	}
	var catalogComponents []types.CatalogComponentServer
	if catalog.CompositeConfig != nil {
		catalogComponents = catalog.CompositeConfig.ComponentServers
	}

	runtimeByID := make(map[string]types.ComponentServer, len(runtimeComponents))
	for _, component := range runtimeComponents {
		runtimeByID[component.ComponentID()] = component
	}
	catalogByID := make(map[string]types.CatalogComponentServer, len(catalogComponents))
	for _, component := range catalogComponents {
		catalogByID[component.ComponentID()] = component
		runtimeComponent, ok := runtimeByID[component.ComponentID()]
		if !ok {
			if catalogManifestHasConfigurationOptions(component.Manifest) {
				return fmt.Errorf("component %q catalog-owned configuration options are missing", component.ComponentID())
			}
			continue
		}
		if err := ValidateCatalogConfigurationConstraints(runtimeComponent.Manifest, component.Manifest); err != nil {
			return fmt.Errorf("component %q: %w", component.ComponentID(), err)
		}
	}
	for _, component := range runtimeComponents {
		if _, ok := catalogByID[component.ComponentID()]; !ok && ManifestHasConfigurationOptions(component.Manifest) {
			return fmt.Errorf("component %q configuration options must be defined by the source catalog entry", component.ComponentID())
		}
	}
	return nil
}

func envOptionConstraints(fields []types.MCPEnv) []optionConstraint {
	constraints := make([]optionConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, optionConstraint{
			key:         field.Key,
			value:       field.Value,
			prefix:      field.Prefix,
			required:    field.Required,
			sensitive:   field.Sensitive,
			file:        field.File,
			dynamicFile: field.DynamicFile,
			options:     field.Options,
		})
	}
	return constraints
}

func headerOptionConstraints(fields []types.MCPHeader) []optionConstraint {
	constraints := make([]optionConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, optionConstraint{
			key:       field.Key,
			value:     field.Value,
			prefix:    field.Prefix,
			required:  field.Required,
			sensitive: field.Sensitive,
			options:   field.Options,
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
		if field.value != expected.value || field.prefix != expected.prefix || field.required != expected.required || field.sensitive != expected.sensitive || field.file != expected.file || field.dynamicFile != expected.dynamicFile || !slices.Equal(field.options, expected.options) {
			return false
		}
	}
	return found
}
