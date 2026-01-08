package validation

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var hostnameRegex = regexp.MustCompile(`^(?:\*\.)?[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)*$`)

// RuntimeValidator defines the interface for validating runtime-specific configurations
type RuntimeValidator interface {
	ValidateConfig(manifest types.MCPServerManifest) error
	ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error
}

// RuntimeValidators is a map type for storing validators by runtime type
type RuntimeValidators map[types.Runtime]RuntimeValidator

// UVXValidator implements RuntimeValidator for UVX runtime
type UVXValidator struct{}

func (v UVXValidator) ValidateConfig(manifest types.MCPServerManifest) error {
	if manifest.Runtime != types.RuntimeUVX {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected UVX runtime",
		}
	}

	if manifest.UVXConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeUVX,
			Field:   "uvxConfig",
			Message: "UVX configuration is required",
		}
	}

	return v.validateUVXConfig(*manifest.UVXConfig)
}

func (v UVXValidator) ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeUVX {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected UVX runtime",
		}
	}

	if manifest.UVXConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeUVX,
			Field:   "uvxConfig",
			Message: "UVX configuration is required",
		}
	}

	return v.validateUVXConfig(*manifest.UVXConfig)
}

func (v UVXValidator) validateUVXConfig(config types.UVXRuntimeConfig) error {
	if strings.TrimSpace(config.Package) == "" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeUVX,
			Field:   "package",
			Message: "package field cannot be empty",
		}
	}

	// Validate args format if provided
	for i, arg := range config.Args {
		if strings.TrimSpace(arg) == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "args[" + strconv.Itoa(i) + "]",
				Message: "argument cannot be empty",
			}
		}
	}

	return nil
}

// NPXValidator implements RuntimeValidator for NPX runtime
type NPXValidator struct{}

func (v NPXValidator) ValidateConfig(manifest types.MCPServerManifest) error {
	if manifest.Runtime != types.RuntimeNPX {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected NPX runtime",
		}
	}

	if manifest.NPXConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig",
			Message: "NPX configuration is required",
		}
	}

	return v.validateNPXConfig(*manifest.NPXConfig)
}

func (v NPXValidator) ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeNPX {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected NPX runtime",
		}
	}

	if manifest.NPXConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig",
			Message: "NPX configuration is required",
		}
	}

	return v.validateNPXConfig(*manifest.NPXConfig)
}

func (v NPXValidator) validateNPXConfig(config types.NPXRuntimeConfig) error {
	if strings.TrimSpace(config.Package) == "" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "package",
			Message: "package field cannot be empty",
		}
	}

	// Validate args format if provided
	for i, arg := range config.Args {
		if strings.TrimSpace(arg) == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "args[" + strconv.Itoa(i) + "]",
				Message: "argument cannot be empty",
			}
		}
	}

	return nil
}

// ContainerizedValidator implements RuntimeValidator for containerized runtime
type ContainerizedValidator struct{}

func (v ContainerizedValidator) ValidateConfig(manifest types.MCPServerManifest) error {
	if manifest.Runtime != types.RuntimeContainerized {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected containerized runtime",
		}
	}

	if manifest.ContainerizedConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "containerizedConfig",
			Message: "containerized configuration is required",
		}
	}

	return v.validateContainerizedConfig(*manifest.ContainerizedConfig)
}

func (v ContainerizedValidator) ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeContainerized {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected containerized runtime",
		}
	}

	if manifest.ContainerizedConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "containerizedConfig",
			Message: "containerized configuration is required",
		}
	}

	return v.validateContainerizedConfig(*manifest.ContainerizedConfig)
}

func (v ContainerizedValidator) validateContainerizedConfig(config types.ContainerizedRuntimeConfig) error {
	if strings.TrimSpace(config.Image) == "" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "image",
			Message: "image field cannot be empty",
		}
	}

	if config.Port <= 0 || config.Port > 65535 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "port",
			Message: "port must be between 1 and 65535",
		}
	}

	if strings.TrimSpace(config.Path) == "" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "path",
			Message: "path field cannot be empty",
		}
	}

	// Validate args format if provided
	for i, arg := range config.Args {
		if strings.TrimSpace(arg) == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "args[" + strconv.Itoa(i) + "]",
				Message: "argument cannot be empty",
			}
		}
	}

	return nil
}

// RemoteValidator implements RuntimeValidator for remote runtime
type RemoteValidator struct{}

func (v RemoteValidator) ValidateConfig(manifest types.MCPServerManifest) error {
	if manifest.Runtime != types.RuntimeRemote {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected remote runtime",
		}
	}

	if manifest.RemoteConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "remote configuration is required",
		}
	}

	return v.validateRemoteConfig(*manifest.RemoteConfig)
}

func (v RemoteValidator) ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeRemote {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected remote runtime",
		}
	}

	if manifest.RemoteConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "remote configuration is required",
		}
	}

	return v.validateRemoteCatalogConfig(*manifest.RemoteConfig)
}

func (v RemoteValidator) validateRemoteConfig(config types.RemoteRuntimeConfig) error {
	if strings.TrimSpace(config.URL) == "" {
		if config.IsTemplate {
			return nil
		}
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "url",
			Message: "URL field cannot be empty",
		}
	}

	// Validate URL format
	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "url",
			Message: fmt.Sprintf("invalid URL format: %v", err),
		}
	}

	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "url",
			Message: "URL scheme must be either https or http",
		}
	}

	// Validate headers
	for i, header := range config.Headers {
		if strings.TrimSpace(header.Key) == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   fmt.Sprintf("header[%d].key", i),
				Message: "header key cannot be empty",
			}
		}
		if header.Value != "" && header.Sensitive {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   fmt.Sprintf("header[%d]", i),
				Message: "static header value cannot be marked as sensitive",
			}
		}
	}

	return nil
}

func (v RemoteValidator) validateRemoteCatalogConfig(config types.RemoteCatalogConfig) error {
	// Either FixedURL, Hostname, or URLTemplate must be provided, but only one
	hasFixedURL := strings.TrimSpace(config.FixedURL) != ""
	hasHostname := strings.TrimSpace(config.Hostname) != ""
	hasURLTemplate := strings.TrimSpace(config.URLTemplate) != ""

	if !hasFixedURL && !hasHostname && !hasURLTemplate {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "either fixedURL, hostname, or urlTemplate must be provided",
		}
	}

	// Count how many fields are set
	fieldCount := 0
	if hasFixedURL {
		fieldCount++
	}
	if hasHostname {
		fieldCount++
	}
	if hasURLTemplate {
		fieldCount++
	}

	if fieldCount > 1 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "cannot specify multiple URL configuration methods (fixedURL, hostname, or urlTemplate)",
		}
	}

	// Validate FixedURL format if provided
	if hasFixedURL {
		parsedURL, err := url.Parse(config.FixedURL)
		if err != nil {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "fixedURL",
				Message: fmt.Sprintf("invalid URL format: %v", err),
			}
		}

		if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "fixedURL",
				Message: "URL scheme must be either https or http",
			}
		}
	}

	// Validate hostname format if provided
	if hasHostname {
		// Basic hostname validation.
		// A wildcard prefix of *. is allowed.
		if !hostnameRegex.MatchString(config.Hostname) {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "hostname",
				Message: "hostname should only contain alphanumeric and hyphens",
			}
		}
	}

	for i, header := range config.Headers {
		if strings.TrimSpace(header.Key) == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   fmt.Sprintf("header[%d].key", i),
				Message: "header key cannot be empty",
			}
		}
		if header.Value != "" && header.Sensitive {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   fmt.Sprintf("header[%d]", i),
				Message: "static header value cannot be marked as sensitive",
			}
		}
	}

	return nil
}

// CompositeValidator implements RuntimeValidator for composite runtime
type CompositeValidator struct{}

func (v CompositeValidator) ValidateConfig(manifest types.MCPServerManifest) error {
	if manifest.Runtime != types.RuntimeComposite {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected composite runtime",
		}
	}

	if manifest.CompositeConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeComposite,
			Field:   "compositeConfig",
			Message: "composite configuration is required",
		}
	}

	if len(manifest.CompositeConfig.ComponentServers) < 1 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeComposite,
			Field:   "compositeConfig.componentServers",
			Message: "must contain at least one component server",
		}
	}

	// Check for duplicate component servers
	componentServerIDs := make(map[string]struct{}, len(manifest.CompositeConfig.ComponentServers))
	for i, component := range manifest.CompositeConfig.ComponentServers {
		// Ensure exactly one of CatalogEntryID or MCPServerID is set
		hasCatalogEntry, hasServerID := component.CatalogEntryID != "", component.MCPServerID != ""
		if (!hasCatalogEntry && !hasServerID) || (hasCatalogEntry && hasServerID) {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "must have one of catalogEntryID or mcpServerID set",
			}
		}

		// Prevent composite MCP servers from being nested
		if component.Manifest.Runtime == types.RuntimeComposite {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d].manifest.runtime", i),
				Message: "runtime cannot be composite",
			}
		}

		// Prevent remote components with static OAuth from being included in composites
		if component.Manifest.Runtime == types.RuntimeRemote &&
			component.Manifest.RemoteConfig != nil &&
			component.Manifest.RemoteConfig.StaticOAuthRequired {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "remote component with static OAuth cannot be included in a composite server",
			}
		}

		// Validate tool overrides
		if err := validateToolOverrides(component.ToolOverrides); err != nil {
			return errors.Join(types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "tool overrides invalid",
			}, err)
		}

		componentID := component.ComponentID()
		if _, ok := componentServerIDs[componentID]; ok {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: fmt.Sprintf("duplicate component server: %s", componentID),
			}
		}
		componentServerIDs[componentID] = struct{}{}
	}

	return nil
}

func (v CompositeValidator) ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeComposite {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected composite runtime",
		}
	}

	if manifest.CompositeConfig == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeComposite,
			Field:   "compositeConfig",
			Message: "composite configuration is required",
		}
	}

	if len(manifest.CompositeConfig.ComponentServers) < 1 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeComposite,
			Field:   "compositeConfig.componentServers",
			Message: "must contain at least one component server",
		}
	}

	// Check for duplicate component servers
	componentServerIDs := make(map[string]struct{}, len(manifest.CompositeConfig.ComponentServers))
	for i, component := range manifest.CompositeConfig.ComponentServers {
		// Ensure exactly one of CatalogEntryID or MCPServerID is set
		hasCatalogEntry, hasServerID := component.CatalogEntryID != "", component.MCPServerID != ""
		if (!hasCatalogEntry && !hasServerID) || (hasCatalogEntry && hasServerID) {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "must have one of catalogEntryID or mcpServerID set",
			}
		}

		// Prevent composite MCP servers from being nested
		if component.Manifest.Runtime == types.RuntimeComposite {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d].manifest.runtime", i),
				Message: "runtime cannot be composite",
			}
		}

		// Prevent remote components with static OAuth from being included in composites
		if component.Manifest.Runtime == types.RuntimeRemote &&
			component.Manifest.RemoteConfig != nil &&
			component.Manifest.RemoteConfig.StaticOAuthRequired {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "remote component with static OAuth cannot be included in a composite server",
			}
		}

		// Validate tool overrides
		if err := validateToolOverrides(component.ToolOverrides); err != nil {
			return errors.Join(types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: "tool overrides invalid",
			}, err)
		}

		componentID := component.ComponentID()
		if _, ok := componentServerIDs[componentID]; ok {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
				Message: fmt.Sprintf("duplicate component server: %s", componentID),
			}
		}
		componentServerIDs[componentID] = struct{}{}
	}

	return nil
}

func validateToolOverrides(overrides []types.ToolOverride) error {
	var (
		toolNames    = make(map[string]struct{}, len(overrides))
		exposedNames = make(map[string]struct{}, len(overrides))
	)

	for i, override := range overrides {
		if override.Name == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("toolOverrides[%d].name", i),
				Message: "original tool name is required",
			}
		}

		// Check for duplicate original names
		if _, ok := toolNames[override.Name]; ok {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("toolOverrides[%d].name", i),
				Message: fmt.Sprintf("duplicate tool name: %s", override.Name),
			}
		}
		toolNames[override.Name] = struct{}{}

		// For disabled tools, we don't care about exposed-name conflicts.
		if !override.Enabled {
			continue
		}

		// Compute the effective exposed name: override name if set, otherwise the original name.
		effectiveName := override.OverrideName
		if effectiveName == "" {
			effectiveName = override.Name
		}

		// Check for duplicate exposed names among enabled tools.
		if _, ok := exposedNames[effectiveName]; ok {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   fmt.Sprintf("toolOverrides[%d].overrideName", i),
				Message: fmt.Sprintf("duplicate override name: %s", effectiveName),
			}
		}
		exposedNames[effectiveName] = struct{}{}
	}

	return nil
}

// getRuntimeValidators returns a map of all available runtime validators
func getRuntimeValidators() RuntimeValidators {
	return RuntimeValidators{
		types.RuntimeUVX:           UVXValidator{},
		types.RuntimeNPX:           NPXValidator{},
		types.RuntimeContainerized: ContainerizedValidator{},
		types.RuntimeRemote:        RemoteValidator{},
		types.RuntimeComposite:     CompositeValidator{},
	}
}

func ValidateServerManifest(manifest types.MCPServerManifest) error {
	if validator, ok := getRuntimeValidators()[manifest.Runtime]; ok {
		return validator.ValidateConfig(manifest)
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}

func ValidateCatalogEntryManifest(manifest types.MCPServerCatalogEntryManifest) error {
	if validator, ok := getRuntimeValidators()[manifest.Runtime]; ok {
		return validator.ValidateCatalogConfig(manifest)
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}
