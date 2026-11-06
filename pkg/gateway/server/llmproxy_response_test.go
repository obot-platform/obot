package server

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type failingClientWriter struct {
	*httptest.ResponseRecorder
	writeError error
	flushError error
	short      bool
	writes     int
	flushes    int
}

// Fixture responses are delivered by the real writer, including EOF finalization.
func writeTestResponse(t *testing.T, resp *http.Response, req *http.Request) (*http.Response, error) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	out := &llmResponseWriter{w: recorder, request: req.WithContext(ctx), header: make(http.Header), cancel: cancel}
	maps.Copy(out.Header(), resp.Header)
	out.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(out, resp.Body); err != nil {
		return nil, err
	}
	out.finish()
	return recorder.Result(), out.err
}

func (w *failingClientWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.short {
		return len(p) - 1, nil
	}
	if w.writeError != nil {
		return 0, w.writeError
	}
	return w.ResponseRecorder.Write(p)
}
func (w *failingClientWriter) FlushError() error {
	w.flushes++
	return w.flushError
}

func TestDirectWriterFragmentedStream(t *testing.T) {
	for _, ending := range []string{"\n\n", "\r\n\r\n", ""} {
		t.Run(strconv.Quote(ending), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Access-Control-Allow-Origin", "client-origin")
			recorder.Header().Set("X-Ratelimit-Remaining", "9")
			out := &llmResponseWriter{w: recorder, request: httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx), header: http.Header{"Content-Type": {"text/event-stream"}, "X-Bifrost-Routing-Info-Key": {"secret"}, "Access-Control-Allow-Origin": {"*"}}, cancel: cancel}
			out.WriteHeader(http.StatusOK)
			first := "event: response.completed\r\ndata: {\"response\":{\"id\":\"r1\",\"extra_"
			_, err := out.Write([]byte(first))
			require.NoError(t, err)
			require.NoError(t, http.NewResponseController(out).Flush())
			require.NotContains(t, recorder.Body.String(), "data:")
			_, err = out.Write([]byte("fields\":{\"key\":\"secret\"}}}" + ending))
			require.NoError(t, err)
			out.finish()
			require.Equal(t, "event: response.completed\r\ndata: {\"response\":{\"id\":\"r1\"}}"+ending, recorder.Body.String())
			require.Equal(t, "client-origin", recorder.Header().Get("Access-Control-Allow-Origin"))
			require.Equal(t, "9", recorder.Header().Get("X-Ratelimit-Remaining"))
			require.Empty(t, recorder.Header().Get("X-Bifrost-Routing-Info-Key"))
		})
	}
}

func TestDirectWriterFailureFinalizesOnce(t *testing.T) {
	for _, failure := range []string{"write", "short write", "flush", "cancel", "stream error"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c := newUsageTestClient(t)
			u := &user.DefaultInfo{UID: "7"}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
			audit := newLLMAuditRecorder(request, u, 1024)
			clientWriter := &failingClientWriter{ResponseRecorder: httptest.NewRecorder()}
			switch failure {
			case "write":
				clientWriter.writeError = io.ErrClosedPipe
			case "short write":
				clientWriter.short = true
			case "flush":
				clientWriter.flushError = io.ErrClosedPipe
			}
			attempts := &gatewayAttempts{admitted: "openai/test", budgets: map[string][]string{"openai/test": {"b1"}}}
			out := &llmResponseWriter{w: clientWriter, request: request, header: http.Header{"Content-Type": {"text/event-stream"}}, cancel: cancel,
				model: "openai/test", modelProvider: system.LLMGatewayModelProvider, user: u, client: c, audit: audit, usage: newLLMUsageRecorder(u, c, "openai/test", nil), gateway: attempts,
				tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &chatCompletionsTokenUsageTracker{}},
			}
			data := "data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n"
			if failure == "stream error" {
				data += "event: error\ndata: {\"type\":\"error\",\"message\":\"failed\"}\n\n"
			}
			_, writeErr := out.Write([]byte(data))
			flushErr := out.FlushError()
			if failure == "cancel" {
				cancel()
			}
			if failure == "write" || failure == "short write" {
				require.Error(t, writeErr)
			}
			if failure == "flush" {
				require.Error(t, flushErr)
			}
			out.finish()
			out.finish()
			require.Error(t, out.err)
			if failure != "stream error" {
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			}
			require.NotEqual(t, gatewaytypes.LLMAuditOutcomeSuccess, audit.log.Outcome)
			stats, err := c.TokenUsageStats(t.Context(), client.TokenUsageStatsOptions{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)})
			require.NoError(t, err)
			require.EqualValues(t, 1, stats.Totals.Requests)
			require.Zero(t, stats.Totals.Successes)
			report, err := c.LLMGatewayBudgetUsageReport(t.Context(), &gatewaytypes.LLMGatewayBudget{ID: "b1", Limits: []gatewaytypes.LLMGatewayBudgetLimit{{Metric: gatewaytypes.LLMGatewayBudgetMetricRequests, Window: "1h"}}})
			require.NoError(t, err)
			require.Equal(t, float64(1), report[0].Used[0], "one budget debit, even after repeated finalization")
		})
	}
}

func TestDirectWriterDialectUsage(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		dialect llmtypes.Dialect
		unary   string
		stream  string
	}{
		{
			name:    "messages",
			path:    "/anthropic/v1/messages",
			dialect: llmtypes.DialectAnthropicMessages,
			unary:   "{\n\"usage\":{\"input_tokens\":5,\"output_tokens\":3}}",
			stream:  "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n",
		},
		{
			name:    "responses",
			path:    "/v1/responses",
			dialect: llmtypes.DialectOpenAIResponses,
			unary:   "{\n\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}",
			stream:  "data: {\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}}\n\n",
		},
		{
			name:    "chat",
			path:    "/v1/chat/completions",
			dialect: llmtypes.DialectOpenAIChatCompletions,
			unary:   "{\n\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}",
			stream:  "data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\ndata: [DONE]\n\n",
		},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tt.name, streaming), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				c := newUsageTestClient(t)
				u := &user.DefaultInfo{UID: "7"}
				recorder := httptest.NewRecorder()
				audit := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, tt.path, nil), u, 1024)
				out := &llmResponseWriter{w: recorder, request: httptest.NewRequest(http.MethodPost, tt.path, nil).WithContext(ctx), header: make(http.Header), cancel: cancel, user: u, client: c, audit: audit, tokenUsageTracker: newTokenUsageTrackerForDialect(v1.Model{}, tt.dialect), modelProvider: system.LLMGatewayModelProvider}
				data := tt.unary
				if streaming {
					out.header.Set("Content-Type", "text/event-stream")
					data = tt.stream
				}
				for _, b := range []byte(data) {
					_, err := out.Write([]byte{b})
					require.NoError(t, err)
				}
				if streaming {
					require.NoError(t, out.FlushError())
				} else {
					require.Empty(t, recorder.Body.String())
				}
				out.finish()
				out.finish()
				require.Equal(t, data, recorder.Body.String())
				require.Equal(t, 5, audit.log.InputTokens)
				require.Equal(t, 3, audit.log.OutputTokens)
			})
		}
	}
}

func TestDirectCanceledPartialEventIsDiscarded(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := httptest.NewRecorder()
	out := &llmResponseWriter{w: w, request: httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx), header: http.Header{"Content-Type": {"text/event-stream"}}, cancel: cancel}
	_, err := out.Write([]byte(`data: {"extra_fields":{"key":"secret"`))
	require.NoError(t, err)
	require.NoError(t, out.FlushError())
	cancel()
	out.finish()
	require.Empty(t, w.Body.String())
	require.ErrorIs(t, out.err, context.Canceled)
}
