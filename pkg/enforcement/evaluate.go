package enforcement

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	defaultUnresolvedReason = "the device could not determine what this tool call targets"

	// MaxMatchedPathBytes bounds the path of a URL that URL entries will consider.
	// Resolving a path costs memory in proportion to its segments, and a device can
	// report a URL as large as the request body, so a longer one matches no URL entry.
	// The query and fragment are cut off first, because they are never matched.
	MaxMatchedPathBytes = 64 << 10
)

var (
	// percentEncodedDot decodes every encoded ".". WHATWG URL parsers treat "%2e" as "." in
	// dot-segments, and elsewhere it is an encoded unreserved character, which RFC 3986
	// section 6.2.2.2 makes equivalent to ".". It never adds or removes a "/", so it cannot
	// move a path outside an allowed prefix.
	percentEncodedDot = strings.NewReplacer("%2e", ".", "%2E", ".")
)

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
	// The query and fragment are never matched, and the decision log drops them,
	// so cut them off before a long one is copied and parsed once per entry.
	if i := strings.IndexAny(callURL, "?#"); i >= 0 {
		callURL = callURL[:i]
	}
	if callURL == "" || len(callURL) > MaxMatchedPathBytes {
		return false
	}

	// An entry saved before the bound was enforced may have a longer path. It is
	// treated as matching nothing, and skipped before the cost of resolving it.
	entry, entryPath, err := ParseClientURL(entryURL)
	if err != nil || len(entryPath) > MaxMatchedPathBytes {
		return false
	}
	actual, actualPath, err := ParseClientURL(callURL)
	if err != nil || len(actualPath) > MaxMatchedPathBytes {
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

	// An entry saved with dot-segments would resolve to a different, often wider,
	// prefix than the one written, so it matches nothing.
	if HasDotSegments(entryPath) {
		return false
	}
	return pathPrefixMatches(ResolvePath(entryPath), ResolvePath(actualPath))
}

// ParseClientURL parses rawURL the way browsers and Node parse it and returns its path
// escaped as such a client sends it. For an http(s) URL, a literal backslash in the path is
// read as "/"; an encoded backslash ("%5C") is not. A literal backslash in the authority, or
// ending it, is an error, because clients disagree on which host it names. The path is taken
// from rawURL rather than from url.URL.EscapedPath, which decodes "%2F" into "/" whenever
// the path also holds a character Go would escape.
func ParseClientURL(rawURL string) (*url.URL, string, error) {
	scheme, _, found := strings.Cut(rawURL, ":")
	if !found || (!strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https")) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, "", err
		}
		return u, u.EscapedPath(), nil
	}

	end := strings.IndexAny(rawURL, "?#")
	if end < 0 {
		end = len(rawURL)
	}
	normalized := strings.ReplaceAll(rawURL[:end], `\`, "/") + rawURL[end:]
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, "", err
	}

	hierarchy := normalized[len(scheme)+1 : end]
	if !strings.HasPrefix(hierarchy, "//") {
		return u, u.EscapedPath(), nil
	}
	authority, path, found := strings.Cut(hierarchy[2:], "/")
	// A backslash in the authority, or ending it, is a separator to WHATWG parsers but
	// part of the userinfo or host to others, so "https://a.com\@b.com" reaches a.com
	// from one client and b.com from another. Neither host can be trusted to match.
	authorityEnd := min(len(scheme)+3+len(authority)+1, end)
	if strings.Contains(rawURL[:authorityEnd], `\`) {
		return nil, "", errors.New("url has a backslash in its authority")
	}
	if !found {
		return u, "", nil
	}
	return u, escapeClientPath("/" + path), nil
}

// escapeClientPath percent-encodes the bytes in path that WHATWG URL parsers encode in a
// path, and leaves every existing escape as written.
func escapeClientPath(path string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(path))
	for i := 0; i < len(path); i++ {
		if c := path[i]; c <= ' ' || c > '~' || strings.IndexByte("\"<>^`{}", c) >= 0 {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xF])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// HasDotSegments reports whether resolving escapedPath with ResolvePath does more than
// decode "%2e", i.e. whether it holds a "." or ".." segment, written plainly or encoded.
func HasDotSegments(escapedPath string) bool {
	return ResolvePath(escapedPath) != percentEncodedDot.Replace(escapedPath)
}

// ResolvePath returns an escaped path from ParseClientURL with dot-segments removed the way
// browsers and Node resolve them, which is the path an MCP client actually requests. Empty
// segments are kept and encoded slashes stay encoded, so "/a//../b" becomes "/a/b", not
// "/b", and "/a/..//b" becomes "//b", not "/b".
func ResolvePath(escapedPath string) string {
	escaped := percentEncodedDot.Replace(escapedPath)
	if !strings.HasPrefix(escaped, "/") {
		return escaped
	}

	segments := strings.Split(escaped[1:], "/")
	resolved := make([]string, 0, len(segments))
	for i, segment := range segments {
		last := i == len(segments)-1
		switch segment {
		case "..":
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
			if last {
				resolved = append(resolved, "")
			}
		case ".":
			if last {
				resolved = append(resolved, "")
			}
		default:
			resolved = append(resolved, segment)
		}
	}
	return "/" + strings.Join(resolved, "/")
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
// Both paths must already be escaped and resolved with ResolvePath.
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
