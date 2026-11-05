//go:build e2e

// The spike is tested against the LLM gateway e2e suite's mock upstream, so it is held to the
// same upstream behavior. Run it with `go test -tags e2e ./pkg/llmcore/`.
package llmcore_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

const (
	headerServedProvider = "X-Obot-LLM-Provider"
	openAIKey            = "e2e-upstream-openai-key"
	anthropicKey         = "e2e-upstream-anthropic-key"

	apiResponses = "responses"
	apiMessages  = "messages"
	apiChat      = "chat"

	// deniedModel is refused by the test's access plugin, as Obot refuses a model the caller
	// isn't granted.
	deniedModel = "gpt-e2e-vip"
)

var (
	allAPIs = []string{apiResponses, apiMessages, apiChat}
)

// replica is one engine serving HTTP, as one Obot replica would.
type replica struct {
	engine *llmcore.Engine
	server *httptest.Server
}

type testEnv struct {
	mock  *harness.MockLLM
	store *llmcoreclient.Client
	// A and B are two engines sharing one database.
	A, B *replica
}

// noPolicies is a guardrail store with no policies, so Obot's guardrail plugins only do what
// they do for every request: check each attempt and report usage.
type noPolicies struct{}

// usage is the token usage Obot's plugins reported for one request.
type usage struct {
	lock     sync.Mutex
	in, out  int
	reported bool
}

// llmResult is a response with its output assembled from either a JSON body or an event
// stream, as the e2e suite assembles it.
type llmResult struct {
	Status     int
	Header     http.Header
	Body       string
	Text       string
	ToolArgs   string
	StopReason string
	ErrorType  string
	ErrorMsg   string
	// Usage is what Obot's plugins reported the request used.
	Usage *usage
}

func (noPolicies) ListGuardrailPolicies(context.Context) ([]types.GuardrailPolicy, error) {
	return nil, nil
}
func (noPolicies) ListGuardrails(context.Context) ([]types.Guardrail, error) { return nil, nil }
func (noPolicies) GuardrailSecrets(context.Context, string) (map[string]string, error) {
	return nil, nil
}

func providerConfig(baseURL string) schemas.ProviderConfig {
	return schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        baseURL,
			AllowPrivateNetwork:            true,
			DefaultRequestTimeoutInSeconds: 30,
		},
		ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{Concurrency: 10, BufferSize: 100},
	}
}

func apiKey(id, value string) schemas.Key {
	return schemas.Key{ID: id, Name: id, Value: *schemas.NewSecretVar(value), Models: schemas.WhiteList{"*"}, Weight: 1}
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	mock, err := harness.StartMockLLM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	store := newTestStore(t)
	for _, p := range []struct{ name, key string }{
		{name: "openai",
			key: openAIKey},
		{name: "anthropic",
			key: anthropicKey},
	} {
		if err := store.CreateProvider(ctx, p.name, llmcoreclient.Provider{Config: providerConfig(mock.URL)}); err != nil {
			t.Fatal(err)
		}
		if err := store.PutKey(ctx, p.name, apiKey(p.name+"-1", p.key)); err != nil {
			t.Fatal(err)
		}
	}

	env := &testEnv{mock: mock, store: store}
	env.A, env.B = env.newReplica(t), env.newReplica(t)
	return env
}

func (env *testEnv) newReplica(t *testing.T) *replica {
	t.Helper()
	// Obot's own plugins, unchanged: they run in the core engine as they do in the transport.
	input, output := guardrails.New(noPolicies{}, nil).Plugins()
	e, err := llmcore.New(context.Background(), env.store, llmcore.Options{Plugins: []schemas.LLMPlugin{input, output}})
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(e)
	t.Cleanup(func() {
		s.Close()
		if err := e.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return &replica{engine: e, server: s}
}

func (r *replica) sync(t *testing.T) {
	t.Helper()
	if _, err := r.engine.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (r *llmResult) requireOK(tb testing.TB) *llmResult {
	tb.Helper()
	if r.Status != http.StatusOK || r.ErrorMsg != "" {
		tb.Fatalf("want a successful response, got HTTP %d: %s", r.Status, r.Body)
	}
	return r
}

func nonce(prefix string) string {
	return prefix + "-" + harness.RandomSuffix() + harness.RandomSuffix()
}

func (r *replica) call(tb testing.TB, api, model, prompt string, stream bool, extra map[string]any, headers ...map[string]string) *llmResult {
	tb.Helper()
	var body map[string]any
	path := "/v1/responses"
	switch api {
	case apiResponses:
		body = map[string]any{"model": model, "input": []any{map[string]any{"role": "user", "content": prompt}}}
	case apiChat:
		path = "/v1/chat/completions"
		body = map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": prompt}}}
	case apiMessages:
		path = "/anthropic/v1/messages"
		body = map[string]any{"model": model, "max_tokens": 256, "messages": []any{map[string]any{"role": "user", "content": prompt}}}
	}
	if stream {
		body["stream"] = true
	}
	maps.Copy(body, extra)
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, r.server.URL+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if api == apiMessages {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	// Bind the request as Obot's proxy does: every model the engine tries is checked, and the
	// usage of the one that serves is reported back.
	u := &usage{}
	token, release := guardrails.BindRequest(guardrails.Identity{}, guardrails.RequestHooks{
		AttemptCheck: func(_ context.Context, a guardrails.Attempt) error {
			if strings.HasSuffix(a.Model, "/"+deniedModel) {
				return &guardrails.AttemptError{Status: http.StatusForbidden, Type: "permission_error", Message: "you may not use " + a.Model}
			}
			return nil
		},
		Cost: func(c guardrails.Cost) {
			u.lock.Lock()
			defer u.lock.Unlock()
			u.in, u.out, u.reported = c.InputTokens, c.OutputTokens, true
		},
	})
	defer release()
	req.Header.Set(guardrails.HeaderRequestToken, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	res := parseLLM(api, resp.StatusCode, resp.Header, out)
	res.Usage = u
	return res
}

func parseLLM(api string, status int, header http.Header, body []byte) *llmResult {
	r := &llmResult{Status: status, Header: header, Body: string(body)}
	if strings.Contains(header.Get("Content-Type"), "text/event-stream") {
		parseStream(api, r)
		return r
	}
	doc := gjson.ParseBytes(body)
	parseError(doc, r)
	if status != http.StatusOK {
		return r
	}
	switch api {
	case apiResponses:
		for _, item := range doc.Get("output").Array() {
			switch item.Get("type").String() {
			case "message":
				for _, part := range item.Get("content").Array() {
					r.Text += part.Get("text").String()
				}
			case "function_call":
				r.ToolArgs += item.Get("arguments").String()
			}
		}
	case apiChat:
		r.Text = doc.Get("choices.0.message.content").String()
		r.ToolArgs = doc.Get("choices.0.message.tool_calls.0.function.arguments").String()
	case apiMessages:
		r.StopReason = doc.Get("stop_reason").String()
		for _, block := range doc.Get("content").Array() {
			switch block.Get("type").String() {
			case "text":
				r.Text += block.Get("text").String()
			case "tool_use":
				r.ToolArgs += block.Get("input").Raw
			}
		}
	}
	return r
}

func parseError(doc gjson.Result, r *llmResult) {
	if e := doc.Get("error"); e.Exists() {
		r.ErrorType = e.Get("type").String()
		r.ErrorMsg = e.Get("message").String()
	}
}

func parseStream(api string, r *llmResult) {
	sc := bufio.NewScanner(strings.NewReader(r.Body))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var completedText string
	sawDelta := false
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" || !gjson.Valid(data) {
			continue
		}
		ev := gjson.Parse(data)
		if ev.Get("error").Exists() || ev.Get("type").String() == "error" || ev.Get("type").String() == "response.failed" {
			parseError(ev, r)
			// Responses terminal errors use top-level message/code after HTTP 200.
			// Preserve nested Chat/Messages errors and response.failed as fallbacks.
			if ev.Get("type").String() == "error" && ev.Get("message").Exists() {
				r.ErrorMsg = ev.Get("message").String()
				r.ErrorType = ev.Get("code").String()
			}
			if r.ErrorType == "" {
				r.ErrorType = ev.Get("response.error.code").String()
			}
			if r.ErrorMsg == "" {
				r.ErrorMsg = ev.Get("response.error.message").String()
			}
		}
		switch api {
		case apiResponses:
			switch ev.Get("type").String() {
			case "response.output_text.delta":
				r.Text += ev.Get("delta").String()
				sawDelta = true
			case "response.function_call_arguments.delta":
				r.ToolArgs += ev.Get("delta").String()
			case "response.completed":
				for _, item := range ev.Get("response.output").Array() {
					for _, part := range item.Get("content").Array() {
						completedText += part.Get("text").String()
					}
				}
			}
		case apiChat:
			r.Text += ev.Get("choices.0.delta.content").String()
			r.ToolArgs += ev.Get("choices.0.delta.tool_calls.0.function.arguments").String()
		case apiMessages:
			switch ev.Get("type").String() {
			case "content_block_delta":
				r.Text += ev.Get("delta.text").String()
				r.ToolArgs += ev.Get("delta.partial_json").String()
			case "message_delta":
				if reason := ev.Get("delta.stop_reason").String(); reason != "" {
					r.StopReason = reason
				}
			}
		}
	}
	if api == apiResponses && !sawDelta {
		r.Text = completedText
	}
}

func tools(api string) []any {
	schema := map[string]any{"type": "object", "properties": map[string]any{"note": map[string]any{"type": "string"}}}
	if api == apiMessages {
		return []any{map[string]any{"name": "deploy", "description": "deploy", "input_schema": schema}}
	}
	return []any{map[string]any{"type": "function", "name": "deploy", "description": "deploy", "parameters": schema}}
}

// TestFormats calls both APIs for models of both providers, streamed and not, so every
// combination of client format and upstream format is translated.
func TestFormats(t *testing.T) {
	env := newTestEnv(t)
	for _, model := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a"} {
		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", model, api, stream), func(t *testing.T) {
					n := nonce("route")
					reply := "pong " + n
					res := env.A.call(t, api, model, "say pong "+n+" "+harness.Reply(reply)+" @@usage:42:7@@", stream, nil).requireOK(t)
					if res.Text != reply {
						t.Fatalf("want reply %q, got %q\n%s", reply, res.Text, res.Body)
					}
					// Obot's budgets and token usage records rely on this.
					if u := res.Usage; !u.reported || u.in != 42 || u.out != 7 {
						t.Fatalf("want 42 input and 7 output tokens reported, got %+v", u)
					}
					if stream != strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
						t.Fatalf("stream=%t but Content-Type is %q", stream, res.Header.Get("Content-Type"))
					}
					reqs := env.mock.InferenceRequests(n)
					if len(reqs) != 1 {
						t.Fatalf("want 1 upstream request, got %d", len(reqs))
					}
					provider, nativeModel, _ := strings.Cut(model, "/")
					up := reqs[0]
					if up.Model != nativeModel || up.Stream != stream {
						t.Fatalf("upstream got model %q stream=%t, want %q stream=%t", up.Model, up.Stream, nativeModel, stream)
					}
					switch provider {
					case "anthropic":
						if up.Format != harness.FormatMessages || up.Key != anthropicKey {
							t.Fatalf("anthropic model was sent as %s with key %q", up.Format, up.Key)
						}
					case "openai":
						if (api != apiChat && up.Format != harness.FormatResponses) || up.Key != openAIKey {
							t.Fatalf("openai model was sent as %s with key %q", up.Format, up.Key)
						}
					}
					// The served provider is known before the first byte, streamed or not.
					if got := res.Header.Get(headerServedProvider); got != provider {
						t.Fatalf("%s = %q, want %q", headerServedProvider, got, provider)
					}
					for name := range res.Header {
						if lower := strings.ToLower(name); strings.HasPrefix(lower, "x-bifrost") || strings.HasPrefix(lower, "x-bf-") {
							t.Fatalf("response leaks engine header %s", name)
						}
					}
					if strings.Contains(res.Body, "extra_fields") || strings.Contains(strings.ToLower(res.Body), "bifrost") {
						t.Fatalf("response leaks engine fields: %s", res.Body)
					}
				})
			}
		}
	}
}

// TestToolCallStopReason checks that a Messages client is told to run the tool when the model
// calls one, whichever provider serves it.
func TestToolCallStopReason(t *testing.T) {
	env := newTestEnv(t)
	for _, model := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				n := nonce("toolstop")
				res := env.A.call(t, apiMessages, model, n+" "+harness.ToolCall("deploy", map[string]any{"note": n}), stream, map[string]any{
					"tools": tools(apiMessages),
				}).requireOK(t)
				if !strings.Contains(res.ToolArgs, n) {
					t.Fatalf("want a tool call with %q, got %q: %s", n, res.ToolArgs, res.Body)
				}
				if res.StopReason != "tool_use" {
					t.Fatalf("want stop_reason tool_use, got %q: %s", res.StopReason, res.Body)
				}
			})
		}
	}
	t.Run("responses", func(t *testing.T) {
		for _, stream := range []bool{false, true} {
			n := nonce("tool")
			res := env.A.call(t, apiResponses, "anthropic/claude-e2e-a", n+" "+harness.ToolCall("deploy", map[string]any{"note": n}), stream, map[string]any{
				"tools": tools(apiResponses),
			}).requireOK(t)
			if !strings.Contains(res.ToolArgs, n) {
				t.Fatalf("stream=%t: want a function call with %q, got %q: %s", stream, n, res.ToolArgs, res.Body)
			}
		}
	})
}

func TestFallbacksAndRetries(t *testing.T) {
	env := newTestEnv(t)
	for _, api := range allAPIs {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				n := nonce("fallback")
				reply := "fallback reply " + n
				res := env.A.call(t, api, "openai/gpt-e2e-broken", "hi "+n+" "+harness.Reply(reply), stream, map[string]any{
					"fallbacks": []string{"anthropic/claude-e2e-a"},
				}).requireOK(t)
				if res.Text != reply {
					t.Fatalf("want the fallback's reply %q, got %q", reply, res.Text)
				}
				reqs := env.mock.InferenceRequests(n)
				if len(reqs) < 2 || reqs[0].Model != "gpt-e2e-broken" {
					t.Fatalf("want the primary then the fallback upstream, got %+v", reqs)
				}
				if last := reqs[len(reqs)-1]; last.Model != "claude-e2e-a" || last.Format != harness.FormatMessages {
					t.Fatalf("last upstream request: %s %s, want the anthropic fallback", last.Format, last.Model)
				}
				if got := res.Header.Get(headerServedProvider); got != "anthropic" {
					t.Fatalf("%s = %q, want the fallback's provider", headerServedProvider, got)
				}
			})
		}
	}

	t.Run("a refused fallback is skipped", func(t *testing.T) {
		for _, api := range allAPIs {
			n := nonce("fallback-denied")
			env.A.call(t, api, "openai/gpt-e2e-broken", "hi "+n, false, map[string]any{
				"fallbacks": []string{"openai/" + deniedModel, "openai/gpt-e2e-a"},
			}).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			for _, r := range reqs {
				if r.Model == deniedModel {
					t.Fatalf("%s: a refused fallback reached the provider", api)
				}
			}
			if len(reqs) == 0 || reqs[len(reqs)-1].Model != "gpt-e2e-a" {
				t.Fatalf("%s: want the request served by the allowed fallback, upstream got %+v", api, reqs)
			}
		}
	})

	t.Run("a refused model is forbidden", func(t *testing.T) {
		for _, api := range allAPIs {
			n := nonce("denied")
			res := env.A.call(t, api, "openai/"+deniedModel, "hi "+n, false, nil)
			if res.Status != http.StatusForbidden || res.ErrorMsg == "" {
				t.Fatalf("%s: want a 403 error, got %d: %s", api, res.Status, res.Body)
			}
			if len(env.mock.InferenceRequests(n)) != 0 {
				t.Fatalf("%s: a refused request reached the provider", api)
			}
		}
	})

	t.Run("an invalid request isn't sent to fallbacks", func(t *testing.T) {
		for _, api := range allAPIs {
			n := nonce("fallback-invalid")
			res := env.A.call(t, api, "openai/gpt-e2e-a", "hi "+n+" @@status:400@@", false, map[string]any{
				"fallbacks": []string{"anthropic/claude-e2e-a"},
			})
			if res.Status != http.StatusBadRequest {
				t.Fatalf("%s: want the provider's 400, got %d: %s", api, res.Status, res.Body)
			}
			if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 {
				t.Fatalf("%s: want only the primary called upstream, got %d requests", api, len(reqs))
			}
		}
	})

	t.Run("no fallback: the error is returned in the request's format", func(t *testing.T) {
		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				res := env.A.call(t, api, "openai/gpt-e2e-broken", "hi "+nonce("nofallback"), stream, nil)
				if res.Status < http.StatusInternalServerError || res.ErrorMsg == "" {
					t.Fatalf("%s stream=%t: want a 5xx error, got %d: %s", api, stream, res.Status, res.Body)
				}
				if anthropic := gjson.Get(res.Body, "type").String() == "error"; anthropic != (api == apiMessages) {
					t.Fatalf("%s: error isn't in the request's format: %s", api, res.Body)
				}
				if strings.Contains(strings.ToLower(res.Body), "bifrost") {
					t.Fatalf("%s: error names the engine: %s", api, res.Body)
				}
			}
		}
	})

	t.Run("an unknown provider is an error in the request's format", func(t *testing.T) {
		for _, api := range allAPIs {
			res := env.A.call(t, api, "nosuch/model", "hi", false, nil)
			if res.Status < http.StatusBadRequest || res.Status >= http.StatusInternalServerError || res.ErrorMsg == "" {
				t.Fatalf("%s: want a 4xx error, got %d: %s", api, res.Status, res.Body)
			}
		}
	})

	t.Run("a provider added at runtime retries transient failures", func(t *testing.T) {
		ctx := context.Background()
		const provider, key = "e2eretry", "e2e-upstream-e2eretry"
		pc := providerConfig(env.mock.URL)
		pc.NetworkConfig.MaxRetries = 3
		pc.NetworkConfig.RetryBackoffInitial = 100 * time.Millisecond
		pc.NetworkConfig.RetryBackoffMax = 200 * time.Millisecond
		pc.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}
		if err := env.store.CreateProvider(ctx, provider, llmcoreclient.Provider{Config: pc}); err != nil {
			t.Fatal(err)
		}
		if err := env.store.PutKey(ctx, provider, apiKey(provider+"-1", key)); err != nil {
			t.Fatal(err)
		}
		env.A.sync(t)

		for _, api := range allAPIs {
			n := nonce("retry")
			env.A.call(t, api, provider+"/retry-model", "hi "+n+" @@flaky:2@@", false, nil).requireOK(t)
			if got := len(env.mock.InferenceRequests(n)); got != 3 {
				t.Fatalf("%s: want 2 failed attempts and 1 success upstream, got %d attempts", api, got)
			}
		}
	})
}

// TestChatCompletionsOnlyProvider checks that Responses and Messages clients can use a provider
// that only has Chat Completions, as many self-hosted OpenAI-compatible servers do. Clients
// don't need the Chat Completions API for that: the engine translates upstream.
func TestChatCompletionsOnlyProvider(t *testing.T) {
	env := newTestEnv(t)
	const provider, key = "e2echat", "e2e-upstream-e2echat"
	env.mock.SetChatOnly(key)

	var allowed schemas.AllowedRequests
	if err := json.Unmarshal([]byte(`{"list_models":true,"chat_completion":true,"chat_completion_stream":true,"embedding":true}`), &allowed); err != nil {
		t.Fatal(err)
	}
	pc := providerConfig(env.mock.URL)
	pc.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI, AllowedRequests: &allowed}
	ctx := context.Background()
	if err := env.store.CreateProvider(ctx, provider, llmcoreclient.Provider{Config: pc}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.PutKey(ctx, provider, apiKey(provider+"-1", key)); err != nil {
		t.Fatal(err)
	}
	env.A.sync(t)

	for _, api := range allAPIs {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				n := nonce("chatonly")
				reply := "chat only reply " + n
				res := env.A.call(t, api, provider+"/chat-model", "hi "+n+" "+harness.Reply(reply), stream, nil).requireOK(t)
				if res.Text != reply {
					t.Fatalf("want the reply %q, got %q: %s", reply, res.Text, res.Body)
				}
				reqs := env.mock.InferenceRequests(n)
				if len(reqs) == 0 {
					t.Fatal("no upstream request")
				}
				for _, r := range reqs {
					if r.Path != "/v1/chat/completions" {
						t.Fatalf("the engine called %s upstream, want only /v1/chat/completions", r.Path)
					}
				}
			})
		}
	}
}

// TestReplicaSync changes the configuration through the shared database and checks that each
// replica serves the change once it syncs, with no rebuild and no restart.
func TestReplicaSync(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	routedModel := func(r *replica, api string) string {
		t.Helper()
		n := nonce("routed")
		r.call(t, api, "openai/gpt-e2e-a", "hi "+n, false, nil, map[string]string{"X-E2E-Route": "yes"}).requireOK(t)
		reqs := env.mock.InferenceRequests(n)
		if len(reqs) != 1 {
			t.Fatalf("want 1 upstream request, got %d", len(reqs))
		}
		return reqs[0].Model
	}

	t.Run("routing rule added, then changed on another replica", func(t *testing.T) {
		provider, model := "openai", "gpt-e2e-b"
		rule := configstoreTables.TableRoutingRule{
			ID:            "rule-1",
			Name:          "e2e header route",
			CelExpression: `"x-e2e-route" in headers && headers["x-e2e-route"] == "yes"`,
			Targets:       []configstoreTables.TableRoutingTarget{{Provider: &provider, Model: &model, Weight: 1}},
			Scope:         "global",
			Priority:      1,
		}
		if err := env.store.PutRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		if got := routedModel(env.A, apiResponses); got != "gpt-e2e-a" {
			t.Fatalf("before syncing, A routed to %s", got)
		}
		env.A.sync(t)
		env.B.sync(t)
		for _, api := range allAPIs {
			for _, r := range []*replica{env.A, env.B} {
				if got := routedModel(r, api); got != "gpt-e2e-b" {
					t.Fatalf("%s: want the rule to route to gpt-e2e-b, got %s", api, got)
				}
			}
		}

		provider, model = "anthropic", "claude-e2e-b"
		if err := env.store.PutRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		env.B.sync(t)
		if got := routedModel(env.B, apiMessages); got != "claude-e2e-b" {
			t.Fatalf("B: want the changed rule to route to claude-e2e-b, got %s", got)
		}
		if got := routedModel(env.A, apiMessages); got != "gpt-e2e-b" {
			t.Fatalf("A hasn't synced, so it should still route to gpt-e2e-b, got %s", got)
		}
		env.A.sync(t)
		if got := routedModel(env.A, apiMessages); got != "claude-e2e-b" {
			t.Fatalf("A: want the changed rule after syncing, got %s", got)
		}

		if err := env.store.DeleteRule(ctx, rule.ID); err != nil {
			t.Fatal(err)
		}
		env.A.sync(t)
		if got := routedModel(env.A, apiResponses); got != "gpt-e2e-a" {
			t.Fatalf("want no routing once the rule is deleted, got %s", got)
		}
	})

	t.Run("key rotated", func(t *testing.T) {
		const rotated = "e2e-upstream-openai-key-2"
		if err := env.store.PutKey(ctx, "openai", apiKey("openai-2", rotated)); err != nil {
			t.Fatal(err)
		}
		if err := env.store.DeleteKey(ctx, "openai", "openai-1"); err != nil {
			t.Fatal(err)
		}
		for _, r := range []*replica{env.A, env.B} {
			r.sync(t)
			n := nonce("rotated")
			r.call(t, apiResponses, "openai/gpt-e2e-a", "hi "+n, false, nil).requireOK(t)
			if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 || reqs[0].Key != rotated {
				t.Fatalf("want the rotated key upstream, got %+v", reqs)
			}
		}
	})

	t.Run("provider deleted", func(t *testing.T) {
		if err := env.store.DeleteProvider(ctx, "anthropic"); err != nil {
			t.Fatal(err)
		}
		env.A.sync(t)
		res := env.A.call(t, apiMessages, "anthropic/claude-e2e-a", "hi", false, nil)
		if res.Status == http.StatusOK {
			t.Fatalf("a deleted provider still served the request: %s", res.Body)
		}
		// B hasn't synced and still has it.
		env.B.call(t, apiMessages, "anthropic/claude-e2e-a", "hi", false, nil).requireOK(t)
	})

	t.Run("sync with no change does nothing", func(t *testing.T) {
		changed, err := env.A.engine.Sync(ctx)
		if err != nil || changed {
			t.Fatalf("want no change, got changed=%t err=%v", changed, err)
		}
	})
}

// A failed provider update must be retried even if no administrator writes again.
func TestSyncRetriesFailedConfiguration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if err := env.store.CreateProvider(ctx, "unsupported-test-provider", llmcoreclient.Provider{Config: providerConfig(env.mock.URL)}); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		changed, err := env.A.engine.Sync(ctx)
		if !changed || err == nil {
			t.Fatalf("attempt %d: expected failed update, got changed=%t err=%v", attempt, changed, err)
		}

	}
	if err := env.store.DeleteProvider(ctx, "unsupported-test-provider"); err != nil {
		t.Fatal(err)
	}
	env.A.sync(t)
	changed, err := env.A.engine.Sync(ctx)
	if err != nil || changed {
		t.Fatalf("recovery did not converge: changed=%t err=%v", changed, err)
	}

}

func TestResponseObserver(t *testing.T) {
	env := newTestEnv(t)
	for _, api := range allAPIs {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				var observed []schemas.BifrostResponseExtraFields
				ctx := llmcore.WithResponseObserver(context.Background(), func(extra schemas.BifrostResponseExtraFields) { observed = append(observed, extra) })
				path := "/v1/responses"
				body := fmt.Sprintf(`{"model":"openai/gpt-e2e-a","input":"hello","stream":%t}`, stream)
				if api == apiMessages {
					path = "/v1/messages"
					body = fmt.Sprintf(`{"model":"openai/gpt-e2e-a","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
				} else if api == apiChat {
					path = "/v1/chat/completions"
					body = fmt.Sprintf(`{"model":"openai/gpt-e2e-a","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
				}
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
				rec := httptest.NewRecorder()
				env.A.engine.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
				}
				if len(observed) == 0 || observed[0].RoutingInfo.Provider != schemas.OpenAI {
					t.Fatalf("missing internal routing metadata: %+v", observed)
				}
				if stream && len(observed) < 2 {
					t.Fatalf("observer missed later stream chunks: %+v", observed)
				}
				if strings.Contains(rec.Body.String(), "extra_fields") {
					t.Fatal("internal metadata leaked to client")
				}
			})
		}
	}
}

func TestRouteSurvivesMissingProvider(t *testing.T) {
	env := newTestEnv(t)
	ctx := t.Context()
	provider, model := "anthropic", "claude-e2e-a"
	rule := configstoreTables.TableRoutingRule{
		ID:            "persistent-route",
		Name:          "route to independently configured provider",
		CelExpression: "true",
		Targets:       []configstoreTables.TableRoutingTarget{{Provider: &provider, Model: &model, Weight: 1}},
		Scope:         "global",
		Priority:      1,
	}
	if err := env.store.PutRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := env.store.DeleteProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	for _, replica := range []*replica{env.A, env.B} {
		for attempt := range 2 {
			changed, err := replica.engine.Sync(ctx)
			if changed != (attempt == 0) || err != nil {
				t.Fatalf("applied route must converge despite its missing target: changed=%t err=%v", changed, err)
			}
			if diagnostic := replica.engine.Err(); diagnostic == nil || !strings.Contains(diagnostic.Error(), "anthropic is not configured") {
				t.Fatalf("missing target must remain visible: %v", diagnostic)
			}
		}
		n := nonce("missing-provider")
		result := replica.call(t, apiResponses, "openai/gpt-e2e-a", "hi "+n, false, nil)
		if result.Status < http.StatusBadRequest {
			t.Fatalf("missing routing target unexpectedly succeeded: %s", result.Body)
		}
		if requests := env.mock.InferenceRequests(n); len(requests) != 0 {
			t.Fatalf("missing target silently bypassed routing: %+v", requests)
		}
	}
	loaded, err := env.store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Config.Rules[rule.ID]; !ok {
		t.Fatal("provider deletion removed its routing rule")
	}
	if err := env.store.CreateProvider(ctx, provider, llmcoreclient.Provider{Config: providerConfig(env.mock.URL)}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.PutKey(ctx, provider, apiKey("recreated", anthropicKey)); err != nil {
		t.Fatal(err)
	}
	for _, replica := range []*replica{env.A, env.B} {
		replica.sync(t)
		replica.call(t, apiResponses, "openai/gpt-e2e-a", "hi "+nonce("restored-provider"), false, nil).requireOK(t)
	}
}

func TestMissingFallbackPreservesHealthyPrimary(t *testing.T) {
	env := newTestEnv(t)
	ctx := t.Context()
	provider, model := "openai", "gpt-e2e-a"
	rule := configstoreTables.TableRoutingRule{
		ID: "optional-fallback", Name: "optional-fallback", CelExpression: "true", Scope: "global",
		Targets:         []configstoreTables.TableRoutingTarget{{Provider: &provider, Model: &model, Weight: 1}},
		ParsedFallbacks: []configstoreTables.RoutingFallback{{Fallback: schemas.Fallback{Provider: schemas.Anthropic, Model: "claude-e2e-a"}}},
	}
	if err := env.store.PutRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := env.store.DeleteProvider(ctx, "anthropic"); err != nil {
		t.Fatal(err)
	}
	env.A.sync(t)
	for _, api := range allAPIs {
		env.A.call(t, api, "openai/gpt-e2e-a", "hi "+nonce("missing-fallback"), false, nil).requireOK(t)
	}
}
