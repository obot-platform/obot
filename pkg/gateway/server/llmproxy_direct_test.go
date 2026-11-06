package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	apiServer "github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/license"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Give the access helper its real indexes without running a controller or server.
type directAccessBackend struct {
	backend.Backend
	storage   kclient.Client
	informers map[string]cache.SharedIndexInformer
}

type directCodec struct{}

type directFixture struct {
	next    chan struct{}
	drained chan struct{}
	before  bool
	later   bool
	headers map[string]string
	query   map[string]string
}

type notifyingWriter struct {
	*httptest.ResponseRecorder
	once  sync.Once
	first chan struct{}
	fail  bool
}

// A broken index simulates the storage/access error during a buffered model list.
type failedAccessBackend struct{ directAccessBackend }

type failingInformer struct{ cache.SharedIndexInformer }

type failedIndexer struct{ cache.Indexer }

type panickingBackend struct {
	*bifrostProviderBackend
	released int
	stream   bool
	ctx      context.Context
}

type panicStatusWriter struct {
	*httptest.ResponseRecorder
	statuses []int
}

// Capture the real direct writer without replacing engine execution or observers.
type mountedDirectBackend struct {
	*bifrostProviderBackend
	output *llmResponseWriter
}

type publicFlushFailureWriter struct {
	panicStatusWriter
	flushes int
}

func (b *directAccessBackend) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return b.storage.GroupVersionKindFor(obj.(kclient.Object))
}
func (b *directAccessBackend) GetInformerForKind(_ context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	obj, err := scheme.Scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	i := cache.NewSharedIndexInformer(&cache.ListWatch{}, obj, 0, cache.Indexers{})
	b.informers[gvk.Kind] = i
	return i, nil
}

func (directCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (directCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (*directFixture) GetName() string { return "direct-fixture" }
func (*directFixture) Cleanup() error  { return nil }
func (p *directFixture) PreRequestHook(ctx *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	p.headers, _ = ctx.Value(schemas.BifrostContextKeyRequestHeaders).(map[string]string)
	p.query, _ = ctx.Value(schemas.BifrostContextKeyRequestQuery).(map[string]string)
	return nil
}
func (*directFixture) PostLLMHook(_ *schemas.BifrostContext, r *schemas.BifrostResponse, e *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return r, e, nil
}
func (p *directFixture) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	failure := &schemas.BifrostError{StatusCode: new(http.StatusTooManyRequests), Error: &schemas.ErrorField{Type: new("rate_limit_error"), Message: "Bifrost: upstream refused"}}
	if p.before {
		return req, &schemas.LLMPluginShortCircuit{Error: failure}, nil
	}
	if req.RequestType == schemas.ChatCompletionRequest || req.RequestType == schemas.ResponsesRequest {
		var chat schemas.BifrostChatResponse
		_ = json.Unmarshal([]byte(`{"id":"c1","choices":[{"message":{"role":"assistant","content":"yes"}}]}`), &chat)
		return req, &schemas.LLMPluginShortCircuit{Response: &schemas.BifrostResponse{
			ChatResponse:      &chat,
			ResponsesResponse: &schemas.BifrostResponsesResponse{ID: new("r1"), Status: new("completed")},
		}}, nil
	}
	requestType := req.RequestType
	chunks := make(chan *schemas.BifrostStreamChunk)
	go func() {
		defer close(chunks)
		defer close(p.drained)
		first := &schemas.BifrostStreamChunk{}
		if requestType == schemas.ChatCompletionStreamRequest {
			first.BifrostChatResponse = &schemas.BifrostChatResponse{ID: "c1", Object: "chat.completion.chunk"}
		} else {
			first.BifrostResponsesStreamResponse = &schemas.BifrostResponsesStreamResponse{Type: schemas.ResponsesStreamResponseTypeCreated, Response: &schemas.BifrostResponsesResponse{ID: new("r1")}}
		}
		select {
		case chunks <- first:
		case <-ctx.Done():
			return
		}
		select {
		case <-p.next:
		case <-ctx.Done():
			return
		}
		if p.later {
			select {
			case chunks <- &schemas.BifrostStreamChunk{BifrostError: failure}:
			case <-ctx.Done():
			}
		}
	}()
	return req, &schemas.LLMPluginShortCircuit{Stream: chunks}, nil
}

func directProxy(t *testing.T, plugin *directFixture) (*llmProviderProxy, api.Context) {
	t.Helper()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, directCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{}))
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir(), LastPlugin: plugin}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	b := &directAccessBackend{storage: storage, informers: map[string]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(t.Context(), b)
	require.NoError(t, err)
	model := &v1.Model{Name: "m1-test", Namespace: system.DefaultNamespace, Spec: v1.ModelSpec{Manifest: types.ModelManifest{Active: true, TargetModel: "openai/test", ModelProvider: system.LLMGatewayModelProvider, Usage: types.ModelUsageLLM}}}
	require.NoError(t, storage.Create(t.Context(), model))
	require.NoError(t, b.informers["Model"].GetIndexer().Add(model))
	policy := &v1.ModelAccessPolicy{Name: "profile", Namespace: system.DefaultNamespace, Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, Models: []types.ModelResource{{ID: model.Name}}}}}
	require.NoError(t, b.informers["ModelAccessPolicy"].GetIndexer().Add(policy))
	c := newUsageTestClient(t)
	return &llmProviderProxy{backend: &bifrostProviderBackend{engine: engine}, mapHelper: helper, modelProvider: &v1.ModelProvider{Name: system.LLMGatewayModelProvider, Namespace: system.DefaultNamespace}}, api.Context{Storage: storage, GatewayClient: c, User: &user.DefaultInfo{UID: "7"}}
}

func (w *notifyingWriter) FlushError() error {
	w.once.Do(func() { close(w.first) })
	if w.fail {
		return io.ErrClosedPipe
	}
	return nil
}

func TestDirectPublicInferenceStreaming(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		for _, failure := range []string{"none", "before", "later", "flush", "cancel"} {
			t.Run(endpoint+"/"+failure, func(t *testing.T) {
				p := &directFixture{next: make(chan struct{}), drained: make(chan struct{}), before: failure == "before", later: failure == "later"}
				proxy, apiReq := directProxy(t, p)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				apiReq.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/"+endpoint+"?compat=kept", strings.NewReader(`{"model":"m1-test","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":8,"stream":true}`)).WithContext(ctx)
				apiReq.SetPathValue("path", "v1/"+endpoint)
				apiReq.Request.Header.Set("Authorization", "Bearer user-secret")
				w := &notifyingWriter{ResponseRecorder: httptest.NewRecorder(), first: make(chan struct{}), fail: failure == "flush"}
				apiReq.ResponseWriter = w
				done := make(chan error, 1)
				go func() { done <- proxy.proxy(apiReq) }()
				if failure != "before" {
					select {
					case <-w.first:
					case err := <-done:
						t.Fatalf("handler ended before first flush: %v body %s", err, w.Body.String())
					case <-time.After(5 * time.Second):
						t.Fatal("first event was not incremental")
					}
					if failure != "flush" {
						select {
						case <-done:
							t.Fatal("handler completed before producer released")
						default:
						}
					}
					if failure == "cancel" {
						cancel()
					} else {
						close(p.next)
					}
				}
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("handler did not drain")
				}
				if failure != "before" {
					select {
					case <-p.drained:
					case <-time.After(time.Second):
						t.Fatal("producer leaked")
					}
				}
				require.NotContains(t, w.Body.String(), "extra_fields")
				require.NotContains(t, w.Body.String(), "Bifrost")
				if failure == "before" {
					require.Equal(t, 429, w.Code)
					require.Contains(t, w.Header().Get("Content-Type"), "application/json")
				} else {
					require.Equal(t, 200, w.Code)
				}
			})
		}
	}
}

func (b *failedAccessBackend) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	i, err := b.directAccessBackend.GetInformerForKind(ctx, gvk)
	return failingInformer{SharedIndexInformer: i}, err
}

func (i failingInformer) GetIndexer() cache.Indexer {
	return failedIndexer{Indexer: i.SharedIndexInformer.GetIndexer()}
}

func (failedIndexer) ByIndex(string, string) ([]any, error) {
	return nil, errors.New("fixture index failure")
}

func TestDirectUnaryPreparationErrorDialect(t *testing.T) {
	for _, anthropic := range []bool{false, true} {
		t.Run(fmt.Sprint(anthropic), func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			b := &failedAccessBackend{directAccessBackend{storage: storage, informers: map[string]cache.SharedIndexInformer{}}}
			helper, err := modelaccesspolicy.NewHelper(t.Context(), b)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.SetPathValue("path", "v1/models")
			if anthropic {
				req.Header.Set("Anthropic-Version", "2023-06-01")
			}
			w := httptest.NewRecorder()
			out := &llmResponseWriter{w: w, request: req, header: make(http.Header), cancel: func() {}, mapHelper: helper, user: &user.DefaultInfo{UID: "7"}}
			_, err = out.Write([]byte(`{"data":[]}`))
			require.NoError(t, err)
			out.finish()
			require.Equal(t, 502, w.Code)
			require.Contains(t, w.Body.String(), "failed to prepare gateway response")
			require.Equal(t, anthropic, strings.Contains(w.Body.String(), `"type":"error"`))
		})
	}
}

func TestDirectPublicInferenceUnary(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		t.Run(endpoint, func(t *testing.T) {
			fixture := &directFixture{}
			proxy, req := directProxy(t, fixture)
			req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/"+endpoint+"?compat=kept", strings.NewReader(`{"model":"m1-test","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":8}`))
			req.SetPathValue("path", "v1/"+endpoint)
			req.Request.Header.Set("Authorization", "Bearer caller-secret")
			req.Request.Header.Set("X-Bf-Key", "caller-key")
			req.Request.Header.Set("Anthropic-Beta", "compat-beta")
			req.Request.Header.Set("X-Forwarded-For", "192.0.2.1")
			req.Request.Header.Set("X-Forwarded-Host", "private.example")
			req.Request.Header.Set("X-Forwarded-Proto", "http")
			w := httptest.NewRecorder()
			req.ResponseWriter = w
			require.NoError(t, proxy.proxy(req))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, "kept", fixture.query["compat"])
			require.Empty(t, fixture.headers["authorization"])
			require.Empty(t, fixture.headers["x-bf-key"])
			require.Empty(t, fixture.headers["x-forwarded-host"])
			require.Empty(t, fixture.headers["x-forwarded-proto"])
			require.Equal(t, "192.0.2.1, 192.0.2.1", fixture.headers["x-forwarded-for"])
			require.Equal(t, "compat-beta", fixture.headers["anthropic-beta"])
			require.Contains(t, w.Header().Get("Content-Type"), "application/json")
			require.NotContains(t, w.Body.String(), "extra_fields")
			require.Equal(t, strconv.Itoa(w.Body.Len()), w.Header().Get("Content-Length"))
		})
	}
}

func (b *panickingBackend) decorateRequest(l *llmProviderProxy, req api.Context, model *v1.Model) (gatewayRequest, error) {
	tracked, err := b.bifrostProviderBackend.decorateRequest(l, req, model)
	if err != nil {
		return nil, err
	}
	attempts := tracked.(*gatewayAttempts)
	// Simulate provider work already admitted and priced before its handler panics.
	attempts.admitted = model.Spec.Manifest.TargetModel
	attempts.budgets[attempts.admitted] = []string{"panic-budget"}
	attempts.priced = &guardrails.Cost{
		InputTokens:  5,
		OutputTokens: 3,
		Input:        0.04,
		Output:       0.06,
	}
	unbind := attempts.unbind
	attempts.unbind = func() {
		b.released++
		unbind()
	}
	return attempts, nil
}
func (b *panickingBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.ctx = r.Context()
	if b.stream {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.WriteHeader(http.StatusOK)
	if b.stream {
		_, _ = w.Write([]byte("data: {}\n\n"))
	} else {
		_, _ = w.Write([]byte(`{"success":true}`))
	}
	panic("fixture panic")
}

func (w *panicStatusWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
	w.ResponseRecorder.WriteHeader(status)
}

func TestDirectHandlerPanicDoesNotFlushSuccess(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			proxy, req := directProxy(t, &directFixture{})
			b := &panickingBackend{
				bifrostProviderBackend: proxy.backend.(*bifrostProviderBackend),
				stream:                 stream,
			}
			proxy.backend = b
			c := newUsageTestClientWithAudit(t, true)
			req.GatewayClient = c
			w := &panicStatusWriter{ResponseRecorder: httptest.NewRecorder()}
			req.ResponseWriter = w
			req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/responses", strings.NewReader(`{"model":"m1-test","input":"hi"}`))
			req.SetPathValue("path", "v1/responses")
			require.PanicsWithValue(t, "fixture panic", func() {
				_ = proxy.proxy(req)
			})
			require.Equal(t, 1, b.released)
			require.ErrorIs(t, b.ctx.Err(), context.Canceled)
			wantStatus := http.StatusInternalServerError
			if stream {
				wantStatus = http.StatusOK
				require.Equal(t, []int{http.StatusOK}, w.statuses)
				require.Equal(t, "data: {}\n\n", w.Body.String())
			} else {
				require.Empty(t, w.statuses, "no buffered success may commit during unwinding")
				require.Empty(t, w.Body.String())
				require.Empty(t, w.Header().Get("Content-Type"))
			}

			// The sentinel shares the FIFO audit queue. Once it persists, all entries
			// from the completed panic path have persisted, including any duplicates.
			c.LogLLMAuditEntry(gatewaytypes.LLMAuditLog{
				ID:        "panic-test-barrier",
				UserID:    "barrier",
				CreatedAt: time.Now(),
			})
			require.EventuallyWithT(t, func(check *assert.CollectT) {
				_, total, err := c.GetLLMAuditLogs(t.Context(), client.LLMAuditLogOptions{UserID: []string{"barrier"}})
				require.NoError(check, err)
				require.EqualValues(check, 1, total)
			}, 2*time.Second, 5*time.Millisecond)
			logs, total, err := c.GetLLMAuditLogs(t.Context(), client.LLMAuditLogOptions{
				UserID:              []string{"7"},
				WithSensitiveFields: true,
			})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, logs, 1)
			require.Equal(t, gatewaytypes.LLMAuditOutcomeError, logs[0].Outcome)
			require.Equal(t, wantStatus, logs[0].ResponseStatus)
			require.Equal(t, "LLM gateway handler panicked", logs[0].Error)
			require.Equal(t, "openai/test", logs[0].TargetModel)
			require.Equal(t, 5, logs[0].InputTokens)
			require.Equal(t, 3, logs[0].OutputTokens)
			require.InDelta(t, 0.1, logs[0].Spend, 1e-9)

			start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
			usage, err := c.TokenUsageForUser(t.Context(), "7", start, end)
			require.NoError(t, err)
			require.Len(t, usage, 1)
			require.Equal(t, gatewaytypes.LLMAuditOutcomeError, usage[0].Outcome)
			require.Equal(t, "openai/test", usage[0].Model)
			require.Equal(t, 8, usage[0].Usage.TotalTokens)
			require.InDelta(t, 0.1, usage[0].Usage.TotalSpend, 1e-9)
			stats, err := c.TokenUsageStats(t.Context(), client.TokenUsageStatsOptions{
				Start: start,
				End:   end,
			})
			require.NoError(t, err)
			require.EqualValues(t, 1, stats.Totals.Requests, "include even duplicate rows with zero usage")
			require.Zero(t, stats.Totals.Successes)

			budget, err := c.LLMGatewayBudgetUsageReport(t.Context(), &gatewaytypes.LLMGatewayBudget{
				ID: "panic-budget",
				Limits: []gatewaytypes.LLMGatewayBudgetLimit{
					{
						Metric: gatewaytypes.LLMGatewayBudgetMetricRequests,
						Window: "1h",
					},
					{
						Metric: gatewaytypes.LLMGatewayBudgetMetricTokens,
						Window: "1h",
					},
					{
						Metric: gatewaytypes.LLMGatewayBudgetMetricSpend,
						Window: "1h",
					},
				},
			})
			require.NoError(t, err)
			require.Len(t, budget, 1)
			require.Equal(t, float64(1), budget[0].Used[0], "one budget debit")
			require.Equal(t, float64(8), budget[0].Used[1])
			require.InDelta(t, 0.1, budget[0].Used[2], 1e-9)
		})
	}
}

func (b *mountedDirectBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.output = w.(*llmResponseWriter)
	b.bifrostProviderBackend.ServeHTTP(w, r)
}

func (w *publicFlushFailureWriter) FlushError() error {
	w.flushes++
	return io.ErrClosedPipe
}

func (w *publicFlushFailureWriter) Flush() { _ = w.FlushError() }

func TestDirectMountedFlushFailure(t *testing.T) {
	for _, waiting := range []bool{false, true} {
		t.Run(fmt.Sprintf("waiting-producer=%t", waiting), func(t *testing.T) {
			p := &directFixture{next: make(chan struct{}), drained: make(chan struct{})}
			if !waiting {
				close(p.next)
			}
			proxy, req := directProxy(t, p)
			b := &mountedDirectBackend{bifrostProviderBackend: proxy.backend.(*bifrostProviderBackend)}
			proxy.backend = b
			c := newUsageTestClientWithAudit(t, true)
			logger, err := audit.New(t.Context(), audit.Options{AuditLogsMode: audit.ModeOff})
			require.NoError(t, err)
			limiter, err := ratelimiter.New(ratelimiter.Options{AuthenticatedRateLimit: 100, UnauthenticatedRateLimit: 100})
			require.NoError(t, err)
			// LLM scope authorizes the fixture; the anonymous marker skips unrelated activity writes.
			principal := &user.DefaultInfo{UID: "7", Groups: []string{types.GroupLLM, authz.UnauthenticatedGroup}}
			authentication := authn.NewAuthenticator(authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
				return &authenticator.Response{User: principal}, true, nil
			}))
			server := apiServer.NewServer(req.Storage, c, nil, system.DefaultNamespace, authentication,
				authz.NewAuthorizer(c, req.Storage, req.Storage, false, nil, nil, nil), nil, logger, limiter, "http://obot.test", nil, &license.Provider{})
			server.HandleFunc("POST /api/llm-proxy/v1/{path...}", func(req api.Context) error {
				req.SetPathValue("path", "v1/"+req.PathValue("path"))
				return proxy.proxy(req)
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", strings.NewReader(`{"model":"m1-test","messages":[{"role":"user","content":"hi"}],"stream":true}`)).WithContext(ctx)
			w := &publicFlushFailureWriter{ResponseRecorder: httptest.NewRecorder()}
			done := make(chan struct{})
			go func() {
				server.ServeHTTP(w, request)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// Cancel only for cleanup if a regression leaves the producer waiting.
				t.Error("flush failure did not cancel and drain the producer")
				cancel()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Fatal("handler did not stop after cleanup cancellation")
				}
			}
			select {
			case <-p.drained:
			case <-time.After(time.Second):
				t.Fatal("producer leaked")
			}
			require.Equal(t, 1, w.flushes, "do not retry a failed public flush")
			require.Equal(t, []int{http.StatusOK}, w.statuses)
			require.Contains(t, w.Body.String(), "data:")
			require.NotContains(t, w.Body.String(), "[DONE]", "no follow-up response after transport failure")
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			require.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
			require.NotNil(t, b.output)
			require.ErrorIs(t, b.output.err, io.ErrClosedPipe)
			require.ErrorIs(t, b.output.request.Context().Err(), context.Canceled)
			require.Zero(t, b.output.audit.observed, "failed flush must not yield a normalized delivered chunk")

			// Reuse the FIFO audit sentinel to check all persisted request entries.
			c.LogLLMAuditEntry(gatewaytypes.LLMAuditLog{ID: "flush-test-barrier", UserID: "barrier", CreatedAt: time.Now()})
			require.EventuallyWithT(t, func(check *assert.CollectT) {
				_, total, err := c.GetLLMAuditLogs(t.Context(), client.LLMAuditLogOptions{UserID: []string{"barrier"}})
				require.NoError(check, err)
				require.EqualValues(check, 1, total)
			}, 2*time.Second, 5*time.Millisecond)
			logs, total, err := c.GetLLMAuditLogs(t.Context(), client.LLMAuditLogOptions{UserID: []string{"7"}, WithSensitiveFields: true})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, logs, 1)
			require.Equal(t, gatewaytypes.LLMAuditOutcomeError, logs[0].Outcome)
			require.Equal(t, io.ErrClosedPipe.Error(), logs[0].Error)
			require.Equal(t, http.StatusOK, logs[0].ResponseStatus)
			stats, err := c.TokenUsageStats(t.Context(), client.TokenUsageStatsOptions{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)})
			require.NoError(t, err)
			require.EqualValues(t, 1, stats.Totals.Requests)
			require.Zero(t, stats.Totals.Successes)
		})
	}
}
