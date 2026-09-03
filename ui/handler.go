package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"io"
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
	// etags maps a path in fsys to that file's content hash, used as its ETag.
	etags map[string]string
}

func Handler(devPort, userOnlyPort int) http.Handler {
	server := newUIServer(embedded)

	if userOnlyPort != 0 {
		server.rp = newUIProxy(userOnlyPort)
		server.userOnly = true
	} else if devPort != 0 {
		server.rp = newUIProxy(devPort)
	}

	return server
}

func newUIServer(fsys fs.FS) *uiServer {
	return &uiServer{fsys: fsys, etags: buildETags(fsys)}
}

// buildETags hashes every file in fsys once, at startup, so each one can be
// served with an ETag. Nothing else gives these responses a validator, because
// Go takes Last-Modified from FileInfo.ModTime, every file in an embed.FS
// reports the zero time, and ServeContent omits the header for a zero time.
// Without a validator a client that already holds a copy still has to download
// the whole body to revalidate it, which is what the HTML pays on every
// navigation, since no-cache makes it revalidate before each use.
//
// A content hash is the right validator here because it is the same on every
// replica running the same build and changes only when the file changes. The
// alternative is a timestamp, which would have to be invented, and process
// start time would differ per replica and move on every restart even when the
// file did not change.
//
// A file that cannot be read is left out of the map and served without an
// ETag, which is how every file behaves today.
func buildETags(fsys fs.FS) map[string]string {
	etags := map[string]string{}

	_ = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		f, err := fsys.Open(name)
		if err != nil {
			return nil
		}
		defer f.Close()

		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return nil
		}

		// 128 bits is far more than enough to tell the files of one build apart,
		// and it keeps the header short.
		etags[name] = fmt.Sprintf("%q", base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:16]))

		return nil
	})

	return etags
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
	s.serveFile(w, r, name)
}

// serveFile serves a file from the build tagged with its content hash, so a
// client that already holds a copy can revalidate with If-None-Match and get a
// 304 rather than the body again. http.ServeContent reads the ETag set here
// when it checks preconditions, so it writes the 304 itself.
func (s *uiServer) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	if etag, ok := s.etags[name]; ok {
		w.Header().Set("ETag", etag)
	}

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
		s.serveFile(w, r, userPath)
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
