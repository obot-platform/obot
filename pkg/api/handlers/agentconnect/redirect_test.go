package agentconnect

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// modifyLocation is the rewrite the proxy applies, extracted so its behaviour
// can be checked without standing up a sandbox.
func modifyLocation(prefix, basePath, location string) string {
	resp := &http.Response{Header: http.Header{}}
	if location != "" {
		resp.Header.Set("Location", location)
	}
	parsed, err := url.Parse(location)
	if location == "" || err != nil || parsed.IsAbs() {
		return resp.Header.Get("Location")
	}
	path := strings.TrimPrefix(parsed.Path, basePath)
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		path = prefix + "/" + strings.TrimPrefix(path, "/")
	}
	parsed.Path = path
	return parsed.String()
}

// An agent redirecting to a path on itself must land back inside
// /agent-connect. Left alone the browser resolves it against Obot's host and
// leaves the agent entirely.
func TestRelativeRedirectStaysInsideTheProxy(t *testing.T) {
	const prefix = "/agent-connect/hai1abc"

	for _, tt := range []struct{ name, basePath, in, want string }{
		{
			name: "production, agent redirects to its own path",
			in:   "/dev-ui/",
			want: prefix + "/dev-ui/",
		},
		{
			// The API server's service proxy rewrites the agent's relative
			// redirect into its own absolute path on the way through.
			name:     "through the API server proxy in development",
			basePath: "/api/v1/namespaces/obot-mcp/services/http:obot-agent-x:80/proxy",
			in:       "/api/v1/namespaces/obot-mcp/services/http:obot-agent-x:80/proxy/dev-ui/",
			want:     prefix + "/dev-ui/",
		},
		{
			name: "query and fragment survive",
			in:   "/dev-ui/?app=chat#top",
			want: prefix + "/dev-ui/?app=chat#top",
		},
		{
			name: "root",
			in:   "/",
			want: prefix + "/",
		},
		{
			// An agent that knows its own prefix emits it already. Prepending
			// again yields /agent-connect/{id}/agent-connect/{id}/…, which is
			// what happened once ADK was given --url_prefix.
			name: "already prefixed by the agent",
			in:   prefix + "/dev-ui/",
			want: prefix + "/dev-ui/",
		},
		{
			name: "already prefixed, exactly the prefix",
			in:   prefix,
			want: prefix,
		},
		{
			// A path that merely starts with the same characters is not
			// prefixed, and must still be rewritten.
			name: "lookalike path is still rewritten",
			in:   prefix + "-other/thing",
			want: prefix + "/" + strings.TrimPrefix(prefix, "/") + "-other/thing",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := modifyLocation(prefix, tt.basePath, tt.in); got != tt.want {
				t.Errorf("Location %q -> %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// An absolute redirect points somewhere else deliberately -- an OAuth provider,
// say -- and rewriting it would break the flow it belongs to.
func TestAbsoluteRedirectIsLeftAlone(t *testing.T) {
	const external = "https://accounts.example.com/authorize?client_id=x"
	if got := modifyLocation("/agent-connect/hai1abc", "", external); got != external {
		t.Errorf("Location = %q, want it untouched", got)
	}
}

// A response with no redirect must not grow one.
func TestNoLocationHeaderIsUntouched(t *testing.T) {
	if got := modifyLocation("/agent-connect/hai1abc", "", ""); got != "" {
		t.Errorf("Location = %q, want none", got)
	}
}
