package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/databricks"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type staticModelProviderURLResolver struct {
	u url.URL
}

func (s staticModelProviderURLResolver) URLForModelProvider(context.Context, string, string) (url.URL, error) {
	return s.u, nil
}

func TestDatabricksProviderBackend(t *testing.T) {
	t.Parallel()
	discoveryURL, _ := url.Parse("http://127.0.0.1:1234")
	backend := databricksProviderBackend{dispatcher: staticModelProviderURLResolver{u: *discoveryURL}}
	creds := map[string]string{
		databricks.WorkspaceURLEnv: "https://workspace.example",
		databricks.TokenEnv:        "secret",
	}

	for _, test := range []struct {
		name      string
		method    string
		path      string
		dialect   llmtypes.Dialect
		model     *v1.Model
		wantURL   string
		wantError bool
	}{
		{
			name:    "models",
			method:  http.MethodGet,
			path:    "v1/models",
			wantURL: "http://127.0.0.1:1234/v1/models",
		},
		{
			name:    "OpenAI Responses",
			method:  http.MethodPost,
			path:    "v1/responses",
			dialect: llmtypes.DialectOpenAIResponses,
			model:   databricksModel("gpt", llmtypes.DialectOpenAIResponses),
			wantURL: "https://workspace.example/serving-endpoints/responses",
		},
		{
			name:    "OpenResponses",
			method:  http.MethodPost,
			path:    "v1/responses",
			dialect: llmtypes.DialectOpenResponses,
			model:   databricksModel("claude", llmtypes.DialectOpenResponses),
			wantURL: "https://workspace.example/serving-endpoints/open-responses",
		},
		{
			name:      "missing model",
			method:    http.MethodPost,
			path:      "v1/responses",
			wantError: true,
		},
		{
			name:      "unsupported dialect",
			method:    http.MethodPost,
			path:      "v1/responses",
			model:     databricksModel("chat", llmtypes.DialectOpenAIChatCompletions),
			wantError: true,
		},
		{
			name:      "unsupported path",
			method:    http.MethodPost,
			path:      "v1/chat/completions",
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://gateway.local", nil)
			req.SetPathValue("path", test.path)
			base, dialect, err := backend.upstreamURL(req, creds, test.model)
			if (err != nil) != test.wantError {
				t.Fatalf("upstreamURL() error = %v, wantError %v", err, test.wantError)
			}
			if err != nil {
				var httpErr *types2.ErrHTTP
				if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
					t.Fatalf("error = %T %v, want bad request", err, err)
				}
				return
			}
			if dialect != test.dialect {
				t.Errorf("dialect = %q, want %q", dialect, test.dialect)
			}
			proxyReq := &httputil.ProxyRequest{In: req, Out: req.Clone(req.Context())}
			llmRewriteRequest(base)(proxyReq)
			if got := proxyReq.Out.URL.String(); got != test.wantURL {
				t.Errorf("URL = %q, want %q", got, test.wantURL)
			}
		})
	}
}

func databricksModel(name string, dialect llmtypes.Dialect) *v1.Model {
	return &v1.Model{
		Name: name,
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			ModelProvider: system.DatabricksModelProvider,
			TargetModel:   "databricks-" + name,
			Dialect:       string(dialect),
		}},
	}
}
