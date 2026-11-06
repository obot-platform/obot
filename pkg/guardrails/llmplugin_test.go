package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/streaming"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	leakedKey = "AKIAIOSFODNN7EXAMPLE"
	// invalidRequest in a request makes the upstream reject it as invalid, with HTTP 400.
	invalidRequest = "INVALID_REQUEST"
)

// upstream is a mock OpenAI API whose answers contain a secret, and which records the
// prompts it receives.
type upstream struct {
	*httptest.Server
	lock    sync.Mutex
	prompts []string
}

type plaintextGatewayCodec struct{}

func (plaintextGatewayCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (plaintextGatewayCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.lock.Lock()
		u.prompts = append(u.prompts, string(body))
		u.lock.Unlock()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		switch {
		case strings.Contains(string(body), invalidRequest) && strings.HasSuffix(r.URL.Path, "/messages"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"Could not process image"}}`)
		case strings.Contains(string(body), invalidRequest):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"You uploaded an unsupported image.","type":"invalid_request_error","param":null,"code":"invalid_image_format"}}`)
		case strings.HasSuffix(r.URL.Path, "/messages/count_tokens"):
			_, _ = io.WriteString(w, `{"input_tokens":12}`)
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = fmt.Fprintf(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"Your key is %s ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`, leakedKey)
		case strings.HasSuffix(r.URL.Path, "/chat/completions") && req["stream"] == true && strings.Contains(string(body), "THINK"):
			// The secret is split across reasoning deltas, before any content.
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []string{"I recall " + leakedKey[:10], leakedKey[10:] + " here"} {
				_, _ = fmt.Fprintf(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning\":%q}}]}\n\n", part)
			}
			_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-4o-mini","object":"model"}]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions") && req["stream"] == true:
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []string{"Your key ", "is " + leakedKey[:10], leakedKey[10:] + " ok"} {
				_, _ = fmt.Fprintf(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", part)
			}
			_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = fmt.Fprintf(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"Your key is %s ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`, leakedKey)
		default:
			http.NotFound(w, r)
		}
	}))
	return u
}

func (u *upstream) lastPrompt() string {
	u.lock.Lock()
	defer u.lock.Unlock()
	for _, p := range slices.Backward(u.prompts) {
		if strings.Contains(p, "messages") {
			return p
		}
	}
	return ""
}

func startGateway(t *testing.T, store *fakeStore) (*llmengine.Engine, *upstream) {
	t.Helper()
	up := newUpstream(t)
	t.Cleanup(up.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Guardrails are registered as part of startup, as Obot does, so the gateway never
	// serves a request without them.
	engine := New(store, nil)
	input, output := engine.Plugins()
	gw := llmengine.New()
	// Model prices for the tests that check what the gateway charges.
	dataDir := t.TempDir()
	modeldatatest.Write(t, dataDir)
	configStore, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(), plaintextGatewayCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gw.Start(ctx, llmengine.Options{Store: configStore, DataDir: dataDir, FirstPlugin: input, LastPlugin: output}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { require.NoError(t, gw.Shutdown(context.Background())) })
	if _, err := gw.CreateProvider(ctx, gatewayJSON(t, map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.CreateKey(ctx, "openai", gatewayJSON(t, map[string]any{"name": "k", "value": "sk-test", "models": []string{"*"}, "weight": 1})); err != nil {
		t.Fatal(err)
	}

	if _, err := gw.CreateProvider(ctx, gatewayJSON(t, map[string]any{
		"provider":       "anthropic",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.CreateKey(ctx, "anthropic", gatewayJSON(t, map[string]any{"name": "ka", "value": "sk-ant-test", "models": []string{"*"}, "weight": 1})); err != nil {
		t.Fatal(err)
	}

	if _, err := gw.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Request logging must only ever see checked content: the input plugin runs before it,
	// and the output plugin after it (so its response hook runs before logging's).
	order := gw.LLMPluginOrder()
	pos := map[string]int{}
	for i, name := range order {
		pos[name] = i
	}
	if order[0] != InputPluginName || pos[OutputPluginName] < pos["logging"] {
		t.Fatalf("guardrail plugins must surround request logging, got %v", order)
	}
	return gw, up
}

func call(t *testing.T, gw *llmengine.Engine, body string, bind bool) (int, string) {
	t.Helper()
	return callPath(t, gw, "/v1/chat/completions", body, bind, nil)
}

func callPath(t *testing.T, gw *llmengine.Engine, path, body string, bind bool, headers map[string]string) (int, string) {
	t.Helper()
	var sink DetectionSink
	if bind {
		sink = func(string, []Detection) {}
	}
	return callWithSink(t, gw, path, body, headers, sink)
}

// callWithSink sends a request bound to a test user whose guardrail detections go to sink,
// or an unbound request when sink is nil.
func callWithSink(t *testing.T, gw *llmengine.Engine, path, body string, headers map[string]string, sink DetectionSink) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if sink != nil {
		token, release := BindRequest(Identity{UserID: "alice", ProfileIDs: []string{testProfile}}, RequestHooks{Detections: sink})
		defer release()
		req.Header.Set(HeaderRequestToken, token)
	}
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b)
}

// The gateway prices each response for the model that served it, from the model data Obot
// bundles, and reports the cost to whoever bound the request, with or without guardrail policies.
func TestGatewayReportsCost(t *testing.T) {
	for _, withPolicies := range []bool{false, true} {
		t.Run(fmt.Sprintf("guardrail policies %v", withPolicies), func(t *testing.T) {
			store := &fakeStore{}
			if withPolicies {
				store.guardrails = []types.Guardrail{guardrail("secrets", "secrets", nil)}
				store.policies = []types.GuardrailPolicy{policy("redact-secrets", types.GuardrailActionRedact, "secrets")}
			}
			gw, _ := startGateway(t, store)
			// The upstream reports 5 input and 3 output tokens; gpt-4o-mini costs $0.15 and $0.60
			// per million.
			const want = 5*0.15e-6 + 3*0.6e-6
			prompt := `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`
			for _, stream := range []bool{false, true} {
				body := prompt
				if stream {
					body = strings.Replace(prompt, `"messages"`, `"stream":true,"stream_options":{"include_usage":true},"messages"`, 1)
				}
				var (
					lock  sync.Mutex
					costs []Cost
				)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				token, release := BindRequest(Identity{UserID: "alice", ProfileIDs: []string{testProfile}}, RequestHooks{Cost: func(c Cost) {
					lock.Lock()
					defer lock.Unlock()
					costs = append(costs, c)
				}})
				req.Header.Set(HeaderRequestToken, token)
				rec := httptest.NewRecorder()
				gw.ServeHTTP(rec, req)
				release()
				if rec.Code != http.StatusOK {
					t.Fatalf("stream=%v: %d %s", stream, rec.Code, rec.Body.String())
				}
				lock.Lock()
				var best float64
				for _, c := range costs {
					best = math.Max(best, c.Input+c.Output)
				}
				lock.Unlock()
				if math.Abs(best-want) > want*1e-6 {
					t.Errorf("stream=%v: cost = %v (reports %+v), want %v", stream, best, costs, want)
				}
				if last := costs[len(costs)-1]; last.InputTokens != 5 || last.OutputTokens != 3 {
					t.Errorf("stream=%v: tokens = %d in, %d out; want 5, 3", stream, last.InputTokens, last.OutputTokens)
				}
			}
		})
	}
}

func TestGatewayPluginsRedact(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{policy("redact-secrets", types.GuardrailActionRedact, "secrets")},
	}
	gw, up := startGateway(t, store)

	prompt := `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"my key is ` + leakedKey + `"}]}`

	// Input is redacted before it reaches the provider, and output before the caller.
	code, body := call(t, gw, prompt, true)
	if code != http.StatusOK || strings.Contains(body, leakedKey) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("non-streamed output not redacted: %d %s", code, body)
	}
	if p := up.lastPrompt(); strings.Contains(p, leakedKey) || !strings.Contains(p, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("input not redacted before the provider: %s", p)
	}

	// A secret split across stream chunks is still found and redacted.
	code, body = call(t, gw, strings.Replace(prompt, `"messages"`, `"stream":true,"messages"`, 1), true)
	if code != http.StatusOK || strings.Contains(body, leakedKey) || strings.Contains(body, leakedKey[:10]) {
		t.Fatalf("streamed output not redacted: %d %s", code, body)
	}
	var streamed strings.Builder
	for line := range strings.SplitSeq(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) == nil {
			for _, c := range chunk.Choices {
				streamed.WriteString(c.Delta.Content)
			}
		}
	}
	if streamed.String() != "Your key is [REDACTED:AWS_ACCESS_KEY_ID] ok" {
		t.Fatalf("unexpected streamed text %q in %s", streamed.String(), body)
	}

	// A secret split across streamed reasoning is found, and never reaches the client.
	code, body = call(t, gw, `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"THINK"}]}`, true)
	if code != http.StatusOK || strings.Contains(body, leakedKey[:10]) || strings.Contains(body, leakedKey[10:]) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("streamed reasoning not redacted: %d %s", code, body)
	}

	// Claude Code requests are forwarded raw by default; guarded ones must not be.
	claudeCode := map[string]string{"User-Agent": "claude-cli/2.0.0 (external, cli)", "anthropic-version": "2023-06-01"}
	anthropicPrompt := `{"model":"anthropic/claude-sonnet-4-5","max_tokens":100,"messages":[{"role":"user","content":"my key is ` + leakedKey + `"}]}`
	code, body = callPath(t, gw, "/anthropic/v1/messages", anthropicPrompt, true, claudeCode)
	if code != http.StatusOK || strings.Contains(body, leakedKey) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("Claude Code output not redacted: %d %s", code, body)
	}
	if p := up.lastPrompt(); strings.Contains(p, leakedKey) || !strings.Contains(p, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("Claude Code input reached the provider unredacted: %s", p)
	}

	// Token counting sends the prompt to the provider too.
	code, body = callPath(t, gw, "/anthropic/v1/messages/count_tokens", `{"model":"anthropic/claude-sonnet-4-5","messages":[{"role":"user","content":"my key is `+leakedKey+`"}]}`, true, claudeCode)
	if code != http.StatusOK {
		t.Fatalf("count_tokens failed: %d %s", code, body)
	}
	if p := up.lastPrompt(); strings.Contains(p, leakedKey) {
		t.Fatalf("count_tokens input reached the provider unredacted: %s", p)
	}

	// Requests Obot didn't bind (e.g. the LLM judge's own calls) are not guarded.
	if _, body := call(t, gw, prompt, false); !strings.Contains(body, leakedKey) {
		t.Fatalf("unbound requests should pass untouched: %s", body)
	}
}

func TestGatewayPluginsBlock(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
	}
	gw, up := startGateway(t, store)

	// Input block: the provider is never called.
	before := len(up.prompts)
	code, body := call(t, gw, `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"key `+leakedKey+`"}]}`, true)
	if code != http.StatusBadRequest || !strings.Contains(body, "block-secrets") {
		t.Fatalf("expected an input block, got %d %s", code, body)
	}
	if len(up.prompts) != before {
		t.Fatal("a blocked request must not reach the provider")
	}

	// Output block on a stream: none of the held content is delivered.
	code, body = call(t, gw, `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`, true)
	if strings.Contains(body, leakedKey[:10]) || strings.Contains(body, "Your key") || !strings.Contains(body, "block-secrets") {
		t.Fatalf("expected the stream to end with a guardrail error and no content, got %d %s", code, body)
	}

	// Output block without streaming.
	code, body = call(t, gw, `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, true)
	if code != http.StatusBadRequest || strings.Contains(body, leakedKey) {
		t.Fatalf("expected an output block, got %d %s", code, body)
	}
}

func TestGatewayMetricsIncludeBlockedOutput(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
	}
	gw, _ := startGateway(t, store)
	code, body := call(t, gw, `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, true)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "block-secrets")
	require.NotContains(t, body, leakedKey)

	scrape := httptest.NewRecorder()
	gw.ServeMetrics(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, scrape.Code)
	require.NotContains(t, scrape.Body.String(), `provider=""`)
	require.Contains(t, scrape.Body.String(), "obot_llm_gateway_guardrail_blocks_total{stage=\"output\"} 1\n")
	const labels = `key="k",model="gpt-4o-mini",provider="openai"`
	// Blocking delivery does not undo the provider's billed 5 input and 3 output tokens.
	for series, want := range map[string]float64{
		`obot_llm_gateway_requests_total{` + labels + `}`:                  1,
		`obot_llm_gateway_attempt_duration_seconds_count{` + labels + `}`:  1,
		`obot_llm_gateway_tokens_total{direction="input",` + labels + `}`:  5,
		`obot_llm_gateway_tokens_total{direction="output",` + labels + `}`: 3,
		`obot_llm_gateway_cost_total{` + labels + `}`:                      5*0.15e-6 + 3*0.6e-6,
	} {
		found := false
		for line := range strings.SplitSeq(scrape.Body.String(), "\n") {
			if !strings.HasPrefix(line, series+" ") {
				continue
			}
			var got float64
			_, err := fmt.Sscanf(strings.TrimPrefix(line, series+" "), "%f", &got)
			require.NoError(t, err)
			require.InDelta(t, want, got, want*1e-6, series)
			found = true
		}
		require.True(t, found, "missing metrics series %s", series)
	}
}

func TestGatewayGuardrailBlockMetrics(t *testing.T) {
	for _, tc := range []struct {
		name       string
		action     string
		input      string
		stream     bool
		loadError  bool
		failClosed bool
		stage      string
	}{
		{
			name:   "input block",
			action: types.GuardrailActionBlock,
			input:  leakedKey,
			stage:  StageInput,
		},
		{
			name:   "stream output block",
			action: types.GuardrailActionBlock,
			stream: true,
			stage:  StageOutput,
		},
		{
			name:      "fail-closed policy loading",
			action:    types.GuardrailActionBlock,
			loadError: true,
			stage:     StageInput,
		},
		{
			name:       "fail-closed output evaluation",
			action:     types.GuardrailActionBlock,
			failClosed: true,
			stage:      StageOutput,
		},
		{
			name:   "redaction is not a block",
			action: types.GuardrailActionRedact,
		},
		{
			name:   "logging is not a block",
			action: types.GuardrailActionLog,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
				policies:   []types.GuardrailPolicy{policy("secrets-policy", tc.action, "secrets")},
			}
			if tc.failClosed {
				store.guardrails = []types.Guardrail{guardrail("secrets", "test-failing", nil)}
				store.policies[0].ApplyTo = types.GuardrailApplyOutput
				store.policies[0].FailClosed = true
			}
			gw, _ := startGateway(t, store)
			if tc.loadError {
				store.loadErr = fmt.Errorf("policy storage unavailable")
			}
			prompt := "hello " + tc.input
			code, body := call(t, gw, fmt.Sprintf(`{"model":"openai/gpt-4o-mini","stream":%t,"messages":[{"role":"user","content":%q}]}`, tc.stream, prompt), true)
			if tc.stage == "" {
				require.Equal(t, http.StatusOK, code, body)
			} else {
				require.Contains(t, body, "guardrail_intervention")
			}
			scrape := httptest.NewRecorder()
			gw.ServeMetrics(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if tc.stage == "" {
				require.NotContains(t, scrape.Body.String(), "obot_llm_gateway_guardrail_blocks_total{")
			} else {
				require.Contains(t, scrape.Body.String(), `obot_llm_gateway_guardrail_blocks_total{stage="`+tc.stage+"\"} 1\n")
			}
			if tc.stage == StageInput {
				require.NotContains(t, scrape.Body.String(), "obot_llm_gateway_requests_total{")
			}
		})
	}
}

func secretDetection(stage, policyName, outcome string) Detection {
	return Detection{
		Stage:         stage,
		PolicyID:      policyName,
		PolicyName:    policyName,
		GuardrailName: "secrets",
		GuardrailType: "secrets",
		Outcome:       outcome,
		Categories:    []string{"secret"},
	}
}

// TestGatewayPluginsReportDetections checks that what the guardrails did to a request is
// reported to whoever bound it, without the content, for blocked, redacted and logged
// findings, with and without streaming.
func TestGatewayPluginsReportDetections(t *testing.T) {
	const (
		secretPrompt       = `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"key ` + leakedKey + `"}]}`
		secretStreamPrompt = `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"key ` + leakedKey + `"}]}`
		// The mock provider answers every prompt with a secret.
		cleanPrompt       = `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`
		cleanStreamPrompt = `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	)
	logAndRedact := []Detection{
		secretDetection(StageInput, "log-secrets", "logged"),
		secretDetection(StageInput, "redact-secrets", "redacted"),
		secretDetection(StageOutput, "log-secrets", "logged"),
		secretDetection(StageOutput, "redact-secrets", "redacted"),
	}
	tests := []struct {
		name     string
		policies []types.GuardrailPolicy
		body     string
		want     []Detection
		// delivered reports that the response's secret reaches the caller.
		delivered bool
	}{
		{
			name:     "input blocked",
			policies: []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
			body:     secretPrompt,
			want:     []Detection{secretDetection(StageInput, "block-secrets", "blocked")},
		},
		{
			name:     "streamed input blocked",
			policies: []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
			body:     secretStreamPrompt,
			want:     []Detection{secretDetection(StageInput, "block-secrets", "blocked")},
		},
		{
			name:     "output blocked",
			policies: []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
			body:     cleanPrompt,
			want:     []Detection{secretDetection(StageOutput, "block-secrets", "blocked")},
		},
		{
			name:     "streamed output blocked",
			policies: []types.GuardrailPolicy{policy("block-secrets", types.GuardrailActionBlock, "secrets")},
			body:     cleanStreamPrompt,
			want:     []Detection{secretDetection(StageOutput, "block-secrets", "blocked")},
		},
		{
			name: "logged and redacted",
			policies: []types.GuardrailPolicy{
				policy("log-secrets", types.GuardrailActionLog, "secrets"),
				policy("redact-secrets", types.GuardrailActionRedact, "secrets"),
			},
			body: secretPrompt,
			want: logAndRedact,
		},
		{
			name: "streamed, logged and redacted",
			policies: []types.GuardrailPolicy{
				policy("log-secrets", types.GuardrailActionLog, "secrets"),
				policy("redact-secrets", types.GuardrailActionRedact, "secrets"),
			},
			body: secretStreamPrompt,
			want: logAndRedact,
		},
		{
			// Nothing is held back when no policy changes the response.
			name:      "streamed and only logged",
			policies:  []types.GuardrailPolicy{policy("log-secrets", types.GuardrailActionLog, "secrets")},
			body:      cleanStreamPrompt,
			want:      []Detection{secretDetection(StageOutput, "log-secrets", "logged")},
			delivered: true,
		},
		{
			name: "guardrail failed",
			policies: []types.GuardrailPolicy{
				policy("broken", types.GuardrailActionBlock, "missing"),
			},
			body: cleanPrompt,
			want: []Detection{
				{Stage: StageInput, PolicyID: "broken", PolicyName: "broken", GuardrailName: "missing", Outcome: "error"},
				{Stage: StageOutput, PolicyID: "broken", PolicyName: "broken", GuardrailName: "missing", Outcome: "error"},
			},
			// The policy fails open.
			delivered: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
				policies:   tt.policies,
			}
			gw, _ := startGateway(t, store)

			var (
				lock    sync.Mutex
				byStage = map[string][]Detection{}
			)
			sink := func(stage string, detections []Detection) {
				lock.Lock()
				defer lock.Unlock()
				byStage[stage] = detections
			}
			code, body := callWithSink(t, gw, "/v1/chat/completions", tt.body, nil, sink)
			// A streamed secret is split across chunks.
			if strings.Contains(body, leakedKey[:10]) != tt.delivered {
				t.Fatalf("unexpected response: %d %s", code, body)
			}

			lock.Lock()
			got := append(slices.Clone(byStage[StageInput]), byStage[StageOutput]...)
			lock.Unlock()
			if !slices.EqualFunc(got, tt.want, func(a, b Detection) bool {
				return a.Stage == b.Stage && a.PolicyID == b.PolicyID && a.PolicyName == b.PolicyName && a.GuardrailName == b.GuardrailName &&
					a.GuardrailType == b.GuardrailType && a.Outcome == b.Outcome && slices.Equal(a.Categories, b.Categories)
			}) {
				t.Errorf("detections = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCoreStreamObserverSeesGuardrailReplay(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{policy("redact-secrets", types.GuardrailActionRedact, "secrets")},
	}
	gw, _ := startGateway(t, store)
	var lock sync.Mutex
	var observed strings.Builder
	token, release := BindRequest(Identity{UserID: "alice", ProfileIDs: []string{testProfile}}, RequestHooks{})
	defer release()
	ctx := llmcore.WithStreamObserver(t.Context(), func(_ *schemas.BifrostContext, chunk *schemas.BifrostStreamChunk, _ *streaming.Accumulator) {
		data, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		lock.Lock()
		defer lock.Unlock()
		observed.Write(data)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
	req.Header.Set(HeaderRequestToken, token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	code, body := rec.Code, rec.Body.String()
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	lock.Lock()
	defer lock.Unlock()
	if strings.Contains(observed.String(), leakedKey[:10]) || !strings.Contains(observed.String(), "REDACTED:AWS_ACCESS_KEY_ID") {
		t.Fatalf("observer saw %s", observed.String())
	}
}

func gatewayJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
