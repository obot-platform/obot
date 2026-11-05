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
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/valyala/fasthttp"
)

const (
	codexFixture  = "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.c2ln"
	codexCatalog  = `{"models":[{"slug":"gpt-5.4","display_name":"Fixture","model_instructions":{"instructions_template":"Keep metadata"},"supported_reasoning_levels":[{"effort":"high"}],"tools":["shell"],"future_field":{"preserve":true}}],"future_catalog":42}`
	codexOutput   = `[{"type":"reasoning","id":"rs_fixture","encrypted_content":"encrypted_fixture","summary":[]},{"type":"function_call","id":"fc_fixture","call_id":"call_fixture","name":"weather","arguments":"{\"city\":\"Phoenix\"}","status":"completed"}]`
	codexResponse = `{"id":"resp_fixture","object":"response","model":"gpt-5.4","status":"completed","output":` + codexOutput + `,"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`
)

type codexRecorded struct {
	host    string
	uri     string
	headers http.Header
	body    []byte
}

// Pin the private-client seam to v1.12.0, just like the Claude recorder. Both
// production authorities are checked before redirecting to the local TLS fixture.
func codexTransport(t *testing.T, e *Engine, server *httptest.Server) {
	t.Helper()
	provider, ok := e.client.GetProviderByKey(schemas.OpenAI).(*openai.OpenAIProvider)
	require.True(t, ok)
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

func codexHTTPRequest(ctx context.Context, path, body string, subscription bool) *http.Request {
	method := http.MethodPost
	if strings.HasPrefix(path, "/v1/models") {
		method = http.MethodGet
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("User-Agent", "codex_cli_rs/0.162.1 (Mac OS; arm64)")
	r.Header.Set("X-Obot-Api-Key", "gateway-fixture")
	r.Header.Set("X-Unapproved", "private")
	r.Header.Set("OpenAI-Beta", "responses=experimental")
	r.Header.Set("Originator", "codex_cli_rs")
	r.Header.Set("Session_id", "session-fixture")
	if subscription {
		r.Header.Set("Authorization", codexFixture)
		r.Header.Set("ChatGPT-Account-ID", "workspace-fixture")
	}
	return r
}

func codexCall(ctx context.Context, e *Engine, path, body string, subscription bool, check guardrails.AttemptCheck) *httptest.ResponseRecorder {
	r := codexHTTPRequest(ctx, path, body, subscription)
	token, release := guardrails.BindRequest(guardrails.Identity{}, guardrails.RequestHooks{AttemptCheck: check})
	defer release()
	r.Header.Set(guardrails.HeaderRequestToken, token)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	return w
}

func codexReply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/models") {
		_, _ = io.WriteString(w, codexCatalog)
		return
	}
	data, _ := io.ReadAll(r.Body)
	if gjson.GetBytes(data, "stream").Bool() {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_fixture","object":"response","status":"in_progress","output":[]}}`,
			`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"function_call","id":"fc_fixture","call_id":"call_fixture","name":"weather","arguments":"","status":"in_progress"}}`,
			`{"type":"response.function_call_arguments.delta","sequence_number":2,"item_id":"fc_fixture","output_index":0,"delta":"{\"city\":\"Phoenix\"}"}`,
			`{"type":"response.function_call_arguments.done","sequence_number":3,"item_id":"fc_fixture","output_index":0,"arguments":"{\"city\":\"Phoenix\"}"}`,
			`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"function_call","id":"fc_fixture","call_id":"call_fixture","name":"weather","arguments":"{\"city\":\"Phoenix\"}","status":"completed"}}`,
			`{"type":"response.completed","sequence_number":5,"response":` + codexResponse + `}`,
		} {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", gjson.Get(event, "type").String(), event)
		}
		return
	}
	_, _ = io.WriteString(w, codexResponse)
}

func TestCodexCredentialAdmission(t *testing.T) {
	for _, tc := range []struct {
		name         string
		headers      http.Header
		valid        bool
		subscription bool
	}{
		{
			name:         "native UA and optional account absent",
			headers:      http.Header{"User-Agent": {"codex_cli_rs/0.162.1"}, "Authorization": {codexFixture}},
			valid:        true,
			subscription: true,
		},
		{
			name:         "existing codex UA",
			headers:      http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "Chatgpt-Account-Id": {"workspace"}},
			valid:        true,
			subscription: true,
		},
		{
			name:    "configured key mode",
			headers: http.Header{"User-Agent": {"codex_cli_rs/1"}, "X-Obot-Api-Key": {"gateway-fixture"}},
			valid:   true,
		},
		{
			name:    "static OpenAI key",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {"Bearer sk-fixture"}},
		},
		{
			name:    "Obot bearer",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {"Bearer obot-fixture"}},
		},
		{
			name:    "Claude token on Codex",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {subscriptionFixture}},
		},
		{
			name:    "wrong UA",
			headers: http.Header{"User-Agent": {"curl/1"}, "Authorization": {codexFixture}},
		},
		{
			name:    "mixed CLI identities",
			headers: http.Header{"User-Agent": {"codex-cli/1 claude-cli/2"}, "Authorization": {codexFixture}},
		},
		{
			name:    "repeated UA",
			headers: http.Header{"User-Agent": {"codex-cli/1", "codex-cli/1"}, "Authorization": {codexFixture}},
		},
		{
			name:    "mixed authorization",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "X-Api-Key": {"configured"}},
		},
		{
			name:    "duplicate authorization casing",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "authorization": {codexFixture}},
		},
		{
			name:    "combined authorization",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture + ", " + codexFixture}},
		},
		{
			name:    "two JWT segments",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {"Bearer eyJabc.c2ln"}},
		},
		{
			name:    "empty JWT signature",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {"Bearer eyJabc.abc."}},
		},
		{
			name:    "invalid JWT alphabet",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {"Bearer eyJabc.a/b.c2ln"}},
		},
		{
			name:    "repeated account",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "Chatgpt-Account-Id": {"one", "two"}},
		},
		{
			name:    "duplicate account casing",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "Chatgpt-Account-Id": {"one"}, "chatgpt-account-id": {"two"}},
		},
		{
			name:    "combined account",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "Chatgpt-Account-Id": {"one,two"}},
		},
		{
			name:    "empty account",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Authorization": {codexFixture}, "Chatgpt-Account-Id": {""}},
		},
		{
			name:    "account without subscription",
			headers: http.Header{"User-Agent": {"codex-cli/1"}, "Chatgpt-Account-Id": {"workspace"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credential, valid := subscriptionCredential(tc.headers)
			require.Equal(t, tc.valid, valid)
			if tc.subscription {
				require.Equal(t, schemas.OpenAI, credential.provider)
				require.Equal(t, codexFixture, credential.token)
			} else {
				require.Empty(t, credential)
			}
			if !tc.valid {
				// An empty engine proves invalid HTTP auth returns before parsing or core dispatch.
				r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
				r.Header = tc.headers
				w := httptest.NewRecorder()
				(&Engine{}).ServeHTTP(w, r)
				require.Equal(t, http.StatusForbidden, w.Code)
			}
		})
	}
}

func TestCodexRecordingCatalogAndToolLoop(t *testing.T) {
	received := make(chan codexRecorded, 20)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- codexRecorded{host: r.Host, uri: r.URL.RequestURI(), headers: r.Header.Clone(), body: body}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		codexReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", subscriptionProvider("configured-key")))
	input, output := guardrails.New(subscriptionPolicies{}, nil).Plugins()
	e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{input, output}})
	require.NoError(t, err)
	defer e.Shutdown()
	codexTransport(t, e, server)

	var metadata []schemas.BifrostResponseExtraFields
	ctx := WithResponseObserver(t.Context(), func(extra schemas.BifrostResponseExtraFields) { metadata = append(metadata, extra) })
	catalog := codexCall(ctx, e, "/v1/models?client_version=0.162.1&upstream_url=https://evil.example", "", true, nil)
	require.Equal(t, http.StatusOK, catalog.Code, catalog.Body.String())
	require.JSONEq(t, codexCatalog, catalog.Body.String())
	recorded := <-received
	require.Equal(t, "chatgpt.com", recorded.host)
	require.Equal(t, "/backend-api/codex/models?client_version=0.162.1", recorded.uri)
	require.Equal(t, codexFixture, recorded.headers.Get("Authorization"))
	require.Equal(t, "workspace-fixture", recorded.headers.Get("ChatGPT-Account-ID"))
	require.Empty(t, recorded.headers.Get("X-Obot-Api-Key"))

	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var attempts []guardrails.Attempt
			check := func(_ context.Context, attempt guardrails.Attempt) error {
				attempts = append(attempts, attempt)
				return nil
			}
			body := fmt.Sprintf(`{"model":"gpt-5.4","input":"weather?","store":false,"stream":%t,"tools":[{"type":"function","name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}`, stream)
			first := codexCall(ctx, e, "/v1/responses", body, true, check)
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			var response string
			if stream {
				require.Contains(t, first.Body.String(), "event: response.created")
				require.Contains(t, first.Body.String(), "event: response.function_call_arguments.delta")
				require.Contains(t, first.Body.String(), "event: response.output_item.done")
				for line := range strings.SplitSeq(first.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") && gjson.Get(line[6:], "type").String() == "response.completed" {
						response = gjson.Get(line[6:], "response").Raw
					}
				}
			} else {
				response = first.Body.String()
			}
			require.Equal(t, "resp_fixture", gjson.Get(response, "id").String())
			require.JSONEq(t, codexOutput, gjson.Get(response, "output").Raw)
			require.Equal(t, []guardrails.Attempt{{Model: "openai/gpt-5.4", Subscription: true}}, attempts)
			sent := <-received
			require.Equal(t, "gpt-5.4", gjson.GetBytes(sent.body, "model").String())
			require.Equal(t, "weather", gjson.GetBytes(sent.body, "tools.0.name").String())

			// Replay native reasoning and the returned function call together with its
			// tool result; IDs and encrypted content must survive both conversions.
			replay := strings.TrimSuffix(gjson.Get(response, "output").Raw, "]") + `,{"type":"function_call_output","call_id":"call_fixture","output":"Sunny"}]`
			followup := codexCall(ctx, e, "/v1/responses", `{"model":"gpt-5.4","input":`+replay+`,"stream":false,"store":false}`, true, check)
			require.Equal(t, http.StatusOK, followup.Code, followup.Body.String())
			replayed := <-received
			// The pinned OpenAI converter removes output-only status on replay.
			// Conversation identity, arguments, tool result and encrypted reasoning stay intact.
			expectedReplay, err := sjson.Delete(replay, "1.status")
			require.NoError(t, err)
			require.JSONEq(t, expectedReplay, gjson.GetBytes(replayed.body, "input").Raw)
			for _, request := range []codexRecorded{sent, replayed} {
				require.Equal(t, "chatgpt.com", request.host)
				require.Equal(t, "/backend-api/codex/responses", request.uri)
				require.Equal(t, codexFixture, request.headers.Get("Authorization"))
				require.Equal(t, "workspace-fixture", request.headers.Get("ChatGPT-Account-ID"))
				require.Empty(t, request.headers.Get("X-Obot-Api-Key"))
				require.Empty(t, request.headers.Get("X-Unapproved"))
				require.Empty(t, request.headers.Get(guardrails.HeaderRequestToken))
				require.Equal(t, "codex_cli_rs", request.headers.Get("Originator"))
				require.Equal(t, "session-fixture", request.headers.Get("Session_id"))
			}
		})
	}
	// No private caller credential, account or configured secret in observers.
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	for _, secret := range []string{codexFixture[7:], "workspace-fixture", "configured-key", "gateway-fixture"} {
		require.NotContains(t, string(encoded), secret)
	}

	// Same client identity without bearer keeps the ordinary API destination/key.
	ordinary := codexCall(t.Context(), e, "/v1/responses", `{"model":"openai/gpt-5.4","input":"hi"}`, false, func(_ context.Context, attempt guardrails.Attempt) error {
		require.False(t, attempt.Subscription)
		return nil
	})
	require.Equal(t, http.StatusOK, ordinary.Code, ordinary.Body.String())
	normal := <-received
	require.Equal(t, "api.openai.com", normal.host)
	require.Equal(t, "/v1/responses", normal.uri)
	require.Equal(t, "Bearer configured-key", normal.headers.Get("Authorization"))
	require.Empty(t, normal.headers.Get("ChatGPT-Account-ID"))
	require.Empty(t, normal.headers.Get("X-Obot-Api-Key"))
	require.Equal(t, http.StatusNotFound, codexCall(t.Context(), e, "/v1/models", "", false, nil).Code)

	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		denied := codexCall(t.Context(), e, "/v1/responses", `{"model":"gpt-5.4","input":"hi"}`, true, func(_ context.Context, attempt guardrails.Attempt) error {
			require.True(t, attempt.Subscription)
			return &guardrails.AttemptError{Status: status, Type: "permission_denied", Message: "denied fixture"}
		})
		require.Equal(t, status, denied.Code, denied.Body.String())
		require.Empty(t, received)
	}
}

func TestCodexRecordingFallbackRetryAndErrors(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var mu sync.Mutex
			var models []string
			var hosts []string
			var auths []string
			var accounts []string
			retry := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				model := gjson.GetBytes(body, "model").String()
				mu.Lock()
				models = append(models, model)
				hosts = append(hosts, r.Host)
				auths = append(auths, r.Header.Get("Authorization"))
				accounts = append(accounts, r.Header.Get("ChatGPT-Account-ID"))
				status := http.StatusOK
				switch model {
				case "fail":
					status = http.StatusServiceUnavailable
				case "retry":
					retry++
					if retry == 1 {
						status = http.StatusServiceUnavailable
					}
				case "unauthorized":
					status = http.StatusUnauthorized
				case "forbidden":
					status = http.StatusForbidden
				}
				mu.Unlock()
				if status != http.StatusOK {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"error":{"type":"fixture_error","message":"fixture rejection"}}`)
					return
				}
				r.Body = io.NopCloser(strings.NewReader(string(body)))
				codexReply(w, r)
			}))
			defer server.Close()
			foreignCalls := make(chan http.Header, 10)
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				foreignCalls <- r.Header.Clone()
				codexReply(w, r)
			}))
			defer foreign.Close()
			store := subscriptionStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "openai", subscriptionProvider("configured-key")))
			custom := subscriptionProvider("foreign-key")
			custom.Config.NetworkConfig.BaseURL = foreign.URL
			custom.Config.NetworkConfig.AllowPrivateNetwork = true
			custom.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}
			require.NoError(t, store.CreateProvider(t.Context(), "foreign", custom))
			input, output := guardrails.New(subscriptionPolicies{}, nil).Plugins()
			e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{input, output}})
			require.NoError(t, err)
			defer e.Shutdown()
			codexTransport(t, e, server)

			call := func(model string, fallbacks []string, subscription bool) *httptest.ResponseRecorder {
				body, err := json.Marshal(map[string]any{"model": model, "input": "hello", "stream": stream, "fallbacks": fallbacks})
				require.NoError(t, err)
				return codexCall(t.Context(), e, "/v1/responses", string(body), subscription, nil)
			}
			recovered := call("openai/fail", []string{"foreign/test", "gpt-5.4"}, true)
			require.Equal(t, http.StatusOK, recovered.Code, recovered.Body.String())
			require.Empty(t, foreignCalls)
			mu.Lock()
			require.Equal(t, []string{"fail", "fail", "gpt-5.4"}, models)
			models = nil
			mu.Unlock()

			// A foreign routed primary is refused too; a same-provider fallback recovers.
			provider, model := "foreign", "test"
			require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{
				ID:            "codex-route",
				Name:          "codex-route",
				Scope:         "global",
				CelExpression: "true",
				Targets: []tables.TableRoutingTarget{{
					Provider: &provider,
					Model:    &model,
					Weight:   1,
				}},
				ParsedFallbacks: []tables.RoutingFallback{{
					Provider: schemas.OpenAI,
					Model:    "gpt-5.4",
				}},
			}))
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			routed := call("gpt-5.4", nil, true)
			require.Equal(t, http.StatusOK, routed.Code, routed.Body.String())
			require.Empty(t, foreignCalls)
			ordinary := call("openai/gpt-5.4", nil, false)
			require.Equal(t, http.StatusOK, ordinary.Code, ordinary.Body.String())
			headers := <-foreignCalls
			require.Equal(t, "Bearer foreign-key", headers.Get("Authorization"))
			require.Empty(t, headers.Get("ChatGPT-Account-ID"))
			require.NoError(t, store.DeleteRule(t.Context(), "codex-route"))
			_, err = e.Sync(t.Context())
			require.NoError(t, err)

			mu.Lock()
			models = nil
			mu.Unlock()
			retried := call("retry", nil, true)
			require.Equal(t, http.StatusOK, retried.Code, retried.Body.String())
			mu.Lock()
			require.Equal(t, []string{"retry", "retry"}, models)
			for _, host := range hosts {
				require.Equal(t, "chatgpt.com", host)
			}
			mu.Unlock()
			for _, tc := range []struct {
				model  string
				status int
			}{
				{
					model:  "unauthorized",
					status: http.StatusUnauthorized,
				},
				{
					model:  "forbidden",
					status: http.StatusForbidden,
				},
			} {
				mu.Lock()
				models = nil
				mu.Unlock()
				result := call(tc.model, []string{"foreign/test"}, true)
				require.Equal(t, tc.status, result.Code, result.Body.String())
				mu.Lock()
				require.Equal(t, []string{tc.model}, models)
				for i := range auths {
					require.Equal(t, codexFixture, auths[i])
					require.Equal(t, "workspace-fixture", accounts[i])
				}
				mu.Unlock()
				require.Empty(t, foreignCalls)
			}
		})
	}
}

func TestCodexAttemptIsolationAndFinalGate(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{
		schemas.OpenAI:    {},
		schemas.Anthropic: {},
	}})
	ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	defer ctx.Cancel()
	ctx.SetValue(subscriptionTokenKey{}, subscriptionCredentialValue{provider: schemas.OpenAI, token: codexFixture, accountID: "workspace-fixture"})
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{"user-agent": {"codex_cli_rs/1"}})
	for _, provider := range []schemas.ModelProvider{schemas.OpenAI, schemas.Anthropic, "custom-openai", schemas.OpenAI} {
		req := &schemas.BifrostRequest{
			RequestType:      schemas.ResponsesRequest,
			ResponsesRequest: &schemas.BifrostResponsesRequest{Provider: provider, Model: "gpt-5.4"},
		}
		denied := prepareSubscription(ctx, req, a)
		extra := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
		require.False(t, ctx.Value(schemas.BifrostContextKeySkipKeySelection).(bool))
		if provider == schemas.OpenAI {
			require.Nil(t, denied)
			require.Equal(t, codexUpstream+"/responses", ctx.Value(schemas.BifrostContextKeyURLPath))
			key := ctx.Value(schemas.BifrostContextKeyDirectKey).(schemas.Key)
			require.Empty(t, key.Value.GetValue())
			require.Empty(t, key.ID)
			require.Empty(t, key.Name)
			require.Equal(t, []string{"workspace-fixture"}, extra["chatgpt-account-id"])
		} else {
			require.NotNil(t, denied)
			require.Nil(t, ctx.Value(schemas.BifrostContextKeyDirectKey))
			require.Nil(t, ctx.Value(schemas.BifrostContextKeyURLPath))
			require.NotContains(t, extra, "authorization")
			require.NotContains(t, extra, "chatgpt-account-id")
		}
		// The guard is restored before untrusted plugin writes can alter reserved fields.
		ctx.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: "plugin-override"})
		if key, ok := ctx.Value(schemas.BifrostContextKeyDirectKey).(schemas.Key); ok {
			require.Empty(t, key.ID)
		}
	}

	received := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		codexReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", subscriptionProvider("configured-key")))
	e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{&subscriptionRewrite{}}})
	require.NoError(t, err)
	defer e.Shutdown()
	codexTransport(t, e, server)
	for _, stream := range []bool{false, true} {
		result := codexCall(t.Context(), e, "/v1/responses", fmt.Sprintf(`{"model":"gpt-5.4","input":"hi","stream":%t}`, stream), true, nil)
		require.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
		require.Empty(t, received)
	}
}

func TestCodexContextPrivacyAndParsing(t *testing.T) {
	e := &Engine{account: &account{}}
	e.account.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{"foreign": {}}})
	r := codexHTTPRequest(t.Context(), "/v1/responses", "", true)
	credential, valid := subscriptionCredential(r.Header)
	require.True(t, valid)
	r = r.WithContext(context.WithValue(r.Context(), subscriptionTokenKey{}, credential))
	ctx, cancel := e.requestContext(r)
	defer cancel()
	for _, header := range []string{"authorization", "chatgpt-account-id", "x-obot-api-key"} {
		require.NotContains(t, ctx.Value(schemas.BifrostContextKeyRequestHeaders), header)
		require.NotContains(t, ctx.Value(schemas.BifrostContextKeyExtraHeaders), header)
	}
	provider, model, fallbacks := e.normalizeRequestModels(ctx, "gpt-5.4", []string{"gpt-5.4", "anthropic/test", "foreign/test"})
	require.Equal(t, schemas.OpenAI, provider)
	require.Equal(t, "gpt-5.4", model)
	require.Equal(t, []schemas.Fallback{
		{Provider: schemas.OpenAI, Model: "gpt-5.4"},
		{Provider: schemas.Anthropic, Model: "test"},
		{Provider: "foreign", Model: "test"},
	}, fallbacks)

	// Trusted direct calls without HTTP admission retain their explicit key and URL.
	direct := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	defer direct.Cancel()
	direct.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: "trusted"})
	direct.SetValue(schemas.BifrostContextKeyURLPath, "https://trusted.example")
	require.Nil(t, prepareSubscription(direct, &schemas.BifrostRequest{}, e.account))
	require.Equal(t, "trusted", direct.Value(schemas.BifrostContextKeyDirectKey).(schemas.Key).ID)
	require.Equal(t, "https://trusted.example", direct.Value(schemas.BifrostContextKeyURLPath))
}

func TestCodexOptionalIdentityStreamFailureAndParseRelease(t *testing.T) {
	received := make(chan http.Header, 10)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		if r.URL.Query().Has("unused") {
			t.Error("caller query reached fixed Responses URL")
		}
		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "stream").Bool() {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_failed\",\"status\":\"in_progress\",\"output\":[]}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_fixture\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\nevent: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\",\"output\":[],\"error\":{\"code\":\"server_error\",\"message\":\"fixture failure\"}}}\n\n")
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		codexReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", subscriptionProvider("configured-key")))
	e, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer e.Shutdown()
	codexTransport(t, e, server)

	for _, stream := range []bool{false, true} {
		r := codexHTTPRequest(t.Context(), "/v1/responses?unused=1", fmt.Sprintf(`{"model":"gpt-5.4","input":"hi","stream":%t}`, stream), true)
		r.Header.Del("ChatGPT-Account-ID")
		originalAuth := "bEaReR " + codexFixture[7:]
		r.Header.Set("Authorization", originalAuth)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		headers := <-received
		require.Empty(t, headers.Get("ChatGPT-Account-ID"))
		require.Equal(t, originalAuth, headers.Get("Authorization"))
		if stream {
			// The typed core converts failed Responses events into its standard SSE error.
			require.Contains(t, w.Body.String(), "event: error")
			require.Contains(t, w.Body.String(), "fixture failure")
			require.NotContains(t, w.Body.String(), "event: response.completed")
		}
	}
	for _, tc := range []struct {
		path   string
		body   string
		status int
	}{
		{
			path:   "/v1/responses",
			body:   `{"model":`,
			status: http.StatusBadRequest,
		},
		{
			path:   "/v1/chat/completions",
			body:   `{"model":"gpt-5.4"}`,
			status: http.StatusNotFound,
		},
	} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, codexHTTPRequest(t.Context(), tc.path, tc.body, true))
		require.Equal(t, tc.status, w.Code, w.Body.String())
		require.Empty(t, received)
		require.True(t, e.subscriptionLock(schemas.OpenAI).TryLock(), "parse failure leaked subscription reader")
		e.subscriptionLock(schemas.OpenAI).Unlock()
	}
}
