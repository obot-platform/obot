package llmcore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
	"unsafe"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/providers/openai"
	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestRuntimeProviderProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://environment.invalid:8080")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "api.openai.com")
	for _, tc := range []struct {
		name     string
		provider schemas.ModelProvider
		config   schemas.ProviderConfig
		global   string
		wantType schemas.ProxyType
		wantURL  string
	}{
		{
			name:     "environment selected despite API exclusion",
			provider: schemas.OpenAI,
			wantType: schemas.EnvProxy,
		},
		{
			name:     "persisted environment proxy",
			provider: schemas.OpenAI,
			config:   schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://environment.invalid:8080")}},
			wantType: schemas.EnvProxy,
		},
		{
			name:     "explicit provider proxy",
			provider: schemas.OpenAI,
			config:   schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://provider.invalid:8080")}},
			wantType: schemas.HTTPProxy,
			wantURL:  "http://provider.invalid:8080",
		},
		{
			name:     "explicit options on matching URL",
			provider: schemas.OpenAI,
			config:   schemas.ProviderConfig{ProxyConfig: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://environment.invalid:8080"), Username: schemas.NewSecretVar("user")}},
			wantType: schemas.HTTPProxy,
			wantURL:  "http://environment.invalid:8080",
		},
		{
			name:     "inherited global proxy with matching URL",
			provider: schemas.OpenAI,
			global:   `{"enabled":true,"enable_for_inference":true,"url":"http://environment.invalid:8080","no_proxy":"chatgpt.com"}`,
			wantType: schemas.HTTPProxy,
			wantURL:  "http://environment.invalid:8080",
		},
		{
			name:     "custom OpenAI provider unchanged",
			provider: schemas.OpenAI,
			config:   schemas.ProviderConfig{CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}},
		},
		{
			name:     "other providers unchanged",
			provider: schemas.Anthropic,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := runtimeProviderProxy(tc.provider, tc.config, json.RawMessage(tc.global))
			if tc.wantType == "" {
				require.True(t, proxy == nil || proxy.Type == schemas.NoProxy)
				return
			}
			require.Equal(t, tc.wantType, proxy.Type)
			require.Equal(t, tc.wantURL, proxy.URL.GetValue())
			if tc.wantType != schemas.EnvProxy {
				require.Equal(t, ProviderProxy(tc.config, json.RawMessage(tc.global)), proxy)
			}
		})
	}
}

func TestChatGPTSubscriptionEnvironmentProxyRecording(t *testing.T) {
	for _, bypass := range []string{"api.openai.com", "chatgpt.com"} {
		t.Run(bypass, func(t *testing.T) {
			received := make(chan codexRecorded, 10)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- codexRecorded{host: r.Host, uri: r.URL.RequestURI(), headers: r.Header.Clone()}
				codexReply(w, r)
			}))
			t.Cleanup(server.Close)
			connects := make(chan string, 10)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect || (r.Host != "api.openai.com:443" && r.Host != "chatgpt.com:443") {
					t.Errorf("unexpected proxy request %s %s", r.Method, r.Host)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				connects <- r.Host
				upstream, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
				if err != nil {
					t.Error(err)
					return
				}
				defer upstream.Close()
				conn, buffered, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
					t.Error(err)
					return
				}
				go func() {
					_, _ = io.Copy(upstream, buffered)
					_ = upstream.Close()
				}()
				_, _ = io.Copy(conn, upstream)
			}))
			t.Cleanup(proxy.Close)
			for _, name := range []string{"HTTP_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
				t.Setenv(name, "")
			}
			t.Setenv("HTTPS_PROXY", proxy.URL)
			t.Setenv("NO_PROXY", bypass)
			store := subscriptionStore(t)
			provider := subscriptionProvider("configured-key")
			if bypass == "chatgpt.com" {
				// Model the environment proxy already persisted for api.openai.com.
				provider.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar(proxy.URL)}
			}
			require.NoError(t, store.CreateProvider(t.Context(), "openai", provider))
			e, err := New(t.Context(), store, Options{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
			config, err := e.account.GetConfigForProvider(schemas.OpenAI)
			require.NoError(t, err)
			dial := providerUtils.ConfigureProxy(&fasthttp.Client{}, config.ProxyConfig, bifrost.NewDefaultLogger(schemas.LogLevelWarn)).Dial
			require.NotNil(t, dial)
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			openaiProvider := e.client.GetProviderByKey(schemas.OpenAI).(*openai.OpenAIProvider)
			for _, name := range []string{"client", "streamingClient"} {
				field := reflect.ValueOf(openaiProvider).Elem().FieldByName(name)
				client := *(**fasthttp.Client)(unsafe.Pointer(field.UnsafeAddr()))
				client.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
				// Pin Bifrost v1.12.0's bypass signal. Only direct dials are redirected;
				// the real environment dialer chooses which hosts use the CONNECT proxy.
				client.Dial = func(addr string) (net.Conn, error) {
					if addr != "api.openai.com:443" && addr != "chatgpt.com:443" {
						return nil, fmt.Errorf("unexpected test destination %s", addr)
					}
					conn, err := dial(addr)
					if err != nil && err.Error() == "target matches no_proxy: dial directly" {
						return net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
					}
					return conn, err
				}
			}
			for _, subscription := range []bool{false, true} {
				for _, stream := range []bool{false, true} {
					body := fmt.Sprintf(`{"model":"openai/gpt-5.4","input":"fixture","store":false,"stream":%t}`, stream)
					response := codexCall(t.Context(), e, "/v1/responses", body, subscription, nil)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					recorded := <-received
					host, auth := "api.openai.com", "Bearer configured-key"
					if subscription {
						host, auth = "chatgpt.com", chatgptFixture
					}
					require.Equal(t, host, recorded.host)
					require.Equal(t, auth, recorded.headers.Get("Authorization"))
				}
			}
			catalog := codexCall(t.Context(), e, "/v1/models?client_version=0.162.1", "", true, nil)
			require.Equal(t, http.StatusOK, catalog.Code, catalog.Body.String())
			require.Equal(t, "chatgpt.com", (<-received).host)
			close(connects)
			require.NotEmpty(t, connects)
			for host := range connects {
				require.NotEqual(t, bypass+":443", host)
			}
		})
	}
}
