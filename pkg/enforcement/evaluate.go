package enforcement

import (
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const defaultUnresolvedReason = "the device could not determine what this tool call targets"

// Evaluate decides whether call is permitted by allowlist. It is fail-closed:
// anything that does not positively match an allow rule is denied.
func Evaluate(call NormalizedCall, allowlist types.EnforcementAllowlist) Decision {
	if call.Unresolved {
		reason := strings.TrimSpace(call.UnresolvedReason)
		if reason == "" {
			reason = defaultUnresolvedReason
		}
		return Decision{Allow: false, Reason: reason}
	}

	// Short-circuit: allow everything.
	if allowlist.AllowEverything {
		return Decision{Allow: true, Reason: "allow-everything toggle is enabled"}
	}

	if allowlist.AllowAllBuiltinAgentTools && isBuiltinAgentToolKind(call.Kind) {
		return Decision{Allow: true, Reason: "built-in agent tools are allowed"}
	}

	if call.Kind == KindMCP {
		// Coarse: Obot-hosted MCP servers.
		if allowlist.AllowAllObotHostedMCP && call.ObotHosted {
			return Decision{Allow: true, Reason: "Obot-hosted MCP servers are allowed"}
		}

		// Coarse: built-in agent MCP servers.
		if allowlist.AllowAllBuiltinAgentMCP && isBuiltinAgentMCP(call.Agent, call.ServerName) {
			return Decision{Allow: true, Reason: "built-in agent MCP servers are allowed"}
		}

		for _, server := range allowlist.Servers {
			if serverMatches(call, server) && toolMatches(call, server) {
				return Decision{Allow: true, Reason: "matched an allowlisted server entry"}
			}
		}
	}

	// Fail-closed default.
	return Decision{Allow: false, Reason: "no matching allowlist entry"}
}

// serverMatches reports whether the call's resolved server matches the single
// dimension declared on the allowlist entry (URL, package, hostname, or
// connector).
func serverMatches(call NormalizedCall, entry types.AllowlistServer) bool {
	switch {
	case entry.URL != "":
		return urlMatches(entry.URL, call.Server.URL)
	case entry.Package != nil:
		return packageMatches(entry.Package, call.Server.Package)
	case entry.Hostname != "":
		return hostnameMatches(entry.Hostname, callHostname(call))
	case entry.Connector != "":
		return connectorMatches(entry.Connector, call.Server.Connector)
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
	return slices.Contains(entry.Tools, call.Tool)
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
	if NormalizedPort(entry) != NormalizedPort(actual) {
		return false
	}

	return pathPrefixMatches(entry.Path, actual.Path)
}

// NormalizedPort returns the explicit port or the scheme's default port.
func NormalizedPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
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

func connectorMatches(entryConnector, callConnector string) bool {
	if callConnector == "" {
		return false
	}
	return strings.EqualFold(entryConnector, callConnector)
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
