package llmcore_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

type idleStreamFixture struct {
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
	done     chan struct{}
}

type streamActivityWriter struct {
	*httptest.ResponseRecorder
	activity atomic.Int64
}

func (*idleStreamFixture) GetName() string { return "idle-stream-fixture" }
func (*idleStreamFixture) Cleanup() error  { return nil }
func (*idleStreamFixture) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}
func (*idleStreamFixture) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func (p *idleStreamFixture) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	chunks := make(chan *schemas.BifrostStreamChunk)
	go func() {
		defer close(p.done)
		defer close(chunks)
		close(p.entered)
		<-ctx.Done()
		close(p.canceled)
		<-p.release
		// Late data and errors must be drained without touching HTTP state or observers.
		chunks <- &schemas.BifrostStreamChunk{
			BifrostResponsesStreamResponse: &schemas.BifrostResponsesStreamResponse{Type: schemas.ResponsesStreamResponseTypeCreated},
			BifrostChatResponse:            &schemas.BifrostChatResponse{Object: "chat.completion.chunk"},
		}
		chunks <- &schemas.BifrostStreamChunk{BifrostError: &schemas.BifrostError{Error: &schemas.ErrorField{Message: "late error"}}}
		chunks <- nil
	}()
	return req, &schemas.LLMPluginShortCircuit{Stream: chunks}, nil
}

func (w *streamActivityWriter) Header() http.Header {
	w.activity.Add(1)
	return w.ResponseRecorder.Header()
}

func (w *streamActivityWriter) WriteHeader(status int) {
	w.activity.Add(1)
	w.ResponseRecorder.WriteHeader(status)
}

func (w *streamActivityWriter) Write(data []byte) (int, error) {
	w.activity.Add(1)
	return w.ResponseRecorder.Write(data)
}

func (w *streamActivityWriter) Flush() {
	w.activity.Add(1)
	w.ResponseRecorder.Flush()
}

func TestIdleStreamCancellationDrainsBeforeReturning(t *testing.T) {
	for _, api := range encodingAPIs {
		t.Run(api.path, func(t *testing.T) {
			plugin := &idleStreamFixture{
				entered:  make(chan struct{}),
				canceled: make(chan struct{}),
				release:  make(chan struct{}),
				done:     make(chan struct{}),
			}
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(plugin.release) })
			store := newTestStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{}))
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{plugin}})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
			var observations atomic.Int64
			ctx, cancel := context.WithCancel(llmcore.WithResponseObserver(t.Context(), func(schemas.BifrostResponseExtraFields) {
				observations.Add(1)
			}))
			defer cancel()
			writer := &streamActivityWriter{ResponseRecorder: httptest.NewRecorder()}
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				engine.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(fmt.Sprintf(api.body, true))).WithContext(ctx))
			}()
			select {
			case <-plugin.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("stream producer did not start")
			}
			cancel()
			select {
			case <-plugin.canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("producer did not observe request cancellation")
			}
			select {
			case <-returned:
				t.Fatal("canceled handler returned before producer closure")
			case <-time.After(50 * time.Millisecond):
			}
			releaseOnce.Do(func() { close(plugin.release) })
			select {
			case <-plugin.done:
			case <-time.After(3 * time.Second):
				t.Fatal("drain did not release the late producer")
			}
			select {
			case <-returned:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not return after producer closure")
			}
			require.Zero(t, writer.activity.Load(), "drain must not access the response writer")
			require.Zero(t, observations.Load(), "drain must not notify the response observer")
		})
	}
}
