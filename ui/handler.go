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

//go:embed all:user/*build
var embedded embed.FS

// assets is the built UI. It is a variable, and an fs.FS rather than the
// embed.FS directly, so that a test can supply a build: the real one is only
// present after the UI has been compiled, which would otherwise make these
// tests pass or fail depending on whether someone had run make.
var assets fs.FS = embedded

// buildAssetPrefix is where the UI build puts its output. Everything under it
// is an asset, and nothing under it is a route.
//
// immutableAssetPrefix is the part of that named by content hash. The rest --
// version.json, say -- keeps its name across builds and so must not be cached
// as though it were fixed.
const (
	buildAssetPrefix     = "/_app/"
	immutableAssetPrefix = "/_app/immutable/"
)

func Handler(devPort, userOnlyPort int) http.Handler {
	server := &uiServer{}

	if userOnlyPort != 0 {
		server.rp = &httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				r.URL.Host = fmt.Sprintf("localhost:%d", userOnlyPort)
			},
		}
		server.userOnly = true
	} else if devPort != 0 {
		server.rp = &httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				r.URL.Host = fmt.Sprintf("localhost:%d", devPort)
			},
		}
	}

	return server
}

type uiServer struct {
	rp       *httputil.ReverseProxy
	userOnly bool
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

	if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		http.NotFound(w, r)
		return
	}

	// Said here rather than left to whatever CDN sits in front. Obot set no
	// policy for the UI at all, so the edge applied its own default -- four
	// hours -- to everything, including a page shell served in place of a
	// missing asset. Stating it keeps that decision where the meaning of each
	// response is known.
	if strings.HasPrefix(r.URL.Path, immutableAssetPrefix) {
		// A change to one of these is a change to its name, so there is nothing
		// a client can hold that will ever be wrong.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// Everything else keeps its name across builds -- the shell above all,
		// which names the assets for the build it came from. Cached, it goes on
		// asking for files that a later build no longer has.
		w.Header().Set("Cache-Control", "no-cache")
	}

	userPath := path.Join("user/build/", r.URL.Path)

	if r.URL.Path == "/" {
		http.ServeFileFS(w, r, assets, "user/build/index.html")
	} else if r.URL.Path == "/admin" {
		http.ServeFileFS(w, r, assets, "user/build/admin.html")
	} else if r.URL.Path == "/admin/" {
		// we have to redirect to /admin instead of serving the index.html file because ending slash will laod a different route for js files
		http.Redirect(w, r, "/admin", http.StatusFound)
	} else if r.URL.Path == "/mcp-servers/" {
		http.Redirect(w, r, "/mcp-servers", http.StatusFound)
	} else if r.URL.Path == "/mcp-servers" {
		http.ServeFileFS(w, r, assets, "user/build/mcp-servers.html")
	} else if strings.HasSuffix(r.URL.Path, "/") {
		// Paths with trailing slashes should redirect to without slash to avoid directory listings
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusFound)
	} else if _, err := fs.Stat(assets, userPath+".html"); err == nil {
		// Try .html version first (for SvelteKit prerendered pages)
		http.ServeFileFS(w, r, assets, userPath+".html")
	} else if _, err := fs.Stat(assets, userPath); err == nil {
		http.ServeFileFS(w, r, assets, userPath)
	} else if strings.HasPrefix(r.URL.Path, buildAssetPrefix) {
		// A build asset is named by a hash of its own contents, so it is never a
		// route to fall back to: if it is not here, the only true answer is that
		// it does not exist.
		//
		// Falling back served the page shell instead, as 200 text/html with the
		// asset's own cache lifetime. Browsers refuse that under strict MIME
		// checking, and -- because it is a 200 -- every cache in the path stores
		// it: the CDN edge, and then each visitor's browser. A rolling deploy is
		// enough to produce it, since for a few seconds one replica serves an
		// index.html naming hashes another replica does not have yet. That
		// window is short; the caching of it is not, and clearing it means
		// purging the CDN and every affected browser.
		//
		// A 404 leaves nothing to cache and lets the next request succeed --
		// but only if it is not itself cached. An edge will store a 404 by
		// default, briefly, which is long enough to outlive the deploy that
		// caused it, so this says not to.
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
	} else {
		http.ServeFileFS(w, r, assets, "user/build/fallback.html")
	}
}
