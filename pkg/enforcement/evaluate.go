package enforcement

import (
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Evaluate decides whether call is permitted by allowlist. It is fail-closed:
// anything that does not positively match an allow rule is denied.
func Evaluate(call NormalizedCall, allowlist types.EnforcementAllowlist) Decision {
	// Short-circuit: allow everything.
	if allowlist.AllowEverything {
		return Decision{Allow: true, Reason: "allow-everything toggle is enabled"}
	}

	isMCP := call.Kind == KindMCP

	// Coarse: any non-MCP built-in agent tool (shell/read/write/task/generic).
	if allowlist.AllowAllBuiltinAgentTools && !isMCP {
		return Decision{Allow: true, Reason: "built-in agent tools are allowed"}
	}

	if isMCP {
		// Coarse: Obot-hosted MCP servers.
		if allowlist.AllowAllObotHostedMCP && call.ObotHosted {
			return Decision{Allow: true, Reason: "Obot-hosted MCP servers are allowed"}
		}

		// Coarse: built-in agent MCP servers.
		if allowlist.AllowAllBuiltinAgentMCP && isBuiltinAgentMCP(call.Agent, call.ServerName) {
			return Decision{Allow: true, Reason: "built-in agent MCP servers are allowed"}
		}
	}

	// Specific server entries.
	for _, server := range allowlist.Servers {
		if serverMatches(call, server) && toolMatches(call, server) {
			return Decision{Allow: true, Reason: "matched an allowlisted server entry"}
		}
	}

	// Fail-closed default.
	return Decision{Allow: false, Reason: "no matching allowlist entry"}
}

// serverMatches reports whether the call's resolved server matches the single
// dimension declared on the allowlist entry (URL, package, or hostname).
func serverMatches(call NormalizedCall, entry types.AllowlistServer) bool {
	switch {
	case entry.URL != "":
		return urlMatches(entry.URL, call.Server.URL)
	case entry.Package != nil:
		return packageMatches(entry.Package, call.Server.Package)
	case entry.Hostname != "":
		return hostnameMatches(entry.Hostname, callHostname(call))
	default:
		// A malformed entry with no dimension set matches nothing.
		return false
	}
}

// toolMatches reports whether the call's tool is permitted by the entry. An
// empty Tools list means every tool on the server is allowed.
func toolMatches(call NormalizedCall, entry types.AllowlistServer) bool {
	if len(entry.Tools) == 0 {
		return true
	}
	for _, tool := range entry.Tools {
		if tool == call.Tool {
			return true
		}
	}
	return false
}

// urlMatches compares a call URL against an allowlisted URL by scheme, host, and
// normalized port, plus an optional path-prefix constraint enforced at a path
// boundary.
func urlMatches(entryURL, callURL string) bool {
	if callURL == "" {
		return false
	}

	entry, err := url.Parse(entryURL)
	if err != nil {
		return false
	}
	actual, err := url.Parse(callURL)
	if err != nil {
		return false
	}

	if !strings.EqualFold(entry.Scheme, actual.Scheme) {
		return false
	}
	if !strings.EqualFold(entry.Hostname(), actual.Hostname()) {
		return false
	}
	if normalizedPort(entry) != normalizedPort(actual) {
		return false
	}

	return pathPrefixMatches(entry.Path, actual.Path)
}

// normalizedPort returns the explicit port or the scheme's default port.
func normalizedPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "wss":
		return "443"
	case "http", "ws":
		return "80"
	default:
		return ""
	}
}

// pathPrefixMatches reports whether callPath is equal to, or a path-boundary
// descendant of, entryPath. An empty (or "/") entry path imposes no constraint.
func pathPrefixMatches(entryPath, callPath string) bool {
	entryPath = strings.TrimSuffix(entryPath, "/")
	if entryPath == "" {
		return true
	}
	callPath = strings.TrimSuffix(callPath, "/")
	if callPath == entryPath {
		return true
	}
	return strings.HasPrefix(callPath, entryPath+"/")
}

// packageMatches compares a resolved package identity against an allowlisted
// package. Source and name must match exactly; an empty allowlist version
// accepts any version.
func packageMatches(entry *types.AllowlistServerPackage, actual *PackageIdentity) bool {
	if entry == nil || actual == nil {
		return false
	}
	if entry.Source != actual.Source {
		return false
	}
	if entry.Name != actual.Name {
		return false
	}
	if entry.Version == "" {
		return true
	}
	return entry.Version == actual.Version
}

// hostnameMatches compares hostnames case-insensitively.
func hostnameMatches(entryHost, callHost string) bool {
	if callHost == "" {
		return false
	}
	return strings.EqualFold(entryHost, callHost)
}

// callHostname returns the call's hostname, deriving it from the resolved URL
// when it was not set explicitly.
func callHostname(call NormalizedCall) string {
	if call.Server.Hostname != "" {
		return call.Server.Hostname
	}
	if call.Server.URL == "" {
		return ""
	}
	u, err := url.Parse(call.Server.URL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
