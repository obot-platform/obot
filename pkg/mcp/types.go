package mcp

import (
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// MaxMCPServerStartupTimeout is the maximum value allowed to be used in ServerConfig.StartupTimeout
	MaxMCPServerStartupTimeout = 10 * time.Minute

	// AuditLogIgnore is a metadata field that tells the audit log persistence layer to ignore audit logs for this server
	AuditLogIgnore = "obot.mcp.ignoreAuditLog"
)

var (
	envVarRegex = regexp.MustCompile(`\${([^}]+)}`)
)

type ServerConfig struct {
	Runtime types.Runtime `json:"runtime"`

	// uvx/npx based configuration.
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env"`
	Files   []File   `json:"files"`

	// Remote configuration.
	LocalhostCallbackEnabled bool     `json:"localhostCallbackEnabled,omitempty"`
	LocalhostCallbackPath    string   `json:"localhostCallbackPath,omitempty"`
	URL                      string   `json:"url"`
	TunnelName               string   `json:"tunnelName,omitempty"`
	Headers                  []string `json:"headers"`
	PassthroughHeaderNames   []string `json:"passthroughHeaderNames"`
	PassthroughHeaderValues  []string `json:"passthroughHeaderValues"`

	// Containerized configuration.
	ContainerImage string `json:"containerImage"`
	ContainerPort  int    `json:"containerPort"`
	ContainerPath  string `json:"containerPath"`
	HealthzPath    string `json:"healthzPath,omitempty"`

	// vMCP configuration.
	Components []ComponentServer `json:"components"`
	ConfigHash string            `json:"configHash"`
	ToolSearch bool              `json:"toolSearch,omitempty"`

	Scope                string `json:"scope"`
	UserID               string `json:"userID"`
	OwnerUserID          string `json:"ownerUserID"`
	MCPServerNamespace   string `json:"mcpServerNamespace"`
	MCPServerName        string `json:"mcpServerName"`
	MCPServerInstanceID  string `json:"mcpServerInstanceID,omitempty"`
	MCPCatalogName       string `json:"mcpCatalogName"`
	MCPCatalogEntryName  string `json:"mcpCatalogEntryName"`
	MCPServerDisplayName string `json:"mcpServerDisplayName"`
	ComponentMCPServer   bool   `json:"componentMCPServer"`
	SystemMCPServer      bool   `json:"systemMCPServer"`

	Audiences []string `json:"audiences"`

	AuditLogMetadata map[string]string `json:"auditLogMetadata"`

	StartupTimeout time.Duration                `json:"startupTimeout,omitempty"`
	Resources      *corev1.ResourceRequirements `json:"resources,omitempty"`
	Webhooks       []Webhook                    `json:"webhooks,omitempty"`
}

type File struct {
	Data    string `json:"data"`
	EnvKey  string `json:"envKey"`
	Dynamic bool   `json:"dynamic"`
}

// caCertificateEnvKeys are the env vars mainstream HTTP/TLS clients (Node, Python,
// curl, git, and Go via SSL_CERT_FILE) check for an additional trusted CA bundle.
// A config entry with Usage: types.CACertificate is mounted once per key so every
// common client picks it up without the user needing to know each one's name.
var caCertificateEnvKeys = []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "GIT_SSL_CAINFO"}

type ComponentServer struct {
	MCPServerInstanceID string               `json:"mcpServerInstanceID,omitempty"`
	DisableTools        bool                 `json:"disableTools,omitempty"`
	Name                string               `json:"name"`
	DisplayName         string               `json:"displayName"`
	URL                 string               `json:"url"`
	Tools               []types.ToolOverride `json:"tools"`
	ToolPrefix          string               `json:"toolPrefix"`
}

// ConnectID identifies the connection while Name identifies its backing server.
func (c ComponentServer) ConnectID() string {
	if c.MCPServerInstanceID != "" {
		return c.MCPServerInstanceID
	}
	return c.Name
}

// isHTTPContainerRuntime reports whether the runtime serves MCP over HTTP
// from its own image. NPX and UVX use the shared MMMCP command wrapper instead.
func isHTTPContainerRuntime(runtime types.Runtime) bool {
	return runtime == types.RuntimeContainerized || runtime == types.RuntimeOpenAPI
}

func (s ServerConfig) hostedConnectionHeaders() []string {
	if s.Runtime == types.RuntimeOpenAPI {
		return s.Headers
	}
	return nil
}

func CoreResourceRequirements(resources *types.MCPResourceRequirements) (*corev1.ResourceRequirements, error) {
	if resources == nil {
		return nil, nil
	}

	result := new(corev1.ResourceRequirements)
	if resources.Requests.CPU != "" || resources.Requests.Memory != "" {
		result.Requests = corev1.ResourceList{}
		if resources.Requests.CPU != "" {
			cpu, err := resource.ParseQuantity(resources.Requests.CPU)
			if err != nil {
				return result, fmt.Errorf("invalid CPU request %q: %w", resources.Requests.CPU, err)
			}
			result.Requests[corev1.ResourceCPU] = cpu
		}
		if resources.Requests.Memory != "" {
			memory, err := resource.ParseQuantity(resources.Requests.Memory)
			if err != nil {
				return result, fmt.Errorf("invalid memory request %q: %w", resources.Requests.Memory, err)
			}
			result.Requests[corev1.ResourceMemory] = memory
		}
	}
	if resources.Limits.CPU != "" || resources.Limits.Memory != "" {
		result.Limits = corev1.ResourceList{}
		if resources.Limits.CPU != "" {
			cpu, err := resource.ParseQuantity(resources.Limits.CPU)
			if err != nil {
				return result, fmt.Errorf("invalid CPU limit %q: %w", resources.Limits.CPU, err)
			}
			result.Limits[corev1.ResourceCPU] = cpu
		}
		if resources.Limits.Memory != "" {
			memory, err := resource.ParseQuantity(resources.Limits.Memory)
			if err != nil {
				return result, fmt.Errorf("invalid memory limit %q: %w", resources.Limits.Memory, err)
			}
			result.Limits[corev1.ResourceMemory] = memory
		}
	}

	return result, nil
}

// expandEnvVars replaces ${VAR} patterns with values from credEnv
func expandEnvVars(text string, credEnv map[string]string, fileEnvVars map[string]struct{}) string {
	if credEnv == nil {
		return text
	}

	return envVarRegex.ReplaceAllStringFunc(text, func(match string) string {
		varName := match[2 : len(match)-1] // Remove ${ and }
		if _, isFileVar := fileEnvVars[varName]; !isFileVar {
			// If it's a file variable, then don't expand here.
			if val, ok := credEnv[varName]; ok {
				return val
			}
		}
		return match // Return original if not found
	})
}

// expandURLTemplate resolves the ${VAR} references in a remote URL template. Rather than
// returning a partially expanded URL, it returns the names of every reference that has no
// configured value so the caller can report them as missing configuration.
func expandURLTemplate(urlTemplate string, credEnv map[string]string) (string, []string) {
	var missing []string
	for _, reference := range URLTemplateReferences(urlTemplate) {
		if credEnv[reference] == "" {
			missing = append(missing, reference)
		}
	}
	if len(missing) > 0 {
		return "", missing
	}

	return expandEnvVars(urlTemplate, credEnv, nil), nil
}

// URLTemplateReferences returns the names referenced by ${VAR} patterns in a remote URL
// template, in order, without duplicates.
func URLTemplateReferences(urlTemplate string) []string {
	var references []string
	for _, match := range envVarRegex.FindAllStringSubmatch(urlTemplate, -1) {
		if !slices.Contains(references, match[1]) {
			references = append(references, match[1])
		}
	}
	return references
}

// ResolveRemoteURLTemplate expands a remote manifest's URL template using the supplied
// configuration alongside the manifest's own static values, the same way the runtime does
// when it builds a ServerConfig. It returns the names of every unresolved reference rather
// than a partially expanded URL.
func ResolveRemoteURLTemplate(manifest types.MCPServerManifest, configuration map[string]string) (string, []string) {
	if manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil || manifest.RemoteConfig.URLTemplate == "" {
		return "", nil
	}

	values := make(map[string]string, len(configuration)+len(manifest.Config))
	maps.Copy(values, configuration)
	for _, config := range manifest.Config {
		if config.Value != "" {
			values[config.Key] = config.Value
		}
	}

	return expandURLTemplate(manifest.RemoteConfig.URLTemplate, values)
}

// dedupe removes duplicate names, preserving the order of their first occurrence.
func dedupe(names []string) []string {
	if len(names) < 2 {
		return names
	}

	seen := make(map[string]struct{}, len(names))
	deduped := names[:0]
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		deduped = append(deduped, name)
	}
	return deduped
}

// applyPrefix adds a prefix to a value if the value doesn't already start with it.
// Returns the original value if prefix is empty or if value already starts with the prefix.
func applyPrefix(value, prefix string) string {
	if value == "" || strings.HasPrefix(value, prefix) {
		return value
	}
	return prefix + value
}

func configureUVXRuntime(serverConfig *ServerConfig, uvxConfig *types.UVXRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}) error {
	if uvxConfig == nil {
		return fmt.Errorf("uvx runtime requires uvx config")
	}

	serverConfig.HealthzPath = "/readyz"
	serverConfig.Command = "uvx"
	if uvxConfig.Command != "" {
		serverConfig.Args = []string{"--from", uvxConfig.Package, expandEnvVars(uvxConfig.Command, credEnv, fileEnvVars)}
	} else {
		serverConfig.Args = []string{uvxConfig.Package}
	}

	for _, arg := range uvxConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	return nil
}

func configureNPXRuntime(serverConfig *ServerConfig, npxConfig *types.NPXRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}) error {
	if npxConfig == nil {
		return fmt.Errorf("npx runtime requires npx config")
	}

	serverConfig.HealthzPath = "/readyz"
	serverConfig.Command = "npx"
	serverConfig.Args = []string{npxConfig.Package}
	for _, arg := range npxConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	return nil
}

func configureContainerizedRuntime(serverConfig *ServerConfig, containerizedConfig *types.ContainerizedRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}, expandImage bool, caCertificate string) error {
	if containerizedConfig == nil {
		return fmt.Errorf("containerized runtime requires containerized config")
	}

	serverConfig.ContainerImage = containerizedConfig.Image
	if expandImage {
		serverConfig.ContainerImage = expandEnvVars(containerizedConfig.Image, credEnv, fileEnvVars)
	}
	serverConfig.ContainerPort = containerizedConfig.Port
	serverConfig.ContainerPath = containerizedConfig.Path
	serverConfig.HealthzPath = containerizedConfig.HealthzPath
	serverConfig.Command = expandEnvVars(containerizedConfig.Command, credEnv, fileEnvVars)
	for _, arg := range containerizedConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	// Best-effort: install the custom CA into the container's OS trust store before
	// starting it. This only works when we know the real start command to re-exec
	// afterward, which is only true when the user gave us an explicit command override.
	if caCertificate != "" && serverConfig.Command != "" {
		wrapCommandForCATrustStore(serverConfig)
	}

	return nil
}

// caTrustStorePreamble best-effort installs a CA certificate mounted via the
// SSL_CERT_FILE env var (see caCertificateEnvKeys) into the container's OS trust
// store, covering both Debian/Ubuntu-style (update-ca-certificates) and
// RHEL/Fedora-style (update-ca-trust) layouts, before handing control back to the
// real command. Every step is guarded so a missing tool or read-only path never
// prevents the real command from starting.
const caTrustStorePreamble = `if [ -n "$SSL_CERT_FILE" ] && [ -f "$SSL_CERT_FILE" ]; then ` +
	`mkdir -p /usr/local/share/ca-certificates 2>/dev/null && cp "$SSL_CERT_FILE" /usr/local/share/ca-certificates/obot-custom-ca.crt 2>/dev/null; ` +
	`mkdir -p /etc/pki/ca-trust/source/anchors 2>/dev/null && cp "$SSL_CERT_FILE" /etc/pki/ca-trust/source/anchors/obot-custom-ca.crt 2>/dev/null; ` +
	`fi; ` +
	`update-ca-certificates >/dev/null 2>&1 || update-ca-trust extract >/dev/null 2>&1 || true; ` +
	`exec "$@"`

// wrapCommandForCATrustStore re-targets the container's command through a shell
// preamble that attempts a best-effort OS trust-store install before re-exec'ing
// the original command and arguments.
func wrapCommandForCATrustStore(serverConfig *ServerConfig) {
	originalCommand := serverConfig.Command
	originalArgs := serverConfig.Args

	serverConfig.Command = "/bin/sh"
	serverConfig.Args = append([]string{"-c", caTrustStorePreamble, "--", originalCommand}, originalArgs...)
}

func configureRemoteRuntime(serverConfig *ServerConfig, remoteConfig *types.RemoteRuntimeConfig, config []types.MCPConfig, credEnv map[string]string) ([]string, error) {
	if remoteConfig == nil {
		return nil, fmt.Errorf("remote runtime requires remote config")
	}

	serverConfig.URL = remoteConfig.URL
	serverConfig.LocalhostCallbackEnabled = remoteConfig.LocalhostCallbackEnabled
	serverConfig.LocalhostCallbackPath = remoteConfig.LocalhostCallbackPath
	serverConfig.TunnelName = remoteConfig.TunnelName
	serverConfig.Headers = make([]string, 0, len(config))

	var missingRequiredNames []string
	switch {
	case remoteConfig.Hostname != "":
		if userURL := credEnv["__url"]; userURL != "" {
			serverConfig.URL = userURL
		}
		if serverConfig.URL == "" {
			missingRequiredNames = append(missingRequiredNames, "__url")
		} else if err := types.ValidateURLHostname(serverConfig.URL, remoteConfig.Hostname); err != nil {
			return nil, err
		}
	case serverConfig.URL == "" && remoteConfig.URLTemplate != "":
		// Servers configured through the API expand the template once and persist the result
		// on the manifest. vMCP component servers never take that path: their configuration
		// lives in a credential synced from the vMCP, so the template is resolved here with
		// the configuration for this connection.
		expanded, missing := expandURLTemplate(remoteConfig.URLTemplate, credEnv)
		if len(missing) > 0 {
			missingRequiredNames = append(missingRequiredNames, missing...)
		} else {
			serverConfig.URL = expanded
		}
	}
	missingRequiredNames = append(missingRequiredNames, configureHeaders(serverConfig, config, credEnv)...)
	return missingRequiredNames, nil
}

// configureHeaders resolves server-owned credentials into HTTP headers. Per-user
// credentials are attached separately by serverInstanceHeaders on each request.
func configureHeaders(serverConfig *ServerConfig, config []types.MCPConfig, credEnv map[string]string) []string {
	var missingRequiredNames []string
	for _, header := range config {
		if header.Usage != types.Header || header.UserAllowed {
			continue
		}
		val := header.Value
		if val == "" {
			val = credEnv[header.Key]
		}

		if val == "" {
			if header.Required {
				missingRequiredNames = append(missingRequiredNames, header.Key)
			}
			continue
		}

		// Only apply the prefix if the value is not static.
		if header.Value == "" {
			val = applyPrefix(val, header.Prefix)
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

	return missingRequiredNames
}

// ServerToServerConfig resolves a manifest into runtime configuration.
// Hosted runtime images are selected by the deployment backend.
func ServerToServerConfig(mcpServer v1.MCPServer, audiences []string, userID, scope, mcpCatalogName string, credEnv map[string]string, devMode bool) (ServerConfig, []string, error) {
	fixedConfig := slices.DeleteFunc(slices.Clone(mcpServer.Spec.Manifest.Config), func(config types.MCPConfig) bool {
		return config.UserAllowed
	})
	if _, err := ValidateConfiguredOptions(fixedConfig, credEnv); err != nil {
		return ServerConfig{}, nil, err
	}

	// Catalog-managed literal values are static configuration, not user credentials.
	// Make them available while expanding runtime arguments, but keep them separate
	// from credEnv so they are never persisted or exposed as user-supplied secrets.
	runtimeCredEnv := make(map[string]string, len(credEnv)+len(mcpServer.Spec.Manifest.Config))
	maps.Copy(runtimeCredEnv, credEnv)
	for _, env := range mcpServer.Spec.Manifest.Config {
		if env.UserAllowed {
			delete(runtimeCredEnv, env.Key)
			continue
		}
		if env.Value != "" {
			runtimeCredEnv[env.Key] = env.Value
		}
	}

	fileEnvVars := make(map[string]struct{})
	for _, file := range mcpServer.Spec.Manifest.Config {
		if file.Usage == types.File || file.Usage == types.DynamicFile {
			fileEnvVars[file.Key] = struct{}{}
		}
	}

	displayName := mcpServer.Spec.Manifest.Name
	if displayName == "" {
		displayName = mcpServer.Name
	}

	var powerUserWorkspaceID string
	if system.IsPowerUserWorkspaceID(mcpCatalogName) {
		powerUserWorkspaceID = mcpCatalogName
	}

	startupTimeoutSeconds := mcpServer.Spec.Manifest.RuntimeStartupTimeoutSeconds()
	startupTimeout := time.Duration(startupTimeoutSeconds) * time.Second
	if startupTimeout > MaxMCPServerStartupTimeout {
		return ServerConfig{}, nil, fmt.Errorf("input %d exceeds the max of %s", startupTimeoutSeconds, MaxMCPServerStartupTimeout)
	}

	var passthroughHeaderNames []string
	for _, header := range mcpServer.Spec.Manifest.Config {
		if header.Usage == types.Header && header.UserAllowed {
			passthroughHeaderNames = append(passthroughHeaderNames, header.Key)
		}
	}

	resources, err := CoreResourceRequirements(mcpServer.Spec.Manifest.Resources)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	serverConfig := ServerConfig{
		Env:                    make([]string, 0, len(mcpServer.Spec.Manifest.Config)),
		UserID:                 userID,
		OwnerUserID:            mcpServer.Spec.UserID,
		Scope:                  fmt.Sprintf("%s-%s", mcpServer.Name, scope),
		MCPServerNamespace:     mcpServer.Namespace,
		MCPServerName:          mcpServer.Name,
		MCPCatalogName:         mcpCatalogName,
		MCPCatalogEntryName:    mcpServer.Spec.MCPServerCatalogEntryName,
		MCPServerDisplayName:   displayName,
		Runtime:                mcpServer.Spec.Manifest.Runtime,
		Audiences:              audiences,
		PassthroughHeaderNames: passthroughHeaderNames,
		ComponentMCPServer:     mcpServer.Spec.VMCPComponentID != "",
		StartupTimeout:         startupTimeout,
		Resources:              resources,
	}

	if !serverConfig.ComponentMCPServer {
		// Component requests are audited at the vMCP level.
		serverConfig.AuditLogMetadata = map[string]string{
			"mcpID":                     mcpServer.Name,
			"mcpServerCatalogEntryName": mcpServer.Spec.MCPServerCatalogEntryName,
			"powerUserWorkspaceID":      powerUserWorkspaceID,
			"mcpServerDisplayName":      displayName,
			"userID":                    userID,
		}
	} else {
		// Tell the audit logger to not store audit logs for component MCP servers
		serverConfig.AuditLogMetadata = map[string]string{
			AuditLogIgnore: "true",
		}
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch mcpServer.Spec.Manifest.Runtime {
	case types.RuntimeOpenAPI:
		missingRequiredNames, err = configureOpenAPIRuntime(&serverConfig, mcpServer.Spec.Manifest.OpenAPIConfig, mcpServer.Spec.Manifest.Config, runtimeCredEnv, devMode)
		if err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeUVX:
		if err := configureUVXRuntime(&serverConfig, mcpServer.Spec.Manifest.UVXConfig, runtimeCredEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeNPX:
		if err := configureNPXRuntime(&serverConfig, mcpServer.Spec.Manifest.NPXConfig, runtimeCredEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeContainerized:
		serverConfig.Args = make([]string, 0, len(mcpServer.Spec.Manifest.ContainerizedConfig.Args))
		caCert := resolveCACertificate(mcpServer.Spec.Manifest.Config, runtimeCredEnv)
		if err := configureContainerizedRuntime(&serverConfig, mcpServer.Spec.Manifest.ContainerizedConfig, runtimeCredEnv, fileEnvVars, true, caCert); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeRemote:
		var err error
		missingRequiredNames, err = configureRemoteRuntime(&serverConfig, mcpServer.Spec.Manifest.RemoteConfig, mcpServer.Spec.Manifest.Config, runtimeCredEnv)
		if err != nil {
			return serverConfig, missingRequiredNames, err
		}
	default:
		return serverConfig, missingRequiredNames, fmt.Errorf("unknown runtime %s", mcpServer.Spec.Manifest.Runtime)
	}

	for _, env := range mcpServer.Spec.Manifest.Config {
		if env.Usage == types.Header {
			continue
		}
		val := env.Value
		isStatic := val != ""
		if !isStatic {
			val = runtimeCredEnv[env.Key]
		}
		if val == "" {
			if env.Required {
				missingRequiredNames = append(missingRequiredNames, env.Key)
			}
			continue
		}

		// Static catalog values are already fully configured, like static headers.
		if !isStatic {
			val = applyPrefix(val, env.Prefix)
		}

		if env.Usage == types.Interpolated {
			continue
		}
		if env.Usage == types.CACertificate {
			for _, key := range caCertificateEnvKeys {
				serverConfig.Files = append(serverConfig.Files, File{Data: val, EnvKey: key})
			}
			continue
		}
		if env.Usage != types.File && env.Usage != types.DynamicFile {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:    val,
			EnvKey:  env.Key,
			Dynamic: env.Usage == types.DynamicFile,
		})
	}

	return serverConfig, dedupe(missingRequiredNames), nil
}

// resolveCACertificate returns the resolved value of the manifest's Usage:
// CACertificate config entry, if any, so callers that run before the main
// per-entry resolution loop (e.g. deciding whether to wrap a containerized
// runtime's command) can see it early.
func resolveCACertificate(config []types.MCPConfig, runtimeCredEnv map[string]string) string {
	for _, c := range config {
		if c.Usage == types.CACertificate {
			return runtimeCredEnv[c.Key]
		}
	}
	return ""
}

// SystemServerToServerConfig converts a v1.SystemMCPServer to a ServerConfig for deployment
func SystemServerToServerConfig(systemServer v1.SystemMCPServer, audiences []string, userID string, credEnv map[string]string) (ServerConfig, []string, error) {
	if _, err := ValidateConfiguredOptions(systemServer.Spec.Manifest.Config, credEnv); err != nil {
		return ServerConfig{}, nil, err
	}
	credEnv = maps.Clone(credEnv)
	if credEnv == nil {
		credEnv = map[string]string{}
	}
	for _, config := range systemServer.Spec.Manifest.Config {
		if config.Value != "" {
			credEnv[config.Key] = config.Value
		}
	}

	fileEnvVars := make(map[string]struct{})
	for _, env := range systemServer.Spec.Manifest.Config {
		if env.Usage == types.File || env.Usage == types.DynamicFile {
			fileEnvVars[env.Key] = struct{}{}
		}
	}

	displayName := systemServer.Spec.Manifest.Name
	if displayName == "" {
		displayName = systemServer.Name
	}

	startupTimeoutSeconds := systemServer.Spec.Manifest.RuntimeStartupTimeoutSeconds()
	startupTimeout := time.Duration(startupTimeoutSeconds) * time.Second
	if startupTimeout > MaxMCPServerStartupTimeout {
		return ServerConfig{}, nil, fmt.Errorf("input %d exceeds the max of %s", startupTimeoutSeconds, MaxMCPServerStartupTimeout)
	}

	resources, err := CoreResourceRequirements(systemServer.Spec.Manifest.Resources)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	serverConfig := ServerConfig{
		Env:                  make([]string, 0, len(systemServer.Spec.Manifest.Config)),
		MCPServerNamespace:   systemServer.Namespace,
		MCPServerName:        systemServer.Name,
		MCPServerDisplayName: displayName,
		Runtime:              systemServer.Spec.Manifest.Runtime,
		Scope:                fmt.Sprintf("%s-system", systemServer.Name),
		Audiences:            audiences,
		UserID:               userID,
		AuditLogMetadata: map[string]string{
			"mcpID":                systemServer.Name,
			"mcpServerDisplayName": displayName,
			"userID":               userID,
		},
		SystemMCPServer: true,
		StartupTimeout:  startupTimeout,
		Resources:       resources,
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch systemServer.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if err := configureUVXRuntime(&serverConfig, systemServer.Spec.Manifest.UVXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeNPX:
		if err := configureNPXRuntime(&serverConfig, systemServer.Spec.Manifest.NPXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeContainerized:
		if err := configureContainerizedRuntime(&serverConfig, systemServer.Spec.Manifest.ContainerizedConfig, credEnv, fileEnvVars, false, ""); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeRemote:
		var err error
		missingRequiredNames, err = configureRemoteRuntime(&serverConfig, systemServer.Spec.Manifest.RemoteConfig, systemServer.Spec.Manifest.Config, credEnv)
		if err != nil {
			return serverConfig, missingRequiredNames, err
		}
	default:
		return ServerConfig{}, nil, fmt.Errorf("unsupported runtime type: %s", systemServer.Spec.Manifest.Runtime)
	}

	// Process environment variables
	for _, env := range systemServer.Spec.Manifest.Config {
		if env.Usage == types.Header {
			continue
		}
		var (
			val      string
			hasValue bool
		)

		// Check for static value first
		if env.Value != "" {
			val = env.Value
			hasValue = true
		} else {
			// Fall back to user-configured value from credentials
			credVal, ok := credEnv[env.Key]
			if ok && credVal != "" {
				val = credVal
				hasValue = true
			}
		}

		if !hasValue {
			if env.Required {
				missingRequiredNames = append(missingRequiredNames, env.Key)
			}
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "sk-")
		// Only apply to user-supplied values, not static values
		if env.Value == "" {
			val = applyPrefix(val, env.Prefix)
		}

		if env.Usage == types.Interpolated {
			continue
		}
		if env.Usage != types.File && env.Usage != types.DynamicFile {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:    val,
			EnvKey:  env.Key,
			Dynamic: env.Usage == types.DynamicFile,
		})
	}

	return serverConfig, dedupe(missingRequiredNames), nil
}

func copyHeaders(headers http.Header, keys, values []string) {
	for i, key := range keys {
		if i < len(values) {
			headers.Set(key, values[i])
		}
	}
}
