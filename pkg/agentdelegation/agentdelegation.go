// Package agentdelegation holds the rules of the approval protocol through which a user lets an agent act on their
// behalf (adr/2026-11-20-agent-on-behalf-of-users.md): the hosts an agent may send a user's browser back to, the
// secret that binds an approval to the browser session that asked for it, and the hook that removes what a user set up
// through an agent. It is pure: its callers read and write storage.
package agentdelegation

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	// maxReturnURLLength bounds a return URL, which Obot stores and sends a browser to.
	maxReturnURLLength = 2048
	// maxLoginHintLength bounds a login hint, which Obot stores and passes on to sign-in.
	maxLoginHintLength = 256

	// secretHashLength is the length of a SHA-256 digest in unpadded base64url.
	secretHashLength = 43
	// minSecretLength and maxSecretLength are the lengths RFC 7636 allows a PKCE code verifier, which the secret is
	// the equivalent of. The minimum keeps a secret too short to be guessed out.
	minSecretLength = 43
	maxSecretLength = 128
)

var (
	// ErrNoAllowedReturnHosts reports that an agent has no allowed return hosts, so it cannot ask users for approval.
	ErrNoAllowedReturnHosts = errors.New("the agent has no allowed return hosts; an administrator must add the host of the return URL to the agent before it can ask users for approval")
)

// Scope names whose setup through agents to remove: that of one user through one agent when both IDs are set, that of
// every user through an agent when only AgentID is, and that of a user through every agent when only UserID is.
type Scope struct {
	AgentID uint
	UserID  uint
}

// Cleanup removes what users set up through agents (owned by principal.DelegatedOwner): instances, credentials and
// upstream OAuth tokens. It is called when a delegation is revoked, after the delegation is deleted, and when an agent
// or a user is deleted, before their delegations are. It must be idempotent, since a cleanup that fails is retried,
// and it must remove setup whether or not a delegation still exists for it: setup can be made while a request is
// pending, before any delegation does.
type Cleanup interface {
	CleanupDelegations(ctx context.Context, scope Scope) error
}

// NoCleanup is the Cleanup of a deployment in which nothing is set up through agents.
type NoCleanup struct{}

// NormalizeAllowedReturnHosts checks the allowed return hosts an administrator gives an agent, and returns them in
// canonical form: lower case, in the order given. Each is a host name or IP address, with an optional port, and
// nothing else: no scheme, path, user or wildcard. An IPv6 address with a port is written in brackets. Duplicates are
// refused.
func NormalizeAllowedReturnHosts(hosts []string) ([]string, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(hosts))
	for _, host := range hosts {
		canonical, err := normalizeHost(host)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed return host %q: %w", host, err)
		}
		if slices.Contains(normalized, canonical) {
			return nil, fmt.Errorf("duplicate allowed return host %q", canonical)
		}
		normalized = append(normalized, canonical)
	}
	return normalized, nil
}

// normalizeHost returns host, an allowed return host, in canonical form.
func normalizeHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", errors.New("it is empty")
	}
	// A host is parsed as the authority of a URL, so that it is read the way a return URL's host is.
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(host, "/?#@*") {
		return "", errors.New("it must be a host name or IP address, with an optional port, and nothing else")
	}
	name := u.Hostname()
	if name == "" {
		return "", errors.New("it has no host name")
	}
	if strings.Contains(name, ":") && net.ParseIP(name) == nil {
		return "", errors.New("it is not a valid IPv6 address")
	}
	if port := u.Port(); port != "" || strings.HasSuffix(host, ":") {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", errors.New("its port must be a number from 1 to 65535")
		}
	}
	return host, nil
}

// CheckReturnURL checks the URL that a user's browser is sent back to after the user approves or cancels a request,
// against the allowed return hosts of the agent that asked: it must be an absolute https URL whose host is on the list.
// Plain http is allowed only to localhost or a loopback address that is itself on the list, for development. A host
// on the list without a port allows only the default port of the URL's scheme. Without allowed hosts, every URL is
// refused (ErrNoAllowedReturnHosts).
//
// Without this check, anyone with the agent's key could use Obot to send a signed-in user's browser anywhere.
func CheckReturnURL(returnURL string, allowedHosts []string) error {
	if len(allowedHosts) == 0 {
		return ErrNoAllowedReturnHosts
	}
	if returnURL == "" {
		return errors.New("returnURL is required")
	}
	if len(returnURL) > maxReturnURLLength {
		return fmt.Errorf("returnURL is longer than %d characters", maxReturnURLLength)
	}
	u, err := url.Parse(returnURL)
	if err != nil {
		return fmt.Errorf("returnURL is not a valid URL: %w", err)
	}
	if !u.IsAbs() || u.Host == "" || u.Opaque != "" {
		return errors.New("returnURL must be an absolute URL")
	}
	if u.User != nil {
		return errors.New("returnURL must not carry a user name or password")
	}
	if u.Fragment != "" || strings.Contains(returnURL, "#") {
		return errors.New("returnURL must not have a fragment")
	}

	var defaultPort string
	switch u.Scheme {
	case "https":
		defaultPort = "443"
	case "http":
		if !isLoopback(u.Hostname()) {
			return errors.New("returnURL must use https; plain http is allowed only to localhost or a loopback address")
		}
		defaultPort = "80"
	default:
		return errors.New("returnURL must use https")
	}

	if !slices.ContainsFunc(allowedHosts, func(allowed string) bool { return hostMatches(allowed, u, defaultPort) }) {
		return fmt.Errorf("the host %q of returnURL is not an allowed return host of the agent", u.Host)
	}
	return nil
}

// hostMatches reports whether allowed, an allowed return host in canonical form, is the host of u, whose scheme's
// default port is defaultPort. A host with no port is on the scheme's default port.
func hostMatches(allowed string, u *url.URL, defaultPort string) bool {
	entry, err := url.Parse("https://" + allowed)
	if err != nil {
		return false
	}
	entryPort, urlPort := entry.Port(), u.Port()
	if entryPort == "" {
		entryPort = defaultPort
	}
	if urlPort == "" {
		urlPort = defaultPort
	}
	return strings.EqualFold(entry.Hostname(), u.Hostname()) && entryPort == urlPort
}

// isLoopback reports whether host, a URL's host name without its port, is localhost or a loopback IP address.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ReturnHost returns the host, with its port if it has one, of returnURL, which CheckReturnURL accepted. It is what
// may be recorded of a return URL, whose path and query are the agent's and could hold anything.
func ReturnHost(returnURL string) (string, error) {
	u, err := url.Parse(returnURL)
	if err != nil {
		return "", err
	}
	return strings.ToLower(u.Host), nil
}

// RedirectURL returns returnURL, which CheckReturnURL accepted, with params added to its query. They replace any
// parameters of the same names that it already has.
func RedirectURL(returnURL string, params map[string]string) (string, error) {
	u, err := url.Parse(returnURL)
	if err != nil {
		return "", err
	}
	query := u.Query()
	for name, value := range params {
		query.Set(name, value)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// CheckSecretHash checks the hash of the secret that an agent keeps in a user's session while it asks for the user's
// approval: the SHA-256 digest of the secret, in unpadded base64url, as a PKCE S256 code challenge is.
func CheckSecretHash(secretHash string) error {
	if len(secretHash) != secretHashLength {
		return fmt.Errorf("secretHash must be the SHA-256 digest of the secret, in unpadded base64url (%d characters)", secretHashLength)
	}
	digest, err := base64.RawURLEncoding.Strict().DecodeString(secretHash)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("secretHash must be the SHA-256 digest of the secret, in unpadded base64url")
	}
	return nil
}

// SecretMatches reports whether secret is the secret that secretHash is the hash of (CheckSecretHash). A secret that
// is not 43 to 128 characters of the unreserved URL characters (RFC 3986: A-Z, a-z, 0-9, "-", ".", "_" and "~"), as a
// PKCE code verifier is (RFC 7636), never matches.
func SecretMatches(secret, secretHash string) bool {
	if len(secret) < minSecretLength || len(secret) > maxSecretLength || strings.ContainsFunc(secret, func(c rune) bool { return !isUnreserved(c) }) {
		return false
	}
	digest := sha256.Sum256([]byte(secret))
	computed := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(secretHash)) == 1
}

// isUnreserved reports whether c is one of the unreserved URL characters (RFC 3986), which a PKCE code verifier is made
// of.
func isUnreserved(c rune) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	default:
		return c == '-' || c == '.' || c == '_' || c == '~'
	}
}

// CheckLoginHint checks the optional login hint an agent passes on to sign-in. It is a convenience, never a check of
// who signs in.
func CheckLoginHint(loginHint string) error {
	if len(loginHint) > maxLoginHintLength {
		return fmt.Errorf("loginHint is longer than %d characters", maxLoginHintLength)
	}
	if strings.ContainsFunc(loginHint, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("loginHint must not hold control characters")
	}
	return nil
}

// CleanupDelegations does nothing.
func (NoCleanup) CleanupDelegations(context.Context, Scope) error {
	return nil
}
