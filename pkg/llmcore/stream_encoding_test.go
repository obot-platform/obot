package llmcore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/streaming"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

var (
	encodingAPIs = []struct {
		path string
		body string
	}{
		{
			path: "/v1/responses",
			body: `{"model":"openai/test","input":"hello","stream":%t}`,
		},
		{
			path: "/v1/messages",
			body: `{"model":"openai/test","messages":[{"role":"user","content":"hello"}],"max_tokens":8,"stream":%t}`,
		},
		{
			path: "/v1/chat/completions",
			body: `{"model":"openai/test","messages":[{"role":"user","content":"hello"}],"stream":%t}`,
		},
	}
)

// Encoding failures originate in provider/plugin output. Inject through a real
// core plugin so tests cover conversion and header commitment in each HTTP API.
type encodingFixture struct {
	later    bool
	release  <-chan struct{}
	canceled chan<- struct{}
}

type failedStreamTransport struct {
	header    http.Header
	writes    int
	flushes   int
	failWrite bool
}

func (*encodingFixture) GetName() string { return "encoding-fixture" }

func (*encodingFixture) Cleanup() error { return nil }

func (*encodingFixture) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (*encodingFixture) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func (p *encodingFixture) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if !bifrost.IsStreamRequestType(req.RequestType) {
		return req, &schemas.LLMPluginShortCircuit{Response: &schemas.BifrostResponse{
			ResponsesResponse: &schemas.BifrostResponsesResponse{
				Temperature:      new(math.NaN()),
				SafeguardResults: json.RawMessage(`{`),
			},
			ChatResponse: &schemas.BifrostChatResponse{PromptFilterResults: json.RawMessage(`{`)},
		}}, nil
	}
	badIndex := 0
	if p.later {
		badIndex = 1
	}
	chunks := make(chan *schemas.BifrostStreamChunk, badIndex+1)
	for i := 0; i <= badIndex; i++ {
		chunk := &schemas.BifrostStreamChunk{}
		if req.RequestType == schemas.ChatCompletionStreamRequest {
			chunk.BifrostChatResponse = &schemas.BifrostChatResponse{Object: "chat.completion.chunk"}
			if i == badIndex {
				chunk.PromptFilterResults = json.RawMessage(`{`)
			}
		} else {
			chunk.BifrostResponsesStreamResponse = &schemas.BifrostResponsesStreamResponse{Type: schemas.ResponsesStreamResponseTypeCreated}
			if i == badIndex {
				chunk.BifrostResponsesStreamResponse.LogProbs = []schemas.ResponsesOutputMessageContentTextLogProb{{LogProb: math.NaN()}}
				chunk.SafeguardResults = json.RawMessage(`{`)
			}
		}
		chunks <- chunk
	}
	if p.release == nil {
		close(chunks)
	} else {
		// Hold producer cleanup after cancellation to expose synchronous HTTP drains.
		go func() {
			<-ctx.Done()
			close(p.canceled)
			<-p.release
			close(chunks)
		}()
	}
	return req, &schemas.LLMPluginShortCircuit{Stream: chunks}, nil
}

func newEncodingEngine(t *testing.T, later bool) *llmcore.Engine {
	t.Helper()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{}))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{&encodingFixture{later: later}}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	return engine
}

func TestStreamCancellationWaitsForProducerCleanup(t *testing.T) {
	for _, api := range encodingAPIs {
		t.Run(api.path, func(t *testing.T) {
			release, canceled := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			store := newTestStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{}))
			plugin := &encodingFixture{release: release, canceled: canceled}
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{plugin}})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
			response := httptest.NewRecorder()
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(fmt.Sprintf(api.body, true))))
			}()
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not cancel the failed stream")
			}
			select {
			case <-returned:
				t.Fatal("handler returned before producer cleanup")
			case <-time.After(50 * time.Millisecond):
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case <-returned:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not return after producer cleanup")
			}
			require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
		})
	}
}

func TestHTTPEncodingFailures(t *testing.T) {
	for _, api := range encodingAPIs {
		t.Run(api.path, func(t *testing.T) {
			for _, phase := range []string{"unary", "first event", "later event"} {
				t.Run(phase, func(t *testing.T) {
					later := phase == "later event"
					engine := newEncodingEngine(t, later)
					response := httptest.NewRecorder()
					observed := 0
					ctx := llmcore.WithStreamObserver(t.Context(), func(_ *schemas.BifrostContext, _ *schemas.BifrostStreamChunk, _ *streaming.Accumulator) {
						observed++
					})
					engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(fmt.Sprintf(api.body, phase != "unary"))).WithContext(ctx))
					if later {
						require.Equal(t, 1, observed, "only the successfully encoded chunk reaches audit capture")
					} else {
						require.Zero(t, observed, "unary responses and unencodable chunks do not reach stream capture")
					}
					if later {
						require.Equal(t, http.StatusOK, response.Code)
						require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
						require.Contains(t, response.Body.String(), "event: error\n")
						require.NotContains(t, response.Body.String(), "[DONE]", "failed streams must not report successful completion")
					} else {
						require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
						require.Equal(t, "application/json", response.Header().Get("Content-Type"))
						require.NotContains(t, response.Body.String(), "data:")
					}
					if later && api.path == "/v1/responses" {
						var event schemas.BifrostResponsesStreamResponse
						require.NoError(t, json.Unmarshal(errorEventData(t, response.Body.String()), &event))
						require.Equal(t, schemas.ResponsesStreamResponseTypeError, event.Type)
						require.Equal(t, new("api_error"), event.Code, "encoding failures use a stable fallback code")
						require.Equal(t, new("failed to encode gateway response"), event.Message)
						require.Equal(t, 1, event.SequenceNumber)
						require.Nil(t, event.Error)
					} else {
						require.Contains(t, response.Body.String(), `"type":"api_error"`)
					}
					if api.path == "/v1/messages" {
						require.Contains(t, response.Body.String(), `"type":"error"`, "Messages failures must use the Anthropic error envelope")
					}
				})
			}
		})
	}
}

func (w *failedStreamTransport) Header() http.Header { return w.header }

func (*failedStreamTransport) WriteHeader(int) {}

func (w *failedStreamTransport) Write(data []byte) (int, error) {
	w.writes++
	if w.failWrite {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func (w *failedStreamTransport) FlushError() error { w.flushes++; return io.ErrClosedPipe }

func TestStreamWriteFailureDoesNotAttemptErrorResponse(t *testing.T) {
	for _, api := range encodingAPIs {
		for _, failure := range []string{"write", "flush"} {
			t.Run(api.path+"/"+failure, func(t *testing.T) {
				engine := newEncodingEngine(t, true)
				w := &failedStreamTransport{header: make(http.Header), failWrite: failure == "write"}
				observed := 0
				ctx := llmcore.WithStreamObserver(t.Context(), func(_ *schemas.BifrostContext, _ *schemas.BifrostStreamChunk, _ *streaming.Accumulator) {
					observed++
				})
				engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(fmt.Sprintf(api.body, true))).WithContext(ctx))
				require.Zero(t, observed, "failed writes must not enter audit accumulation")
				require.Equal(t, 1, w.writes, "do not attempt SSE errors or further chunks after the transport fails")
				if failure == "write" {
					require.Zero(t, w.flushes, "do not flush a failed write")
				} else {
					require.Equal(t, 1, w.flushes)
				}
			})
		}
	}
}
