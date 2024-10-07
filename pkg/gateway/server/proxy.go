package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/gorm"
)

// proxyToProvider will proxy the request based on the "provider" path parameter.
func (s *Server) proxyToProvider(proxyRequest *httputil.ProxyRequest) {
	logger := kcontext.GetLogger(proxyRequest.In.Context())
	user := kcontext.GetUser(proxyRequest.In.Context())

	providerSlug := proxyRequest.In.PathValue("provider")
	path := proxyRequest.In.PathValue("path")
	provider := new(types.LLMProvider)
	if err := s.db.WithContext(proxyRequest.In.Context()).Where("slug = ?", providerSlug).Where("disabled IS NULL OR disabled != ?", true).First(provider).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		// The provider was not found, so the provider path variable is part of the path we should forward to.
		path = strings.TrimSuffix(fmt.Sprintf("%s/%s", providerSlug, path), "/")
		// If the provider is not found, inspect the model from the request both.
		var body map[string]any
		if err := json.NewDecoder(proxyRequest.In.Body).Decode(&body); err == nil {
			if modelName, ok := body["model"].(string); ok {
				model := new(types.Model)
				if err := s.db.WithContext(proxyRequest.In.Context()).Transaction(func(tx *gorm.DB) error {
					if err := tx.Where("id = ?", modelName).Where("disabled IS NULL OR disabled != ?", true).First(model).Error; err != nil {
						return err
					}

					return tx.Where("id = ?", model.LLMProviderID).Where("disabled IS NULL OR disabled != ?", true).First(provider).Error
				}); err != nil {
					// Indicate that there was an issue with getting the provider for this request.
					// This will produce an 'unsupported protocol scheme "not-found"' error that the error handler will detect.
					proxyRequest.Out, _ = http.NewRequest("GET", fmt.Sprintf("not-found://%s/%s", providerSlug, modelName), nil)
					return
				}

				// Here we have the provider from the model name.
				// Replace the model in the request body.
				body["model"] = model.ProviderModelName

				b, err := json.Marshal(body)
				if err != nil {
					// Indicate that there was an unexpected issue.
					// This will produce an 'unsupported protocol scheme "unexpected"' error that the error handler will detect.
					proxyRequest.Out, _ = http.NewRequest("GET", fmt.Sprintf("unexpected://%v", err), nil)
					return
				}

				// Convert the body back to JSON
				proxyRequest.In.Body = io.NopCloser(bytes.NewReader(b))
				proxyRequest.In.ContentLength = int64(len(b))
				proxyRequest.In.Header.Set("Content-Length", fmt.Sprintf("%d", proxyRequest.In.ContentLength))
			}
		}
	} else if err != nil {
		// Indicate that there was an issue with getting the provider for this request.
		// This will produce an 'unsupported protocol scheme "not-found"' error that the error handler will detect.
		proxyRequest.Out, _ = http.NewRequest("GET", "not-found://"+providerSlug, nil)
		return
	}

	u, err := url.Parse(provider.BaseURL)
	if err != nil {
		// Indicate that there was an unexpected issue.
		// This will produce an 'unsupported protocol scheme "unexpected"' error that the error handler will detect.
		proxyRequest.Out, _ = http.NewRequest("GET", fmt.Sprintf("unexpected://%v", err), nil)
		return
	}

	proxyRequest.Out = new(http.Request)
	*proxyRequest.Out = *proxyRequest.In

	proxyRequest.Out.Header.Set("Authorization", "Bearer "+provider.Token)

	// Rewrite the URL
	proxyRequest.SetURL(&url.URL{
		Scheme:      u.Scheme,
		Host:        u.Host,
		RawQuery:    proxyRequest.In.URL.RawQuery,
		Fragment:    proxyRequest.In.URL.Fragment,
		RawFragment: proxyRequest.In.URL.RawFragment,
	})
	proxyRequest.Out.URL.Path = u.Path + "/" + path
	proxyRequest.Out.URL.Host = u.Host

	logger.InfoContext(proxyRequest.In.Context(), "proxy request received", "path", proxyRequest.In.RequestURI, "username", user.Username)
}

func (s *Server) proxyError(w http.ResponseWriter, r *http.Request, err error) {
	if err.Error() == `unsupported protocol scheme "not-found"` {
		writeError(r.Context(), kcontext.GetLogger(r.Context()), w, http.StatusNotFound, fmt.Errorf("unable to find LLM provider [%s] or model [%s]", r.URL.Host, strings.TrimPrefix(r.URL.Path, "/")))
		return
	}
	if err.Error() == `unsupported protocol scheme "unexpected"` {
		writeError(r.Context(), kcontext.GetLogger(r.Context()), w, http.StatusInternalServerError, fmt.Errorf("an unexpected error occurred: %s", r.URL.Host))
		return
	}

	writeError(r.Context(), kcontext.GetLogger(r.Context()), w, http.StatusInternalServerError, err)
}
