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
	"k8s.io/apimachinery/pkg/api/resource"
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

func ValidateServerManifest(manifest types.MCPServerManifest, isMultiUser bool) error {
	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}

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

// ValidateCatalogEntryForRoute checks that a catalog entry is compatible with the
// route used to create a server. catalogID and workspaceID come from the URL path.
func ValidateCatalogEntryForRoute(manifest types.MCPServerCatalogEntryManifest, catalogID, workspaceID string) error {
	isMultiUser := catalogID != "" || workspaceID != ""
	if isMultiUser {
		// Deploying catalog entries as multi-user servers is not yet supported.
		// This will be enabled when multi-user template deployment is implemented.
		return fmt.Errorf("deploying catalog entries as multi-user servers is not yet supported")
	}
	if !manifest.ServerUserType.IsSingleUser() {
		return fmt.Errorf("only single-user catalog entries are supported")
	}
	return nil
}

func ValidateCatalogEntryManifest(manifest types.MCPServerCatalogEntryManifest) error {
	if !manifest.ServerUserType.IsSingleUser() {
		return fmt.Errorf("unsupported serverUserType %q: only %q is currently supported for catalog entries", manifest.ServerUserType, types.ServerUserTypeSingleUser)
	}

	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}

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
		Resources:           manifest.Resources,
	})
}

func ValidateSystemMCPServerManifest(manifest types.SystemMCPServerManifest) error {
	if err := validateMCPResourceRequirements(manifest.Runtime, manifest.Resources); err != nil {
		return err
	}

	if err := validateRuntimeStartupTimeout(manifest.Runtime, manifest.RuntimeStartupTimeoutSeconds()); err != nil {
		return err
	}

	if validator, ok := getRuntimeValidators()[manifest.Runtime]; ok {
		if err := validator.ValidateSystemConfig(manifest); err != nil {
			return err
		}

		for _, env := range manifest.Env {
			if env.SecretBinding != nil {
				return fmt.Errorf("env %q: secretBinding is not supported for system MCP servers", env.Key)
			}
		}

		if manifest.RemoteConfig != nil {
			for _, header := range manifest.RemoteConfig.Headers {
				if header.SecretBinding != nil {
					return fmt.Errorf("header %q: secretBinding is not supported for system MCP servers", header.Key)
				}
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
	if startupTimeoutSeconds > int(mcp.MaxMCPServerStartupTimeout.Seconds()) {
		return types.RuntimeValidationError{
			Runtime: runtime,
			Field:   field,
			Message: fmt.Sprintf("must be less than %d", int(mcp.MaxMCPServerStartupTimeout.Seconds())),
		}
	}

	return nil
}

// ValidateSecretBindings enforces the rules for secretBinding references on
// env vars and headers. Bindings are GitOps-only: they may only appear on
// catalog entries synced from git (gitManaged=true). They also require the
// kubernetes MCP runtime backend, are mutually exclusive with a static value,
// require non-empty name/key, and are rejected in unsupported combinations
// (env bindings under remote runtime).
func ValidateSecretBindings(manifest types.MCPServerManifest, gitManaged bool, mcpBackend string) error {
	check := func(kind, key string, h types.MCPHeader) error {
		if h.SecretBinding == nil {
			return nil
		}
		if mcpBackend != "kubernetes" && mcpBackend != "k8s" {
			return fmt.Errorf("%s %q: secretBinding requires the kubernetes MCP runtime backend", kind, key)
		}
		if !gitManaged {
			return fmt.Errorf("%s %q: secretBinding is only allowed on git-synced catalog entries", kind, key)
		}
		if h.Value != "" {
			return fmt.Errorf("%s %q: secretBinding and value are mutually exclusive", kind, key)
		}
		if h.SecretBinding.Name == "" || h.SecretBinding.Key == "" {
			return fmt.Errorf("%s %q: secretBinding requires both name and key", kind, key)
		}
		return nil
	}

	for _, env := range manifest.Env {
		if env.SecretBinding != nil {
			if manifest.Runtime == types.RuntimeRemote {
				return fmt.Errorf("env %q: secretBinding on env vars is not supported for remote runtime", env.Key)
			}
		}
		if err := check("env", env.Key, env.MCPHeader); err != nil {
			return err
		}
	}
	if manifest.RemoteConfig != nil {
		for _, h := range manifest.RemoteConfig.Headers {
			if err := check("header", h.Key, h); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateSecretBindingsCatalogEntry is a thin wrapper around
// ValidateSecretBindings that adapts a catalog-entry manifest (which does not
// carry the runtime/env shape of MCPServerManifest directly) by extracting
// the fields that matter for binding validation. The catalog-entry manifest
// uses the same MCPEnv/MCPHeader types, so we reuse the core logic.
func ValidateSecretBindingsCatalogEntry(manifest types.MCPServerCatalogEntryManifest, gitManaged bool, mcpBackend string) error {
	// Reject URL templates that reference secret-bound env vars. Remote
	// secretBinding support is limited to headers; URL templates are not a
	// supported binding target.
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.URLTemplate != "" {
		bound := make(map[string]bool, len(manifest.Env))
		for _, env := range manifest.Env {
			if env.SecretBinding != nil {
				bound[env.Key] = true
			}
		}
		for _, ref := range extractEnvRefs(manifest.RemoteConfig.URLTemplate) {
			if bound[ref] {
				return fmt.Errorf("remoteConfig.urlTemplate references secret-bound env var %q; use a header binding instead", ref)
			}
		}
	}

	// Synthesize a minimal MCPServerManifest so we can reuse the core check.
	synthetic := types.MCPServerManifest{
		Runtime:      manifest.Runtime,
		Env:          manifest.Env,
		RemoteConfig: remoteCatalogToRuntime(manifest.RemoteConfig),
	}
	return ValidateSecretBindings(synthetic, gitManaged, mcpBackend)
}

func remoteCatalogToRuntime(c *types.RemoteCatalogConfig) *types.RemoteRuntimeConfig {
	if c == nil {
		return nil
	}
	return &types.RemoteRuntimeConfig{Headers: c.Headers}
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
			for _, h := range m.RemoteConfig.Headers {
				out = append(out, h.Value)
			}
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
			for _, h := range m.RemoteConfig.Headers {
				out = append(out, h.Value)
			}
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
func validateTemplateReferences(envs []types.MCPEnv, fields []string, requireDeclared bool) error {
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
	return validateTemplateReferences(manifest.Env, serverTemplateFields(manifest), false)
}

// ValidateTemplateReferencesCatalogEntry is the catalog-entry counterpart.
// Catalog entries don't get the auto-extraction fixup, so undeclared
// ${VAR} references are an error.
func ValidateTemplateReferencesCatalogEntry(manifest types.MCPServerCatalogEntryManifest) error {
	return validateTemplateReferences(manifest.Env, catalogTemplateFields(manifest), true)
}
