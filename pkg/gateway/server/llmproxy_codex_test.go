package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/valyala/fasthttp"
)

// Pin the recording seam to Bifrost v1.12.0 without relaxing production TLS policy.
func recordCodexTransport(t *testing.T, engine *llmengine.Engine, server *httptest.Server) {
	t.Helper()
	provider := engine.Client().GetProviderByKey(schemas.OpenAI).(*openai.OpenAIProvider)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, name := range []string{"client", "streamingClient"} {
		field := reflect.ValueOf(provider).Elem().FieldByName(name)
		client := *(**fasthttp.Client)(unsafe.Pointer(field.UnsafeAddr()))
		client.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
		client.Dial = func(addr string) (net.Conn, error) {
			if addr != "chatgpt.com:443" && addr != "api.openai.com:443" {
				return nil, fmt.Errorf("unexpected test destination %s", addr)
			}
			return net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		}
	}
}

func TestPublicCodexRecording(t *testing.T) {
	const bearer = "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.c2ln"
	const catalog = `{"models":[{"slug":"test","model_instructions":{"instructions_template":"keep"},"future":42},{"slug":"forbidden","future":43}],"future_catalog":true}`
	received := make(chan *http.Request, 4)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = io.WriteString(w, catalog)
		} else {
			_, _ = io.WriteString(w, `{"id":"resp_fixture","object":"response","model":"test","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
		}
	}))
	defer upstream.Close()
	proxy, req := directProxy(t, &directFixture{})
	store, err := llmcoreclient.New(req.Storage, directCodec{})
	require.NoError(t, err)
	input, output := guardrails.New(req.GatewayClient, nil).Plugins()
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir(), FirstPlugin: input, LastPlugin: output}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	recordCodexTransport(t, engine, upstream)
	require.NoError(t, req.GatewayClient.SetLLMGatewaySettings(t.Context(), gatewaytypes.LLMGatewaySettings{CodexSubscriptionPassthrough: true}))
	proxy.backend = &bifrostProviderBackend{engine: engine}
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "inference without configured keys",
			method: http.MethodPost,
			path:   "v1/responses",
			body:   `{"model":"test","input":"hi"}`,
		},
		{
			name:   "native catalog access filtering",
			method: http.MethodGet,
			path:   "v1/models?client_version=0.162.1&provider=anthropic",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/llm-proxy/"+tc.path, strings.NewReader(tc.body))
			r.SetPathValue("path", strings.Split(tc.path, "?")[0])
			r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
			r.Header.Set("Authorization", bearer)
			r.Header.Set("ChatGPT-Account-ID", "workspace-fixture")
			r.Header.Set(HeaderObotAPIKey, "gateway-fixture")
			r.Header.Set("OpenAI-Organization", "caller-organization")
			r.Header.Set("Cookie", "caller-cookie")
			w := httptest.NewRecorder()
			call := req
			call.Request, call.ResponseWriter = r, w
			require.NoError(t, proxy.proxy(call))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			select {
			case sent := <-received:
				require.Equal(t, "chatgpt.com", sent.Host)
				require.Equal(t, bearer, sent.Header.Get("Authorization"))
				require.Equal(t, "workspace-fixture", sent.Header.Get("ChatGPT-Account-ID"))
				require.Empty(t, sent.Header.Get(HeaderObotAPIKey))
				require.Empty(t, sent.Header.Get("OpenAI-Organization"))
				require.Empty(t, sent.Header.Get("Cookie"))
				if tc.method == http.MethodGet {
					require.Equal(t, "/backend-api/codex/models?client_version=0.162.1", sent.URL.RequestURI())
					require.Equal(t, "test", gjson.Get(w.Body.String(), "models.0.slug").String())
					require.Equal(t, int64(1), gjson.Get(w.Body.String(), "models.#").Int())
					require.Equal(t, "keep", gjson.Get(w.Body.String(), "models.0.model_instructions.instructions_template").String())
					require.True(t, gjson.Get(w.Body.String(), "future_catalog").Bool())
				} else {
					require.Equal(t, "/backend-api/codex/responses", sent.URL.Path)
				}
			case <-time.After(time.Second):
				t.Fatal("no upstream request")
			}
		})
	}
	// Disabled passthrough cannot silently fall back to configured-key billing.
	proxy.backend = &bifrostProviderBackend{engine: engine}
	require.NoError(t, req.GatewayClient.SetLLMGatewaySettings(t.Context(), gatewaytypes.LLMGatewaySettings{SubscriptionPassthrough: true}))
	r := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models", nil)
	r.SetPathValue("path", "v1/models")
	r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
	r.Header.Set("Authorization", bearer)
	req.Request, req.ResponseWriter = r, httptest.NewRecorder()
	require.Error(t, proxy.proxy(req))
	require.Empty(t, received)
}

func TestCodexAttemptsStayOnOpenAI(t *testing.T) {
	attempts := &gatewayAttempts{budgets: map[string][]string{}, subscriptionProvider: "openai"}
	check := attempts.check(nil, nil, api.Context{}, nil)
	for _, target := range []string{"anthropic/test", "custom-openai/test"} {
		err := check(t.Context(), guardrails.Attempt{Model: target, Subscription: true})
		var refused *guardrails.AttemptError
		require.ErrorAs(t, err, &refused)
		require.Equal(t, http.StatusForbidden, refused.Status)
	}
	require.False(t, attempts.paidBySubscription())
}

func TestCodexCatalogRequiresOpenAIGrant(t *testing.T) {
	for _, granted := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		r.Header.Set("User-Agent", "codex_cli_rs/0.162.1")
		r.Header.Set("Authorization", "Bearer eyJfixture.payload.signature")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Request:    r,
			Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"test","future":42}]}`)),
		}
		require.NoError(t, filterModelListResponse(resp, map[string]bool{"anthropic/test": true, "test": true, "openai/test": granted}, false))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, granted, gjson.GetBytes(body, "models.0").Exists())
	}
}
