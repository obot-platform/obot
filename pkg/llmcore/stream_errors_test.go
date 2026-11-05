package llmcore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

// Reuse the no-op hooks and inject a provider error through the real core stream.
type streamErrorFixture struct {
	encodingFixture
	before bool
}

func (p *streamErrorFixture) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	chunks := make(chan *schemas.BifrostStreamChunk, 2)
	if !p.before {
		first := &schemas.BifrostStreamChunk{}
		if req.RequestType == schemas.ChatCompletionStreamRequest {
			first.BifrostChatResponse = &schemas.BifrostChatResponse{Object: "chat.completion.chunk"}
		} else {
			first.BifrostResponsesStreamResponse = &schemas.BifrostResponsesStreamResponse{
				Type:           schemas.ResponsesStreamResponseTypeCreated,
				SequenceNumber: 7,
			}
		}
		chunks <- first
	}
	chunks <- &schemas.BifrostStreamChunk{BifrostError: &schemas.BifrostError{
		StatusCode: schemas.Ptr(http.StatusTooManyRequests),
		Error: &schemas.ErrorField{
			Type:    new("rate_limit_error"),
			Code:    new("quota_exceeded"),
			Message: "Bifrost: quota exhausted",
			Param:   "input",
		},
	}}
	close(chunks)
	return req, &schemas.LLMPluginShortCircuit{Stream: chunks}, nil
}

func errorEventData(t *testing.T, body string) []byte {
	t.Helper()
	for frame := range strings.SplitSeq(body, "\n\n") {
		if !strings.HasPrefix(frame, "event: error\n") {
			continue
		}
		for line := range strings.SplitSeq(frame, "\n") {
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				return []byte(data)
			}
		}
	}
	t.Fatalf("missing SSE error event: %s", body)
	return nil
}

func TestProviderStreamErrorsUseAPISpecificEnvelopes(t *testing.T) {
	for _, api := range encodingAPIs {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/before=%t", api.path, before), func(t *testing.T) {
				store := newTestStore(t)
				require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{}))
				engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{&streamErrorFixture{before: before}}})
				require.NoError(t, err)
				defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(fmt.Sprintf(api.body, true))))
				expected := `{"error":{"type":"rate_limit_error","message":"gateway: quota exhausted"}}`
				if api.path == "/v1/messages" {
					expected = `{"type":"error","error":{"type":"rate_limit_error","message":"gateway: quota exhausted"}}`
				}
				if before {
					require.Equal(t, http.StatusTooManyRequests, response.Code)
					require.Equal(t, "application/json", response.Header().Get("Content-Type"))
					require.JSONEq(t, expected, response.Body.String(), "pre-stream errors keep their HTTP envelope")
					return
				}
				require.Equal(t, http.StatusOK, response.Code)
				require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
				require.NotContains(t, response.Body.String(), "[DONE]")
				require.NotContains(t, response.Body.String(), "extra_fields")
				data := errorEventData(t, response.Body.String())
				if api.path != "/v1/responses" {
					require.JSONEq(t, expected, string(data))
					return
				}
				// Decode using the Responses event type, not the HTTP error type.
				var event schemas.BifrostResponsesStreamResponse
				require.NoError(t, json.Unmarshal(data, &event))
				require.Equal(t, schemas.ResponsesStreamResponseTypeError, event.Type)
				require.Equal(t, new("quota_exceeded"), event.Code)
				require.Equal(t, new("gateway: quota exhausted"), event.Message)
				require.Equal(t, new("input"), event.Param)
				require.Equal(t, 8, event.SequenceNumber)
				require.Nil(t, event.Error, "Responses SSE errors use top-level fields")
			})
		}
	}
}
