package validation

import (
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

	return nil
}

func (v RemoteValidator) validateRemoteCatalogConfig(config types.RemoteCatalogConfig) error {
	// Either FixedURL or Hostname must be provided, but not both
	hasFixedURL := strings.TrimSpace(config.FixedURL) != ""
	hasHostname := strings.TrimSpace(config.Hostname) != ""

	if !hasFixedURL && !hasHostname {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "either fixedURL or hostname must be provided",
		}
	}

	if hasFixedURL && hasHostname {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "cannot specify both fixedURL and hostname",
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

	return nil
}

// getRuntimeValidators returns a map of all available runtime validators
func getRuntimeValidators() RuntimeValidators {
	return RuntimeValidators{
		types.RuntimeUVX:           UVXValidator{},
		types.RuntimeNPX:           NPXValidator{},
		types.RuntimeContainerized: ContainerizedValidator{},
		types.RuntimeRemote:        RemoteValidator{},
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
