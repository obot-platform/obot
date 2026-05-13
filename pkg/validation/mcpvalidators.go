package validation

import (
	"cmp"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

var (
	// toolNameRegex matches the character set allowed for composite
	// component tools: ASCII letters, digits, underscore, hyphen, dot,
	// and forward slash. Note that '.' and '/' produce a soft warning downstream
	// (some MCP clients reject them) but are permitted here so admins who know
	// their clients can use them.
	toolNameRegex = regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
	hostnameRegex = regexp.MustCompile(`^(?:\*\.)?[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
)

const (
	// maxToolNameLength is the max length of an MCP server tool.
	// It's used to validate effective tool names after tool overrides and prefixes are applied.
	maxToolNameLength = 128

	// maxToolPrefixLength is the max length of a composite component tool prefix.
	maxToolPrefixLength = 64
)

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

// RuntimeValidator defines the interface for validating runtime-specific configurations
type RuntimeValidator interface {
	ValidateConfig(manifest types.MCPServerManifest) error
	ValidateCatalogConfig(manifest types.MCPServerCatalogEntryManifest) error
	ValidateSystemConfig(manifest types.SystemMCPServerManifest) error
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

func (v UVXValidator) ValidateSystemConfig(manifest types.SystemMCPServerManifest) error {
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

func (v NPXValidator) ValidateSystemConfig(manifest types.SystemMCPServerManifest) error {
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

func (v ContainerizedValidator) ValidateSystemConfig(manifest types.SystemMCPServerManifest) error {
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

func (v RemoteValidator) ValidateSystemConfig(manifest types.SystemMCPServerManifest) error {
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

func (v CompositeValidator) ValidateSystemConfig(manifest types.SystemMCPServerManifest) error {
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
func getRuntimeValidators() RuntimeValidators {
	return RuntimeValidators{
		types.RuntimeUVX:           UVXValidator{},
		types.RuntimeNPX:           NPXValidator{},
		types.RuntimeContainerized: ContainerizedValidator{},
		types.RuntimeRemote:        RemoteValidator{},
		types.RuntimeComposite:     CompositeValidator{},
	}
}

func ValidateServerManifest(manifest types.MCPServerManifest, isMultiUser bool) error {
	if manifest.MultiUserConfig != nil && !isMultiUser {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "multiUserConfig",
			Message: "multiUserConfig may only be set for multi-user servers",
		}
	}
	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

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
	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if validator, ok := getRuntimeValidators()[manifest.Runtime]; ok {
		return validator.ValidateCatalogConfig(manifest)
	}

	return types.RuntimeValidationError{
		Runtime: manifest.Runtime,
		Field:   "runtime",
		Message: "unsupported runtime",
	}
}

func ValidateSystemMCPServerCatalogEntryManifest(manifest types.SystemMCPServerCatalogEntryManifest) error {
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

	return ValidateCatalogEntryManifest(types.MCPServerCatalogEntryManifest{
		Metadata:            manifest.Metadata,
		Name:                manifest.Name,
		ShortDescription:    manifest.ShortDescription,
		Description:         manifest.Description,
		Icon:                manifest.Icon,
		RepoURL:             manifest.RepoURL,
		ToolPreview:         manifest.ToolPreview,
		Runtime:             manifest.Runtime,
		UVXConfig:           manifest.UVXConfig,
		NPXConfig:           manifest.NPXConfig,
		ContainerizedConfig: manifest.ContainerizedConfig,
		RemoteConfig:        manifest.RemoteConfig,
		Env:                 manifest.Env,
	})
}

func ValidateSystemMCPServerManifest(manifest types.SystemMCPServerManifest) error {
	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if validator, ok := getRuntimeValidators()[manifest.Runtime]; ok {
		return validator.ValidateSystemConfig(manifest)
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
	if startupTimeoutSeconds > int(mcp.MaxMCPServerStartupTimeout.Seconds()) {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   field,
			Message: fmt.Sprintf("must be less than %d", int(mcp.MaxMCPServerStartupTimeout.Seconds())),
		}
	}

	return nil
}
