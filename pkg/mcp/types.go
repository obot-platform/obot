package mcp

import (
	"fmt"
	"maps"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
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
	LocalhostCallbackEnabled bool               `json:"localhostCallbackEnabled,omitempty"`
	LocalhostCallbackPath    string             `json:"localhostCallbackPath,omitempty"`
	URL                      string             `json:"url"`
	TunnelName               string             `json:"tunnelName,omitempty"`
	Headers                  []string           `json:"headers"`
	InjectObotHeaders        []types.ObotHeader `json:"injectObotHeaders,omitempty"`
	PassthroughHeaderNames   []string           `json:"passthroughHeaderNames"`
	PassthroughHeaderValues  []string           `json:"passthroughHeaderValues"`

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

	// ToolPreview marks a temporary server launched only to list tools. Backends label its
	// resources so that a missed cleanup can be found and swept.
	ToolPreview bool `json:"toolPreview,omitempty"`
}

type File struct {
	Data    string `json:"data"`
	EnvKey  string `json:"envKey"`
	Dynamic bool   `json:"dynamic"`
}

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

func (s ServerConfig) hostedConnectionObotHeaders() []types.ObotHeader {
	// Hosted connections become remote configs, so check the original runtime
	// before carrying selections into the HTTP gateway.
	if !types.SupportsObotHeaders(s.Runtime) {
		return nil
	}
	return s.InjectObotHeaders
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
// configured value so the caller can report them as missing configuration. Once every reference
// resolves, it returns an error if the values change the host the template pins.
//
// adminValues holds the values an admin set (see adminURLTemplateValues), keyed like credEnv.
// They are written into the template before the host check, so their text counts as literal
// text the admin wrote: an admin who fixes DD_SITE to "datadoghq.eu" in
// "https://mcp.${DD_SITE}/api/mcp" chose that domain, and the check must not reject it.
func expandURLTemplate(urlTemplate string, credEnv, adminValues map[string]string) (string, []string, error) {
	var missing []string
	for _, reference := range URLTemplateReferences(urlTemplate) {
		if credEnv[reference] == "" {
			missing = append(missing, reference)
		}
	}
	if len(missing) > 0 {
		return "", missing, nil
	}

	expanded := expandEnvVars(urlTemplate, credEnv, nil)
	adminTemplate := expandEnvVars(urlTemplate, adminValues, nil)
	// With every value admin-set, nothing a user supplied reaches the URL, so there is nothing
	// to pin. Skipping also keeps the check's errors, which name the remaining variables, from
	// naming none.
	if len(URLTemplateReferences(adminTemplate)) == 0 {
		return expanded, nil, nil
	}
	if err := validateURLTemplateHost(adminTemplate, expanded, credEnv); err != nil {
		return "", nil, err
	}
	return expanded, nil, nil
}

// adminURLTemplateValues returns the URL template values in values that an admin chose: a
// static catalog Value, or a value that matches one of the field's admin-written Options. A
// user can pick an option, but only from the admin's list, so the result is still a domain the
// admin wrote. The option is matched here rather than trusted because the field has Options,
// since a stored value is not re-validated on every path that reaches this.
func adminURLTemplateValues(config []types.MCPConfig, values map[string]string) map[string]string {
	adminValues := make(map[string]string)
	for _, c := range config {
		if c.Value != "" {
			adminValues[c.Key] = c.Value
			continue
		}
		if value, ok := values[c.Key]; ok && slices.ContainsFunc(c.Options, func(option types.MCPConfigurationOption) bool {
			return option.Value == value
		}) {
			adminValues[c.Key] = value
		}
	}
	return adminValues
}

// validateURLTemplateHost rejects configuration values that change the host an admin wrote
// into a URL template. Without it, a user could set a value like "evil.example/x?" for
// "https://${REGION}.api.example.com/mcp" and Obot would send the entry's static headers to
// a host the user chose.
//
// The template is admin-authored, so its literal "://" and the first '/', '?' or '#' after it
// reliably mark where the authority begins and ends. Values may fill in parts of it (a host
// label, the whole host, a port) but must not end or restructure it, and unless a value is the
// whole hostname it must not form the host's registrable domain. A template without a
// literal scheme separator, such as "${BASE_URL}/mcp", lets users choose the whole host on
// purpose, so there is nothing to pin.
func validateURLTemplateHost(urlTemplate, expanded string, values map[string]string) error {
	scheme, rest, ok := strings.Cut(urlTemplate, "://")
	// A '/', '?' or '#' before the first "://" means that "://" is in the path or query (as in
	// "${BASE}/r?u=https://x"), not a literal scheme separator.
	if !ok || strings.ContainsAny(scheme, "/?#") {
		return nil
	}
	// The authority ends at the template's first literal '/', '?' or '#'. A template with none of
	// those after the host, such as "https://api.example.com${PATH}", has no admin-written end to
	// its authority, so the variable counts as part of the host and any value that starts a path
	// is rejected. That limit is deliberate: cutting the expanded authority at the value's own
	// '/' instead would let a value end the host wherever it likes, which is the attack this
	// check exists to stop. Admins can write "https://api.example.com/${PATH}" instead.
	authority := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority = rest[:i]
	}
	// Only the part after the template's last literal '@' is the host the admin pinned; anything
	// before it is userinfo the admin wrote (as in "https://user:${PW}@api.example.com"), so a
	// value there may contain any character, including '@', without moving the host.
	hostTemplate := authority[strings.LastIndex(authority, "@")+1:]

	// Expanding the scheme and host on their own gives what the admin's structure allows. If a
	// value smuggled in a delimiter, the parser splits the URL elsewhere and the hosts differ:
	// '/', '?' or '#' end the host early, and an '@' in a host value becomes the URL's last '@',
	// turning the admin's host prefix (the "api-" in "api-${REGION}.example.com") into userinfo.
	// url.Parse lowercases the scheme, so a template written as "HTTPS://..." compares equal.
	//
	// Both errors name variables but never err or the URL: those contain the configured values,
	// which may be secrets (userinfo) and which these errors carry into API responses and logs.
	u, err := url.Parse(expanded)
	if err != nil {
		// Any value can make the URL unparseable (a non-numeric port, or an invalid escape such
		// as "%zz" in a path value), so name every variable in the template.
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "urlTemplate",
			Message: fmt.Sprintf("the values of %s do not form a valid URL",
				strings.Join(URLTemplateReferences(urlTemplate), ", ")),
		}
	}
	// An authority written as one variable and nothing else ("https://${SERVER}") is the admin
	// letting users choose any host, so there is nothing to pin, and a value such as
	// "tenant.example.com/api/mcp" that brings its own path, query or userinfo stays allowed. Any
	// literal text in the host ("${REGION}.api.example.com", "${HOST}:8443") is pinned.
	//
	// The whole authority must be that one variable, not just the part after the last '@'. In
	// "https://${TOKEN}@${HOST}/mcp" HOST may be admin-fixed while TOKEN is the user's, and a
	// TOKEN of "x@evil.example/" ends the authority early and moves the host to evil.example.
	// This function cannot tell who supplied which value, so userinfo before a one-variable host
	// keeps the host pinned.
	userChosenHost := hostTemplate == authority && authority != "" && envVarRegex.FindString(authority) == authority
	if !strings.EqualFold(u.Scheme, expandEnvVars(scheme, values, nil)) ||
		(!userChosenHost && u.Host != expandEnvVars(hostTemplate, values, nil)) {
		// Only a value in the scheme or authority can change them, so name just those variables.
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "urlTemplate",
			Message: fmt.Sprintf("the values of %s must not change the URL template's scheme or host structure",
				strings.Join(URLTemplateReferences(scheme+"://"+authority), ", ")),
		}
	}
	if userChosenHost {
		return nil
	}

	// The structure check above still lets a value put any text, dots included, where its
	// variable sits. That is harmless before admin-written domain text
	// ("${REGION}.api.example.com") but not when the value forms the registrable domain:
	// "api.lab.${TLD}" with TLD "attacker.example", or "api.${X}.com" with X "evil", sends the
	// entry's static headers to a domain the user owns. So the registrable domain (see
	// icannRegistrableDomain) must come entirely from literal text after the hostname's last
	// variable.
	//
	// The port is cut off first so a port variable ("api.example.com:${PORT}") doesn't count as
	// the hostname's last variable. A ':' before the last ']' is inside an IPv6 literal.
	hostnameTemplate := hostTemplate
	if i := strings.LastIndex(hostTemplate, ":"); i > strings.LastIndex(hostTemplate, "]") {
		hostnameTemplate = hostTemplate[:i]
	}
	domainErr := types.RuntimeValidationError{
		Runtime: types.RuntimeRemote,
		Field:   "urlTemplate",
		Message: fmt.Sprintf("the values of %s must not change the URL template's domain",
			strings.Join(URLTemplateReferences(hostnameTemplate), ", ")),
	}
	// No variables in the hostname ("localhost:${PORT}") leaves nothing for a value to change.
	// A hostname that is one variable ("${HOST}:8443", "${TOKEN}@${HOST}") is the admin letting
	// users choose the host, and only its structure is pinned above.
	if !strings.Contains(hostnameTemplate, "${") || envVarRegex.FindString(hostnameTemplate) == hostnameTemplate {
		return nil
	}
	// The checks below must see the host the HTTP client will dial, not the text as written.
	// net/http converts a non-ASCII host to ASCII before dialing, and that conversion turns
	// full-width dots ('。', '．', '｡') into '.', so "mcp-${ID}" with ID "x。attacker。example"
	// looks like one label here but dials mcp-x.attacker.example. A host that can't be
	// converted is rejected rather than skipped. A fully qualified name's trailing '.'
	// ("api.example.com.") names the same host, so it is dropped here and from the literal tail
	// below to keep it out of the domain lookup.
	hostname, err := dialedHostname(u.Hostname())
	if err != nil {
		return domainErr
	}
	hostname = strings.TrimSuffix(hostname, ".")
	// Remaining cases with no domain for a value to take over:
	//   - an IP address behind literal text ("10.0.0.${N}"), whose literal octets bound the
	//     result. A template that starts with a variable ("${A}.${B}") would let a user pick any
	//     IP, so it gets no exemption and fails the domain check below;
	//   - a single-label name ("mcp-${ID}"), which only resolves on the local network and that no
	//     one can register publicly.
	if (net.ParseIP(hostname) != nil && !strings.HasPrefix(hostnameTemplate, "${")) || !strings.Contains(hostname, ".") {
		return nil
	}
	// A leading '.' on both sides compares whole labels, so literal "example.com" right after a
	// variable doesn't match a value-made "evilexample.com". The tail is converted the same way
	// as the hostname, so an admin-written non-ASCII domain ("${R}.münchen.de") compares in the
	// form that is dialed. A lookup error (for example an empty label in "api..example.com")
	// rejects the value rather than skipping the check.
	literalTail := hostnameTemplate[strings.LastIndex(hostnameTemplate, "}")+1:]
	leadingDot := strings.HasPrefix(literalTail, ".")
	literalTail, err = dialedHostname(strings.TrimPrefix(literalTail, "."))
	if err != nil {
		return domainErr
	}
	if leadingDot {
		literalTail = "." + literalTail
	}
	literalTail = strings.TrimSuffix(literalTail, ".")
	domain, err := icannRegistrableDomain(hostname)
	if err != nil || !strings.HasSuffix("."+literalTail, "."+domain) {
		return domainErr
	}
	return nil
}

// dialedHostname returns host in the form net/http dials it: an ASCII host as is (lowercased
// for comparison), and any other host converted with IDNA's lookup profile, as net/http does.
// ASCII hosts skip the conversion because net/http skips it too, and its stricter rules would
// reject names net/http accepts, such as ones with '_'.
func dialedHostname(host string) (string, error) {
	if strings.IndexFunc(host, func(r rune) bool { return r >= utf8.RuneSelf }) < 0 {
		return strings.ToLower(host), nil
	}
	return idna.Lookup.ToASCII(host)
}

// icannRegistrableDomain returns the hostname's registrable domain: its ICANN public suffix plus
// one label ("example.com", "example.co.uk"). Provider-registered suffixes on the public suffix
// list, such as "azurewebsites.net" or "run.app", are skipped on purpose. A template like
// "https://${APP}.azurewebsites.net" names the platform it trusts, and letting the user pick a
// tenant on it is what such a template is for. An unlisted TLD ("lab.test") counts as a
// one-label suffix, as the list's default rule does.
func icannRegistrableDomain(hostname string) (string, error) {
	if hostname == "" || strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") || strings.Contains(hostname, "..") {
		return "", fmt.Errorf("invalid hostname")
	}
	suffix, icann := publicsuffix.PublicSuffix(hostname)
	// PublicSuffix returns the longest matching rule, so a private suffix comes back as itself.
	// Look up its parent to find the next shorter suffix, until an ICANN one or the last label.
	for !icann {
		_, parent, ok := strings.Cut(suffix, ".")
		if !ok {
			break
		}
		suffix, icann = publicsuffix.PublicSuffix(parent)
	}
	if hostname == suffix {
		return "", fmt.Errorf("hostname is a public suffix")
	}
	rest := hostname[:len(hostname)-len(suffix)-1]
	return rest[strings.LastIndex(rest, ".")+1:] + "." + suffix, nil
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

// ResolveURLTemplate expands a remote URL template using the supplied configuration alongside
// the manifest's own static config values, the same way the runtime does when it builds a
// ServerConfig. It takes the template and config fields rather than a manifest because system
// servers use a different manifest type. Static values override the supplied configuration, so
// a user cannot replace a value the admin fixed. It returns the names of every unresolved
// reference rather than a partially expanded URL, and an error if the values change the host
// the template pins.
//
// adminKeys names configuration keys whose values an admin set outside the manifest, such as a
// vMCP component's fixed configuration. Like static values and admin options, their text counts
// as literal template text in the host check.
func ResolveURLTemplate(urlTemplate string, config []types.MCPConfig, configuration map[string]string, adminKeys []string) (string, []string, error) {
	values := make(map[string]string, len(configuration)+len(config))
	maps.Copy(values, configuration)
	for _, c := range config {
		if c.Value != "" {
			values[c.Key] = c.Value
		}
	}

	adminValues := adminURLTemplateValues(config, values)
	for _, key := range adminKeys {
		if value, ok := values[key]; ok {
			adminValues[key] = value
		}
	}
	return expandURLTemplate(urlTemplate, values, adminValues)
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

func configureContainerizedRuntime(serverConfig *ServerConfig, containerizedConfig *types.ContainerizedRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}, expandImage bool) error {
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

	return nil
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
		//
		// A vMCP's fixed values don't count as admin-set here, because the manifest doesn't record
		// which keys the vMCP fixed. The vMCP controller does count them and stores the URL it
		// resolves before marking the server synced, and vMCP connections wait for that
		// (vmcpaccess.ServerConfigurationSynced in vmcp.go). So a vMCP connection only reaches
		// this path when the controller deliberately left the URL empty because it rejected the
		// values or some were missing, and refusing here as well is correct.
		expanded, missing, err := expandURLTemplate(remoteConfig.URLTemplate, credEnv, adminURLTemplateValues(config, credEnv))
		if err != nil {
			return nil, err
		}
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
		InjectObotHeaders:      slices.Clone(mcpServer.Spec.Manifest.InjectObotHeaders),
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
		if err := configureContainerizedRuntime(&serverConfig, mcpServer.Spec.Manifest.ContainerizedConfig, runtimeCredEnv, fileEnvVars, true); err != nil {
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
		if err := configureContainerizedRuntime(&serverConfig, systemServer.Spec.Manifest.ContainerizedConfig, credEnv, fileEnvVars, false); err != nil {
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
