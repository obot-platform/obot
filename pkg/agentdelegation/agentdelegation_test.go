package agentdelegation

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// s256 returns the hash an agent sends for secret.
func s256(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestNormalizeAllowedReturnHosts(t *testing.T) {
	hosts, err := NormalizeAllowedReturnHosts([]string{"Agent.Example.com", "agent.example.com:8443", "localhost:3000", "127.0.0.1", "[::1]:8080", " [::1] "})
	require.NoError(t, err)
	require.Equal(t, []string{"agent.example.com", "agent.example.com:8443", "localhost:3000", "127.0.0.1", "[::1]:8080", "[::1]"}, hosts)

	hosts, err = NormalizeAllowedReturnHosts(nil)
	require.NoError(t, err)
	require.Nil(t, hosts)

	for _, tt := range []struct {
		name  string
		hosts []string
	}{
		{
			name:  "empty",
			hosts: []string{""},
		},
		{
			name:  "a scheme",
			hosts: []string{"https://agent.example.com"},
		},
		{
			name:  "a path",
			hosts: []string{"agent.example.com/callback"},
		},
		{
			name:  "a query",
			hosts: []string{"agent.example.com?x=1"},
		},
		{
			name:  "a user",
			hosts: []string{"user@agent.example.com"},
		},
		{
			name:  "a wildcard",
			hosts: []string{"*.example.com"},
		},
		{
			name:  "a port that is not a number",
			hosts: []string{"agent.example.com:https"},
		},
		{
			name:  "a port out of range",
			hosts: []string{"agent.example.com:70000"},
		},
		{
			name:  "an empty port",
			hosts: []string{"agent.example.com:"},
		},
		{
			name:  "an IPv6 address without brackets",
			hosts: []string{"::1"},
		},
		{
			name:  "a space",
			hosts: []string{"agent example.com"},
		},
		{
			name:  "a duplicate, in another case",
			hosts: []string{"agent.example.com", "AGENT.example.com"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeAllowedReturnHosts(tt.hosts)
			require.Error(t, err)
		})
	}
}

func TestCheckReturnURL(t *testing.T) {
	allowed := []string{"agent.example.com", "agent.example.com:8443", "localhost:3000", "127.0.0.1", "[::1]:8080"}

	for _, tt := range []struct {
		name      string
		returnURL string
		allowed   []string
		wantErr   string
	}{
		{
			name:      "an allowed host",
			returnURL: "https://agent.example.com/callback?session=1",
		},
		{
			name:      "an allowed host on its default port",
			returnURL: "https://AGENT.example.com:443/callback",
		},
		{
			name:      "an allowed host and port",
			returnURL: "https://agent.example.com:8443/callback",
		},
		{
			name:      "an allowed host on a port it does not list",
			returnURL: "https://agent.example.com:9443/callback",
			wantErr:   "not an allowed return host",
		},
		{
			name:      "another host",
			returnURL: "https://evil.example.com/callback",
			wantErr:   "not an allowed return host",
		},
		{
			name:      "a subdomain of an allowed host",
			returnURL: "https://x.agent.example.com/callback",
			wantErr:   "not an allowed return host",
		},
		{
			name:      "http to an allowed host that is not a loopback",
			returnURL: "http://agent.example.com/callback",
			wantErr:   "must use https",
		},
		{
			name:      "http to localhost on the list, for development",
			returnURL: "http://localhost:3000/callback",
		},
		{
			name:      "http to a loopback address on the list",
			returnURL: "http://127.0.0.1/callback",
		},
		{
			name:      "http to an IPv6 loopback address on the list",
			returnURL: "http://[::1]:8080/callback",
		},
		{
			name:      "http to localhost on a port not on the list",
			returnURL: "http://localhost:3001/callback",
			wantErr:   "not an allowed return host",
		},
		{
			name:      "http to a loopback address that is not on the list",
			returnURL: "http://127.0.0.2/callback",
			wantErr:   "not an allowed return host",
		},
		{
			name:      "https to a loopback address on the list",
			returnURL: "https://127.0.0.1/callback",
		},
		{
			name:      "an agent with no allowed hosts",
			returnURL: "https://agent.example.com/callback",
			allowed:   []string{},
			wantErr:   "no allowed return hosts",
		},
		{
			name:      "another scheme",
			returnURL: "javascript://agent.example.com/%0aalert(1)",
			wantErr:   "must use https",
		},
		{
			name:      "a relative URL",
			returnURL: "/callback",
			wantErr:   "absolute",
		},
		{
			name:      "a user and password",
			returnURL: "https://user:pass@agent.example.com/callback",
			wantErr:   "user name or password",
		},
		{
			name:      "a fragment",
			returnURL: "https://agent.example.com/callback#done",
			wantErr:   "fragment",
		},
		{
			name:      "too long",
			returnURL: "https://agent.example.com/" + strings.Repeat("a", maxReturnURLLength),
			wantErr:   "longer than",
		},
		{
			name:      "empty",
			returnURL: "",
			wantErr:   "required",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hosts := allowed
			if tt.allowed != nil {
				hosts = tt.allowed
			}
			err := CheckReturnURL(tt.returnURL, hosts)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
	require.ErrorIs(t, CheckReturnURL("https://agent.example.com/callback", nil), ErrNoAllowedReturnHosts)
}

func TestRedirectURL(t *testing.T) {
	redirect, err := RedirectURL("https://agent.example.com/callback?session=1&code=stale", map[string]string{
		"requestID": "adr1abc",
		"code":      "fresh",
	})
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	require.Equal(t, "agent.example.com", u.Host)
	require.Equal(t, "/callback", u.Path)
	require.Equal(t, url.Values{"session": {"1"}, "requestID": {"adr1abc"}, "code": {"fresh"}}, u.Query())

	host, err := ReturnHost("https://Agent.Example.com:8443/callback?secret=1")
	require.NoError(t, err)
	require.Equal(t, "agent.example.com:8443", host)
}

func TestSecretHash(t *testing.T) {
	secret := strings.Repeat("s", 43)
	hash := s256(secret)
	require.NoError(t, CheckSecretHash(hash))
	require.True(t, SecretMatches(secret, hash))

	// Another secret, or the hash itself, does not match.
	require.False(t, SecretMatches(strings.Repeat("t", 43), hash))
	require.False(t, SecretMatches(hash, hash))

	// A secret too short or too long to be a PKCE verifier never matches, even its own hash, and neither does one
	// with characters a verifier cannot have.
	for _, weak := range []string{"short", strings.Repeat("s", 129), strings.Repeat("s", 42) + "!"} {
		require.False(t, SecretMatches(weak, s256(weak)), weak)
	}
	every := strings.Repeat("aZ09-._~", 16)
	require.True(t, SecretMatches(every, s256(every)))

	for _, malformed := range []string{
		"",
		"plain",
		hash + "=",
		hash[:42],
		// A character of standard base64 rather than base64url.
		"+" + hash[1:],
		strings.Repeat("!", 43),
	} {
		require.Error(t, CheckSecretHash(malformed), malformed)
	}
}

func TestCheckLoginHint(t *testing.T) {
	require.NoError(t, CheckLoginHint(""))
	require.NoError(t, CheckLoginHint("alice@example.com"))
	require.Error(t, CheckLoginHint(strings.Repeat("a", maxLoginHintLength+1)))
	require.Error(t, CheckLoginHint("alice\r\nSet-Cookie: x"))
}
