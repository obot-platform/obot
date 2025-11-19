package ui

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/pkg/oauth"
)

func Handler(devPort, userOnlyPort int, uiBasePath string) http.Handler {
	server := &uiServer{
		uiBasePath: uiBasePath,
	}

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
				if strings.HasPrefix(r.URL.Path, "/legacy-admin") {
					r.URL.Host = fmt.Sprintf("localhost:%d", devPort)
				} else {
					r.URL.Host = fmt.Sprintf("localhost:%d", devPort+1)
				}
			},
		}
	}

	return server
}

type uiServer struct {
	rp         *httputil.ReverseProxy
	userOnly   bool
	uiBasePath string
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

	// If no base path is provided and no proxy is set, return 404
	if s.uiBasePath == "" {
		http.NotFound(w, r)
		return
	}

	if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		http.NotFound(w, r)
		return
	}

	userBuildPath := filepath.Join(s.uiBasePath, "ui", "user", "build")
	adminBuildPath := filepath.Join(s.uiBasePath, "ui", "admin", "build", "client")

	userPath := filepath.Join(userBuildPath, r.URL.Path)
	adminPath := filepath.Join(adminBuildPath, strings.TrimPrefix(r.URL.Path, "/legacy-admin"))

	if r.URL.Path == "/" {
		http.ServeFile(w, r, filepath.Join(userBuildPath, "index.html"))
	} else if r.URL.Path == "/admin" {
		http.ServeFile(w, r, filepath.Join(userBuildPath, "admin.html"))
	} else if r.URL.Path == "/admin/" {
		// we have to redirect to /admin instead of serving the index.html file because ending slash will laod a different route for js files
		http.Redirect(w, r, "/admin", http.StatusFound)
	} else if r.URL.Path == "/mcp-publisher/" {
		http.Redirect(w, r, "/mcp-publisher", http.StatusFound)
	} else if r.URL.Path == "/mcp-publisher" {
		http.ServeFile(w, r, filepath.Join(userBuildPath, "mcp-publisher.html"))
	} else if strings.HasSuffix(r.URL.Path, "/") {
		// Paths with trailing slashes should redirect to without slash to avoid directory listings
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusFound)
	} else if _, err := os.Stat(userPath + ".html"); err == nil {
		// Try .html version first (for SvelteKit prerendered pages)
		http.ServeFile(w, r, userPath+".html")
	} else if _, err := os.Stat(userPath); err == nil {
		http.ServeFile(w, r, userPath)
	} else if _, err := os.Stat(adminPath); err == nil {
		http.ServeFile(w, r, adminPath)
	} else if strings.HasPrefix(r.URL.Path, "/legacy-admin") {
		http.ServeFile(w, r, filepath.Join(adminBuildPath, "index.html"))
	} else {
		http.ServeFile(w, r, filepath.Join(userBuildPath, "fallback.html"))
	}
}
