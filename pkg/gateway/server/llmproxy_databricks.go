package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/databricks"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

var (
	_ modelProviderURLResolver = (*dispatcher.Dispatcher)(nil)
)

type modelProviderURLResolver interface {
	URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (url.URL, error)
}

type databricksProviderBackend struct {
	dispatcher modelProviderURLResolver
}

func (s *Server) newDatabricksLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   databricksProviderBackend{dispatcher: s.dispatcher},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (databricksProviderBackend) modelProviderName() string {
	return system.DatabricksModelProvider
}

func (b databricksProviderBackend) upstreamURL(req *http.Request, credEnv map[string]string, model *v1.Model) (url.URL, llmtypes.Dialect, error) {
	reqPath := strings.Trim(strings.TrimPrefix(strings.Trim(req.PathValue("path"), "/"), "v1/"), "/")
	if req.Method == http.MethodGet && reqPath == "models" {
		if b.dispatcher == nil {
			return url.URL{}, "", fmt.Errorf("databricks model provider dispatcher is unavailable")
		}
		u, err := b.dispatcher.URLForModelProvider(req.Context(), system.DefaultNamespace, system.DatabricksModelProvider)
		return u, "", err
	}
	if req.Method != http.MethodPost || reqPath != "responses" {
		return url.URL{}, "", types2.NewErrBadRequest("unsupported Databricks model path %q", req.PathValue("path"))
	}
	if model == nil {
		return url.URL{}, "", types2.NewErrBadRequest("Databricks Responses requests require a model")
	}

	dialect := llmtypes.Dialect(model.Spec.Manifest.Dialect)
	var upstreamPath string
	switch dialect {
	case llmtypes.DialectOpenAIResponses:
		upstreamPath = "responses"
	case llmtypes.DialectOpenResponses:
		upstreamPath = "open-responses"
	default:
		return url.URL{}, "", types2.NewErrBadRequest("Databricks model %q has unsupported dialect %q", model.Name, dialect)
	}

	u, err := databricks.BaseURL(credEnv)
	if err != nil {
		return url.URL{}, "", err
	}
	u.Path = "/serving-endpoints"
	req.SetPathValue("path", upstreamPath)
	return u, dialect, nil
}

func (databricksProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	return databricks.Transport(credEnv, http.DefaultTransport)
}
