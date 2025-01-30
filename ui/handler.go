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

	"github.com/obot-platform/obot/pkg/api/static"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed all:admin/*build all:user/*build
var embedded embed.FS

func Handler(devPort int, client kclient.Client, additionalStaticDir string) (http.Handler, error) {
	server := &uiServer{
		client: client,
		lock:   new(sync.RWMutex),
	}

	if devPort != 0 {
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

	var handler http.Handler = server

	if additionalStaticDir != "" {
		var err error
		handler, err = static.Wrap(handler, additionalStaticDir)
		if err != nil {
			return nil, err
		}
	}

	handler = oauthMiddleware(handler)

	return handler, nil
}

type uiServer struct {
	lock       *sync.RWMutex
	configured bool
	client     kclient.Client
	rp         *httputil.ReverseProxy
}

func (s *uiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.rp != nil {
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

func isOAuthCallbackResponse(r *http.Request) bool {
	return r.URL.Path == "/" &&
		(r.URL.Query().Get("code") != "" ||
			r.URL.Query().Get("error") != "" ||
			r.URL.Query().Get("state") != "")
}

func oauthMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isOAuthCallbackResponse(r) {
			redirectURL := r.URL
			redirectURL.Path = "/oauth2/callback"
			http.Redirect(w, r, redirectURL.String(), http.StatusFound)
			return
		}
		h.ServeHTTP(w, r)
	})
}
