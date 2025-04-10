package ui

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"
	"sync"

	"github.com/obot-platform/obot/pkg/oauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed all:admin/*build all:user/*build
var embedded embed.FS

func Handler(devPort, userOnlyPort int, client kclient.Client) http.Handler {
	server := &uiServer{
		client: client,
		lock:   new(sync.RWMutex),
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
				if strings.HasPrefix(r.URL.Path, "/admin") {
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
	lock       *sync.RWMutex
	configured bool
	client     kclient.Client
	rp         *httputil.ReverseProxy
	userOnly   bool
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

	userPath := path.Join("user/build/", r.URL.Path)
	adminPath := path.Join("admin/build/client", strings.TrimPrefix(r.URL.Path, "/admin"))

	if !strings.HasPrefix(r.URL.Path, "/admin/") && !s.hasModelProviderConfigured(r.Context()) {
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	}

	if r.URL.Path == "/" {
		http.ServeFileFS(w, r, embedded, "user/build/index.html")
	} else if _, err := fs.Stat(embedded, userPath); err == nil {
		http.ServeFileFS(w, r, embedded, userPath)
	} else if _, err := fs.Stat(embedded, adminPath); err == nil {
		http.ServeFileFS(w, r, embedded, adminPath)
	} else if strings.HasPrefix(r.URL.Path, "/admin") {
		http.ServeFileFS(w, r, embedded, "admin/build/client/index.html")
	} else {
		http.ServeFileFS(w, r, embedded, "user/build/fallback.html")
	}
}

func (s *uiServer) hasModelProviderConfigured(ctx context.Context) bool {
	s.lock.RLock()
	configured := s.configured
	s.lock.RUnlock()
	if configured {
		return configured
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	if s.configured {
		return s.configured
	}

	var models v1.ModelList
	if err := s.client.List(ctx, &models); err != nil {
		return false
	}

	s.configured = len(models.Items) > 0
	return s.configured
}
