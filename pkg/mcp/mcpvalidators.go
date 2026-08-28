package mcp

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// maxToolNameLength is the max length of an MCP server tool.
	// It's used to validate effective tool names after tool overrides and prefixes are applied.
	maxToolNameLength = 128

	// maxToolPrefixLength is the max length of a composite component tool prefix.
	maxToolPrefixLength = 64

	// maxShortDescriptionLength is the max length of a catalog entry shortDescription.
	maxShortDescriptionLength = 160
)

var (
	// toolNameRegex matches the character set allowed for composite
	// component tools: ASCII letters, digits, underscore, hyphen, dot,
	// and forward slash. Note that '.' and '/' produce a soft warning downstream
	// (some MCP clients reject them) but are permitted here so admins who know
	// their clients can use them.
	toolNameRegex = regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
	hostnameRegex = regexp.MustCompile(`^(?:\*\.)?[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	// envVarRefRegex matches ${VAR} references inside command/args/URL templates.
	envVarRefRegex = regexp.MustCompile(`\${([^}]+)}`)
)

// RuntimeValidator defines the interface for validating runtime-specific configurations
type RuntimeValidator interface {
	ValidateConfig(ctx context.Context, manifest types.MCPServerManifest) error
	ValidateCatalogConfig(ctx context.Context, manifest types.MCPServerCatalogEntryManifest) error
	ValidateSystemConfig(ctx context.Context, manifest types.SystemMCPServerManifest) error
}

// RuntimeValidators is a map type for storing validators by runtime type
type RuntimeValidators map[types.Runtime]RuntimeValidator

// Options configures runtime validation behavior.
type ValidationOptions struct {
	AllowMissingURL              bool
	RemoteMCPURLValidationConfig RemoteMCPURLValidationConfig
	ResourceMaximums             ResourceMaximums
}

// UVXValidator implements RuntimeValidator for UVX runtime
type UVXValidator struct{}

// NPXValidator implements RuntimeValidator for NPX runtime
type NPXValidator struct{}

// ContainerizedValidator implements RuntimeValidator for containerized runtime
type ContainerizedValidator struct{}

// RemoteValidator implements RuntimeValidator for remote runtime
type RemoteValidator struct {
	AllowMissingURL              bool
	RemoteMCPURLValidationConfig RemoteMCPURLValidationConfig
}

// CompositeValidator implements RuntimeValidator for composite runtime
type CompositeValidator struct{}

func validateEgressDomains(runtime types.Runtime, domains []string, denyAllEgress *bool) error {
	if denyAllEgress != nil && *denyAllEgress && len(domains) > 0 {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   "denyAllEgress",
			Message: "denyAllEgress cannot be true when egressDomains are specified",
		}
	}

	for i, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain cannot be empty",
			}
		}

		if strings.Contains(domain, "://") {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain must not include a protocol",
			}
		}

		if net.ParseIP(domain) != nil {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain must not be an IP address",
			}
		}

		if strings.ContainsAny(domain, "/:") {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain must not include a path or port",
			}
		}

		if !hostnameRegex.MatchString(domain) {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain must be a valid hostname or leading wildcard hostname",
			}
		}

		hostname := strings.TrimPrefix(strings.ToLower(domain), "*.")
		labels := strings.Split(hostname, ".")
		if len(labels) < 2 {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain must contain at least two DNS labels",
			}
		}

		if isDeniedEgressDomain(hostname) {
			return types.RuntimeValidationError{
				Runtime: runtime,
				Field:   fmt.Sprintf("egressDomains[%d]", i),
				Message: "egress domain is not allowed",
			}
		}
	}

	return nil
}

func isDeniedEgressDomain(hostname string) bool {
	switch hostname {
	case "localhost", "metadata.google.internal", "cluster.local":
		return true
	}

	for _, suffix := range []string{
		".localhost",
		".cluster.local",
		".svc",
		".in-addr.arpa",
		".ip6.arpa",
	} {
		if strings.HasSuffix(hostname, suffix) {
			return true
		}
	}

	return false
}

func (v UVXValidator) ValidateConfig(_ context.Context, manifest types.MCPServerManifest) error {
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

func (v UVXValidator) ValidateCatalogConfig(_ context.Context, manifest types.MCPServerCatalogEntryManifest) error {
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

func (v UVXValidator) ValidateSystemConfig(_ context.Context, manifest types.SystemMCPServerManifest) error {
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

	if err := validateEgressDomains(types.RuntimeUVX, config.EgressDomains, config.DenyAllEgress); err != nil {
		return err
	}
	if err := validateStartupTimeout(types.RuntimeUVX, "uvxConfig.startupTimeoutSeconds", config.StartupTimeoutSeconds); err != nil {
		return err
	}

	return nil
}

func (v NPXValidator) ValidateConfig(_ context.Context, manifest types.MCPServerManifest) error {
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

func (v NPXValidator) ValidateCatalogConfig(_ context.Context, manifest types.MCPServerCatalogEntryManifest) error {
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

func (v NPXValidator) ValidateSystemConfig(_ context.Context, manifest types.SystemMCPServerManifest) error {
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

	if err := validateEgressDomains(types.RuntimeNPX, config.EgressDomains, config.DenyAllEgress); err != nil {
		return err
	}
	if err := validateStartupTimeout(types.RuntimeNPX, "npxConfig.startupTimeoutSeconds", config.StartupTimeoutSeconds); err != nil {
		return err
	}

	return nil
}

func (v ContainerizedValidator) ValidateConfig(_ context.Context, manifest types.MCPServerManifest) error {
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

func (v ContainerizedValidator) ValidateCatalogConfig(_ context.Context, manifest types.MCPServerCatalogEntryManifest) error {
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

func (v ContainerizedValidator) ValidateSystemConfig(_ context.Context, manifest types.SystemMCPServerManifest) error {
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

	if err := validateEgressDomains(types.RuntimeContainerized, config.EgressDomains, config.DenyAllEgress); err != nil {
		return err
	}
	if err := validateStartupTimeout(types.RuntimeContainerized, "containerizedConfig.startupTimeoutSeconds", config.StartupTimeoutSeconds); err != nil {
		return err
	}

	return nil
}

func (v RemoteValidator) ValidateConfig(ctx context.Context, manifest types.MCPServerManifest) error {
	if err := validateConfigurationOptions(manifest.Config, ""); err != nil {
		return err
	}
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

	return v.validateRemoteConfig(ctx, *manifest.RemoteConfig)
}

func (v RemoteValidator) ValidateCatalogConfig(ctx context.Context, manifest types.MCPServerCatalogEntryManifest) error {
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

	return v.validateRemoteCatalogConfig(ctx, *manifest.RemoteConfig)
}

func (v RemoteValidator) ValidateSystemConfig(ctx context.Context, manifest types.SystemMCPServerManifest) error {
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

	return v.validateRemoteConfig(ctx, *manifest.RemoteConfig)
}

func (v RemoteValidator) validateRemoteConfig(ctx context.Context, config types.RemoteRuntimeConfig) error {
	config.TunnelName = strings.TrimSpace(config.TunnelName)
	if config.TunnelName != "" &&
		(config.IsTemplate || strings.TrimSpace(config.URLTemplate) != "" || len(extractEnvRefs(config.URL)) > 0) {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "tunnelName cannot be used with a URL template",
		}
	}

	config.URL = strings.TrimSpace(config.URL)
	if config.URL == "" {
		if !v.AllowMissingURL && !config.IsTemplate {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "url",
				Message: "URL field cannot be empty",
			}
		}
	} else {
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
		if err := v.validateRemoteMCPURL(ctx, "url", config.URL, config.TunnelName != ""); err != nil {
			return err
		}
	}

	return nil
}

func (v RemoteValidator) validateRemoteCatalogConfig(ctx context.Context, config types.RemoteCatalogConfig) error {
	// Either FixedURL, Hostname, or URLTemplate must be provided, but only one
	hasFixedURL := strings.TrimSpace(config.FixedURL) != ""
	hasHostname := strings.TrimSpace(config.Hostname) != ""
	hasURLTemplate := strings.TrimSpace(config.URLTemplate) != ""
	hasTunnel := strings.TrimSpace(config.TunnelName) != ""

	if hasTunnel && hasURLTemplate {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "tunnelName cannot be used with urlTemplate",
		}
	}
	if hasTunnel && len(extractEnvRefs(config.FixedURL)) > 0 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "remoteConfig",
			Message: "tunnelName cannot be used with a URL template",
		}
	}

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
		if err := validateRemoteURLFormat("fixedURL", config.FixedURL); err != nil {
			return err
		}
		if err := v.validateRemoteMCPURL(ctx, "fixedURL", config.FixedURL, hasTunnel); err != nil {
			return err
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

func validateRemoteURLFormat(field, rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   field,
			Message: fmt.Sprintf("invalid URL format: %v", err),
		}
	}

	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   field,
			Message: "URL scheme must be either https or http",
		}
	}

	return nil
}

func (v RemoteValidator) validateRemoteMCPURL(ctx context.Context, field, rawURL string, tunneled bool) error {
	if tunneled {
		// The tunnel client resolves and requests the target, so Obot's local
		// network restrictions do not apply. The referenced MCPTunnel's
		// AllowedURLs are validated separately against the target.
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   field,
				Message: fmt.Sprintf("invalid URL format: %v", err),
			}
		}
		if parsedURL.Hostname() == "" {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   field,
				Message: "URL hostname is required",
			}
		}
		if parsedURL.User != nil {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   field,
				Message: "URL must not include user information",
			}
		}
		return nil
	}
	if err := ValidateRemoteMCPURL(ctx, rawURL, v.RemoteMCPURLValidationConfig); err != nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   field,
			Message: err.Error(),
		}
	}
	return nil
}

func (v CompositeValidator) ValidateConfig(_ context.Context, manifest types.MCPServerManifest) error {
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

	numComponents := len(manifest.CompositeConfig.ComponentServers)
	if numComponents < 1 {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeComposite,
			Field:   "compositeConfig.componentServers",
			Message: "must contain at least one component server",
		}
	}

	var (
		componentServerIDs = make(map[string]struct{}, numComponents)
		toolPrefixes       = make(map[string]struct{}, numComponents)
		effectiveToolNames = make(map[string]struct{})
	)
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

		// Validate the tool prefix
		prefix := component.ToolPrefix
		if prefix != "" {
			// Prevent duplicates
			if _, ok := toolPrefixes[prefix]; ok {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolPrefix", i),
					Message: fmt.Sprintf("duplicate toolPrefix: %s", prefix),
				}
			}
			toolPrefixes[prefix] = struct{}{}

			// Ensure the prefix is valid separately
			if !toolNameRegex.MatchString(prefix) {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolPrefix", i),
					Message: "toolPrefix must match " + toolNameRegex.String(),
				}
			}
			if len(prefix) > maxToolPrefixLength {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolPrefix", i),
					Message: fmt.Sprintf("toolPrefix must be at most %d characters", maxToolPrefixLength),
				}
			}
		}

		// Validate tool overrides
		for j, override := range component.ToolOverrides {
			if override.Name == "" {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolOverrides[%d].name", i, j),
					Message: "original tool name is required",
				}
			}

			// For disabled tools, we don't care about validating the effective tool names
			if !override.Enabled {
				continue
			}

			// Compute the effective tool name
			effectiveToolName := prefix + cmp.Or(override.OverrideName, override.Name)

			// Validate length
			if len(effectiveToolName) > maxToolNameLength {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolOverrides[%d]", i, j),
					Message: fmt.Sprintf("effective tool name must be at most %d characters: %q", maxToolNameLength, effectiveToolName),
				}
			}

			// Validate character set
			if !toolNameRegex.MatchString(effectiveToolName) {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolOverrides[%d]", i, j),
					Message: "effective tool name must match " + toolNameRegex.String(),
				}
			}

			// Prevent effective duplicates (across entire composite)
			if _, ok := effectiveToolNames[effectiveToolName]; ok {
				return types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d].toolOverrides[%d]", i, j),
					Message: fmt.Sprintf("duplicate tool name: %s", effectiveToolName),
				}
			}
			effectiveToolNames[effectiveToolName] = struct{}{}
		}

		// Prevent duplicate component servers
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

func (v CompositeValidator) ValidateCatalogConfig(_ context.Context, manifest types.MCPServerCatalogEntryManifest) error {
	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "composite runtime is not supported for catalog entries",
	}
}

func (v CompositeValidator) ValidateSystemConfig(_ context.Context, manifest types.SystemMCPServerManifest) error {
	if manifest.Runtime != types.RuntimeComposite {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: "expected composite runtime",
		}
	}

	return types.RuntimeValidationError{
		Runtime: types.RuntimeComposite,
		Field:   "runtime",
		Message: "composite runtime is not supported for system servers",
	}
}

// getRuntimeValidators returns a map of all available runtime validators
func getRuntimeValidators(options ValidationOptions) RuntimeValidators {
	return RuntimeValidators{
		types.RuntimeUVX:           UVXValidator{},
		types.RuntimeNPX:           NPXValidator{},
		types.RuntimeContainerized: ContainerizedValidator{},
		types.RuntimeRemote: RemoteValidator{
			RemoteMCPURLValidationConfig: options.RemoteMCPURLValidationConfig,
			AllowMissingURL:              options.AllowMissingURL,
		},
		types.RuntimeComposite: CompositeValidator{},
	}
}

func validateMCPResourceRequirements(runtime types.Runtime, resources *types.MCPResourceRequirements) error {
	if resources == nil {
		return nil
	}

	parse := func(field, value string) (*resource.Quantity, error) {
		if value == "" {
			return nil, nil
		}
		quantity, err := resource.ParseQuantity(value)
		if err != nil {
			return nil, types.RuntimeValidationError{
				Runtime: runtime,
				Field:   field,
				Message: fmt.Sprintf("invalid quantity %q: %v", value, err),
			}
		}
		if quantity.Sign() < 0 {
			return nil, types.RuntimeValidationError{
				Runtime: runtime,
				Field:   field,
				Message: fmt.Sprintf("must be non-negative, got %q", value),
			}
		}
		return &quantity, nil
	}

	requestCPU, err := parse("resources.requests.cpu", resources.Requests.CPU)
	if err != nil {
		return err
	}
	requestMemory, err := parse("resources.requests.memory", resources.Requests.Memory)
	if err != nil {
		return err
	}
	limitCPU, err := parse("resources.limits.cpu", resources.Limits.CPU)
	if err != nil {
		return err
	}
	limitMemory, err := parse("resources.limits.memory", resources.Limits.Memory)
	if err != nil {
		return err
	}

	if requestCPU != nil && limitCPU != nil && limitCPU.Cmp(*requestCPU) < 0 {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   "resources.limits.cpu",
			Message: "must be greater than or equal to resources.requests.cpu",
		}
	}
	if requestMemory != nil && limitMemory != nil && limitMemory.Cmp(*requestMemory) < 0 {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   "resources.limits.memory",
			Message: "must be greater than or equal to resources.requests.memory",
		}
	}

	return nil
}

func validateMCPResourceMaximums(resources *types.MCPResourceRequirements, maximums ResourceMaximums) error {
	if maximums.Empty() || resources == nil {
		return nil
	}

	coreResources, err := CoreResourceRequirements(resources)
	if err != nil {
		return err
	}
	if coreResources == nil {
		return nil
	}

	return maximums.Validate(*coreResources)
}

// validateCompositeServerResourceMaximums validates the resource maximums for a composite server.
// No-op if the server is not a composite server.
func validateCompositeServerResourceMaximums(manifest types.MCPServerManifest, maximums ResourceMaximums) error {
	if maximums.Empty() || manifest.CompositeConfig == nil {
		return nil
	}

	for _, component := range manifest.CompositeConfig.ComponentServers {
		if err := validateMCPResourceMaximums(component.Manifest.Resources, maximums); err != nil {
			return err
		}
	}
	return nil
}

func ValidateServerManifest(ctx context.Context, manifest types.MCPServerManifest, isMultiUser bool, options ValidationOptions) error {
	if err := validateServerConfigurationOptions(manifest); err != nil {
		return err
	}
	if err := manifest.ValidateConfig(); err != nil {
		return err
	}
	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}
	if err := validateMCPResourceMaximums(manifest.Resources, options.ResourceMaximums); err != nil {
		return err
	}

	for _, config := range manifest.Config {
		if config.UserAllowed && (!isMultiUser || config.Usage != types.Header) {
			return types.RuntimeValidationError{
				Runtime: manifest.Runtime,
				Field:   "config",
				Message: "userAllowed may only be set for multi-user headers",
			}
		}
	}
	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if err := validateCompositeServerResourceMaximums(manifest, options.ResourceMaximums); err != nil {
		return err
	}

	if validator, ok := getRuntimeValidators(options)[manifest.Runtime]; ok {
		return validator.ValidateConfig(ctx, manifest)
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}

// ValidateCatalogEntryForRoute checks that a catalog entry is compatible with the
// route used to create a server. catalogID and workspaceID come from the URL path.
func ValidateCatalogEntryForRoute(manifest types.MCPServerCatalogEntryManifest, catalogID, workspaceID string) error {
	_ = manifest
	_ = catalogID
	_ = workspaceID
	return nil
}

func ValidateCatalogEntryManifest(ctx context.Context, manifest types.MCPServerCatalogEntryManifest, gitManaged bool, options ValidationOptions) error {
	if err := manifest.ValidateConfig(); err != nil {
		return err
	}
	if err := validateCatalogConfigurationOptions(manifest, ""); err != nil {
		return err
	}
	if utf8.RuneCountInString(manifest.ShortDescription) > maxShortDescriptionLength {
		return fmt.Errorf("short description must be less than or equal to %d characters", maxShortDescriptionLength)
	}

	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}
	if err := validateMCPResourceMaximums(manifest.Resources, options.ResourceMaximums); err != nil {
		return err
	}

	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if gitManaged {
		if err := validateGitManagedCatalogEntryManifest(manifest); err != nil {
			return err
		}
	}

	if validator, ok := getRuntimeValidators(options)[manifest.Runtime]; ok {
		return validator.ValidateCatalogConfig(ctx, manifest)
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}

func validateGitManagedCatalogEntryManifest(manifest types.MCPServerCatalogEntryManifest) error {
	if err := validateCatalogSyncedTunnelName(manifest, ""); err != nil {
		return err
	}

	return nil
}

func validateCatalogSyncedTunnelName(manifest types.MCPServerCatalogEntryManifest, fieldPrefix string) error {
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.TunnelName != "" {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   fieldPrefix + "remoteConfig.tunnelName",
			Message: "cannot be set on catalog-synced entries",
		}
	}

	return nil
}

func ValidateSystemMCPServerCatalogEntryManifest(ctx context.Context, manifest types.SystemMCPServerCatalogEntryManifest, options ValidationOptions) error {
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.TunnelName != "" {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "remoteConfig.tunnelName",
			Message: "tunnels are not supported for system MCP servers",
		}
	}

	if manifest.SystemMCPServerType == types.SystemMCPServerTypeFilter {
		if manifest.FilterConfig == nil {
			return types.RuntimeValidationError{
				Runtime: manifest.Runtime,
				Field:   "filterConfig",
				Message: "filterConfig is required when systemMCPServerType is filter",
			}
		}
		if manifest.FilterConfig.ToolName == "" {
			return types.RuntimeValidationError{
				Runtime: manifest.Runtime,
				Field:   "filterConfig.toolName",
				Message: "toolName is required in filterConfig when systemMCPServerType is filter",
			}
		}
	}

	if err := manifest.ValidateConfig(); err != nil {
		return err
	}
	if err := validateConfigurationOptions(manifest.Config, ""); err != nil {
		return err
	}
	for _, env := range manifest.Config {
		if env.SecretBinding != nil {
			return fmt.Errorf("env %q: secretBinding is not supported for system MCP servers", env.Key)
		}
	}
	for _, header := range manifest.Config {
		if header.Usage != types.Header {
			continue
		}
		if strings.TrimSpace(header.Key) == "" {
			return fmt.Errorf("header key cannot be empty")
		}
		if header.SecretBinding != nil {
			return fmt.Errorf("header %q: secretBinding is not supported for system MCP servers", header.Key)
		}
	}
	if utf8.RuneCountInString(manifest.ShortDescription) > maxShortDescriptionLength {
		return fmt.Errorf("short description must be less than or equal to %d characters", maxShortDescriptionLength)
	}
	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}
	if err := validateMCPResourceMaximums(manifest.Resources, options.ResourceMaximums); err != nil {
		return err
	}
	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}
	if manifest.Runtime == types.RuntimeRemote {
		if manifest.RemoteConfig == nil {
			return types.RuntimeValidationError{Runtime: types.RuntimeRemote, Field: "remoteConfig", Message: "remote configuration is required"}
		}
		return (RemoteValidator{
			AllowMissingURL:              options.AllowMissingURL,
			RemoteMCPURLValidationConfig: options.RemoteMCPURLValidationConfig,
		}).validateRemoteCatalogConfig(ctx, types.RemoteCatalogConfig{
			FixedURL:            manifest.RemoteConfig.FixedURL,
			URLTemplate:         manifest.RemoteConfig.URLTemplate,
			Hostname:            manifest.RemoteConfig.Hostname,
			StaticOAuthRequired: manifest.RemoteConfig.StaticOAuthRequired,
		})
	}
	if validator, ok := getRuntimeValidators(options)[manifest.Runtime]; ok {
		return validator.ValidateSystemConfig(ctx, types.SystemMCPServerManifest{
			Runtime:             manifest.Runtime,
			UVXConfig:           manifest.UVXConfig,
			NPXConfig:           manifest.NPXConfig,
			ContainerizedConfig: manifest.ContainerizedConfig,
			Config:              manifest.Config,
			Resources:           manifest.Resources,
		})
	}
	return types.RuntimeValidationError{Runtime: manifest.Runtime, Field: "runtime", Message: "unsupported runtime"}
}

func ValidateSystemMCPServerManifest(ctx context.Context, manifest types.SystemMCPServerManifest, options ValidationOptions) error {
	if err := (types.MCPServerManifest{Config: manifest.Config}).ValidateConfig(); err != nil {
		return err
	}
	for _, config := range manifest.Config {
		if config.UserAllowed {
			return fmt.Errorf("config %q: userAllowed is not supported for system MCP servers", config.Key)
		}
	}
	if err := validateConfigurationOptions(manifest.Config, ""); err != nil {
		return err
	}
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.TunnelName != "" {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "remoteConfig.tunnelName",
			Message: "tunnels are not supported for system MCP servers",
		}
	}

	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}

	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if validator, ok := getRuntimeValidators(options)[manifest.Runtime]; ok {
		if err := validator.ValidateSystemConfig(ctx, manifest); err != nil {
			return err
		}

		for _, env := range manifest.Config {
			if env.SecretBinding != nil {
				return fmt.Errorf("env %q: secretBinding is not supported for system MCP servers", env.Key)
			}
		}

		return nil
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}

func validateRuntimeStartupTimeout(runtime types.Runtime, startupTimeoutSeconds int) error {
	switch runtime {
	case types.RuntimeUVX:
		return validateStartupTimeout(runtime, "uvxConfig.startupTimeoutSeconds", startupTimeoutSeconds)
	case types.RuntimeNPX:
		return validateStartupTimeout(runtime, "npxConfig.startupTimeoutSeconds", startupTimeoutSeconds)
	case types.RuntimeContainerized:
		return validateStartupTimeout(runtime, "containerizedConfig.startupTimeoutSeconds", startupTimeoutSeconds)
	default:
		return nil
	}
}

func validateStartupTimeout(runtime types.Runtime, field string, startupTimeoutSeconds int) error {
	if startupTimeoutSeconds < 0 {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   field,
			Message: "must be greater than or equal to 0",
		}
	}
	if startupTimeoutSeconds > int(MaxMCPServerStartupTimeout.Seconds()) {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   field,
			Message: fmt.Sprintf("must be less than %d", int(MaxMCPServerStartupTimeout.Seconds())),
		}
	}

	return nil
}

// ValidateSecretBindings enforces the rules for secretBinding references on
// env vars and headers. Bindings may appear on git-managed catalog entries,
// multi-user catalog entries, or admin-managed multi-user servers. They require the kubernetes MCP runtime
// backend, are mutually exclusive with a static value, require non-empty
// name/key, and are rejected in unsupported combinations (env bindings under
// remote runtime).
func ValidateSecretBindings(manifest types.MCPServerManifest, gitManaged, adminManaged bool, mcpBackend string) error {
	check := func(kind, key string, h types.MCPHeader) error {
		if h.SecretBinding == nil {
			return nil
		}
		if !IsKubernetesBackend(mcpBackend) {
			return fmt.Errorf("%s %q: secretBinding requires the kubernetes MCP runtime backend", kind, key)
		}
		if !gitManaged && !adminManaged {
			return fmt.Errorf("%s %q: secretBinding is only allowed on git-synced catalog entries, multi-user catalog entries, or admin-managed multi-user servers", kind, key)
		}
		if h.Value != "" {
			return fmt.Errorf("%s %q: secretBinding and value are mutually exclusive", kind, key)
		}
		if h.SecretBinding.Name == "" || h.SecretBinding.Key == "" {
			return fmt.Errorf("%s %q: secretBinding requires both name and key", kind, key)
		}
		return nil
	}

	for _, env := range manifest.Config {
		if env.UserAllowed && env.SecretBinding != nil {
			return fmt.Errorf("multi-user header %q: secretBinding is not supported for user-defined headers", env.Key)
		}
		if env.SecretBinding != nil {
			if manifest.Runtime == types.RuntimeRemote && env.Usage != types.Header {
				return fmt.Errorf("env %q: secretBinding on env vars is not supported for remote runtime", env.Key)
			}
		}
		if err := check(string(env.Usage), env.Key, env.ToHeader()); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSecretBindingsCatalogEntry validates binding ownership and the
// catalog-specific restrictions on secret-bound URL template inputs.
func ValidateSecretBindingsCatalogEntry(manifest types.MCPServerCatalogEntryManifest, gitManaged, userIsAdmin bool, mcpBackend string) error {
	if err := validateNoAdminAddedCatalogBindings(manifest); err != nil {
		return err
	}

	// Reject URL templates that reference secret-bound env vars. Remote
	// secretBinding support is limited to headers; URL templates are not a
	// supported binding target.
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.URLTemplate != "" {
		bound := make(map[string]bool, len(manifest.Config))
		for _, config := range manifest.Config {
			if config.SecretBinding != nil && config.Usage != types.Header {
				bound[config.Key] = true
			}
		}
		for _, ref := range extractEnvRefs(manifest.RemoteConfig.URLTemplate) {
			if bound[ref] {
				return fmt.Errorf("remoteConfig.urlTemplate references secret-bound env var %q; use a header binding instead", ref)
			}
		}
	}

	for _, config := range manifest.Config {
		if config.SecretBinding == nil {
			continue
		}
		if !IsKubernetesBackend(mcpBackend) {
			return fmt.Errorf("config %q: secretBinding requires the kubernetes MCP runtime backend", config.Key)
		}
		if !gitManaged && !userIsAdmin {
			return fmt.Errorf("config %q: secretBinding is only allowed on git-synced catalog entries or administrator-managed vMCPs", config.Key)
		}
		if config.Value != "" {
			return fmt.Errorf("config %q: secretBinding and value are mutually exclusive", config.Key)
		}
		if config.SecretBinding.Name == "" || config.SecretBinding.Key == "" {
			return fmt.Errorf("config %q: secretBinding requires both name and key", config.Key)
		}
		if manifest.Runtime == types.RuntimeRemote && config.Usage != types.Header {
			return fmt.Errorf("config %q: secretBinding is only supported for headers on remote runtime", config.Key)
		}
	}
	return nil
}

func validateNoAdminAddedCatalogBindings(manifest types.MCPServerCatalogEntryManifest) error {
	for _, config := range manifest.Config {
		if config.SecretBinding != nil && config.SecretBinding.AdminAdded {
			return fmt.Errorf("config %q: secretBinding.adminAdded is not valid for catalog entry", config.Key)
		}
	}
	return nil
}

// extractEnvRefs returns the variable names referenced by ${name} patterns in s.
func extractEnvRefs(s string) []string {
	if s == "" {
		return nil
	}
	matches := envVarRefRegex.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

// serverTemplateFields returns every command/args/URL string in a server
// manifest that may carry ${VAR} references.
func serverTemplateFields(m types.MCPServerManifest) []string {
	var out []string
	switch m.Runtime {
	case types.RuntimeUVX:
		if m.UVXConfig != nil {
			out = append(out, m.UVXConfig.Command)
			out = append(out, m.UVXConfig.Args...)
		}
	case types.RuntimeNPX:
		if m.NPXConfig != nil {
			out = append(out, m.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if m.ContainerizedConfig != nil {
			out = append(out, m.ContainerizedConfig.Image, m.ContainerizedConfig.Command)
			out = append(out, m.ContainerizedConfig.Args...)
		}
	case types.RuntimeRemote:
		if m.RemoteConfig != nil {
			out = append(out, m.RemoteConfig.URL)
		}
	}
	return out
}

// catalogTemplateFields is the catalog-entry counterpart to serverTemplateFields.
// The remote runtime config differs from the server-side shape (FixedURL /
// URLTemplate instead of URL), so we extract from those fields instead.
func catalogTemplateFields(m types.MCPServerCatalogEntryManifest) []string {
	var out []string
	switch m.Runtime {
	case types.RuntimeUVX:
		if m.UVXConfig != nil {
			out = append(out, m.UVXConfig.Command)
			out = append(out, m.UVXConfig.Args...)
		}
	case types.RuntimeNPX:
		if m.NPXConfig != nil {
			out = append(out, m.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if m.ContainerizedConfig != nil {
			out = append(out, m.ContainerizedConfig.Image, m.ContainerizedConfig.Command)
			out = append(out, m.ContainerizedConfig.Args...)
		}
	case types.RuntimeRemote:
		if m.RemoteConfig != nil {
			out = append(out, m.RemoteConfig.FixedURL, m.RemoteConfig.URLTemplate)
		}
	}
	return out
}

// validateTemplateReferences enforces that every ${VAR} reference inside
// fields resolves to an env entry marked Required=true. References to
// undeclared env vars error only when requireDeclared is set — server
// manifests auto-extract undeclared refs into Required=true env entries
// elsewhere, so the server-side caller passes false; catalog-entry manifests
// have no such fixup and pass true.
func validateTemplateReferences(envs []types.MCPConfig, fields []string, requireDeclared bool) error {
	required := make(map[string]bool, len(envs))
	for _, env := range envs {
		required[env.Key] = env.Required
	}
	for _, f := range fields {
		for _, name := range extractEnvRefs(f) {
			req, ok := required[name]
			if !ok {
				if requireDeclared {
					return fmt.Errorf("template references undeclared env var %q; declare it under env with required=true", name)
				}
				continue
			}
			if !req {
				return fmt.Errorf("env var %q is referenced from a command/args/URL template and must be required=true", name)
			}
		}
	}
	return nil
}

// ValidateTemplateReferences enforces that any ${VAR} reference inside a
// server manifest's command/args/URL fields points to an env entry with
// Required=true. Undeclared references are tolerated here because
// addExtractedEnvVars in the server-create path auto-stamps a Required=true
// entry for them; this validator catches the case where the user pre-supplied
// the same key with Required=false, which today produces a literal
// "${VAR}" string at runtime instead of a substituted value.
func ValidateTemplateReferences(manifest types.MCPServerManifest) error {
	fields := serverTemplateFields(manifest)
	for _, config := range manifest.Config {
		fields = append(fields, config.Value)
	}
	return validateTemplateReferences(manifest.Config, fields, false)
}

// ValidateTemplateReferencesCatalogEntry is the catalog-entry counterpart.
// Catalog entries don't get the auto-extraction fixup, so undeclared
// ${VAR} references are an error.
func ValidateTemplateReferencesCatalogEntry(manifest types.MCPServerCatalogEntryManifest) error {
	fields := catalogTemplateFields(manifest)
	for _, config := range manifest.Config {
		fields = append(fields, config.Value)
	}
	return validateTemplateReferences(manifest.Config, fields, true)
}
