package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"

	"github.com/obot-platform/obot/pkg/oauth"
)

const (
	// immutablePrefix is where the UI build puts its content-hashed assets.
	immutablePrefix = "/_app/immutable/"
)

var (
	//go:embed all:user/*build
	embedded embed.FS
)

type uiServer struct {
	rp       *httputil.ReverseProxy
	userOnly bool
	// fsys holds the built UI. It is a field so tests can supply a stub in place
	// of the embedded build, which only exists after `make ui`.
	fsys fs.FS
}

func Handler(devPort, userOnlyPort int) http.Handler {
	server := &uiServer{fsys: embedded}

	if userOnlyPort != 0 {
		server.rp = newUIProxy(userOnlyPort)
		server.userOnly = true
	} else if devPort != 0 {
		server.rp = newUIProxy(devPort)
	}

	return server
}

// newUIProxy proxies to a UI server on localhost. SetXForwarded keeps the
// X-Forwarded-For behavior that ReverseProxy used to apply automatically under
// the deprecated Director.
func newUIProxy(port int) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetXForwarded()
			r.Out.URL.Scheme = "http"
			r.Out.URL.Host = fmt.Sprintf("localhost:%d", port)
		},
	}
}

// serveHTML serves one of the UI's HTML entry points. Each one names the
// content-hashed assets of the build it came from, and those assets only exist
// in that build's binary, so a stale copy asks for chunks the running binary
// does not have. no-cache still lets a client hold onto it, but forces a
// revalidation before it is used.
func (s *uiServer) serveHTML(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.fsys, name)
}

func (s *uiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Always include the X-Frame-Options header
	w.Header().Set("X-Frame-Options", "DENY")

	if oauth.HandleOAuthRedirect(w, r) {
		return
	}

	if s.rp != nil && (!s.userOnly || !strings.HasPrefix(r.URL.Path, "/admin")) {
		s.rp.ServeHTTP(w, r)
		return
	}

	userPath := path.Join("user/build/", r.URL.Path)

	if r.URL.Path == "/" {
		s.serveHTML(w, r, "user/build/index.html")
	} else if r.URL.Path == "/admin" {
		s.serveHTML(w, r, "user/build/admin.html")
	} else if r.URL.Path == "/admin/" {
		// we have to redirect to /admin instead of serving the index.html file because ending slash will laod a different route for js files
		http.Redirect(w, r, "/admin", http.StatusFound)
	} else if r.URL.Path == "/mcp-servers/" {
		http.Redirect(w, r, "/mcp-servers", http.StatusFound)
	} else if r.URL.Path == "/mcp-servers" {
		s.serveHTML(w, r, "user/build/mcp-servers.html")
	} else if pathWithoutTrailingSlash, ok := strings.CutSuffix(r.URL.Path, "/"); ok {
		// Paths with trailing slashes should redirect to without slash to avoid directory listings
		http.Redirect(w, r, pathWithoutTrailingSlash, http.StatusFound)
	} else if _, err := fs.Stat(s.fsys, userPath+".html"); err == nil {
		// Try .html version first (for SvelteKit prerendered pages)
		s.serveHTML(w, r, userPath+".html")
	} else if _, err := fs.Stat(s.fsys, userPath); err == nil {
		if strings.HasPrefix(r.URL.Path, immutablePrefix) {
			// These filenames carry a hash of their contents, so what a given URL
			// returns can never change and the client never has to ask again.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, s.fsys, userPath)
	} else if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		// Non-browser clients get a real 404 for unknown paths rather than the SPA
		// fallback, so a mistyped API path doesn't return HTML with a 200. no-store
		// keeps CDNs and browsers from caching this against a URL that may exist on
		// the next deploy.
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
	} else {
		s.serveHTML(w, r, "user/build/fallback.html")
	}
}
