package guardrails

import (
	"errors"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestProviderError(t *testing.T) {
	status := func(code int) *int { return &code }
	str := func(s string) *string { return &s }
	allow := func(b bool) *bool { return &b }

	tests := []struct {
		name          string
		err           *schemas.BifrostError
		wantFallbacks bool
	}{
		{
			name: "invalid request",
			err: &schemas.BifrostError{StatusCode: status(400), Error: &schemas.ErrorField{
				Type: str("invalid_request_error"), Code: str("invalid_image_format"), Message: "You uploaded an unsupported image.",
			}},
			wantFallbacks: false,
		},
		{
			name:          "server error",
			err:           &schemas.BifrostError{StatusCode: status(503), Error: &schemas.ErrorField{Type: str("server_error"), Message: "overloaded"}},
			wantFallbacks: true,
		},
		{
			name: "model not found",
			err: &schemas.BifrostError{StatusCode: status(404), Error: &schemas.ErrorField{
				Type: str("invalid_request_error"), Code: str("model_not_found"), Message: "The model does not exist",
			}},
			wantFallbacks: true,
		},
		{
			name: "rate limited",
			err: &schemas.BifrostError{StatusCode: status(429), Error: &schemas.ErrorField{
				Type: str("rate_limit_error"), Message: "Rate limit reached",
			}},
			wantFallbacks: true,
		},
		{
			// An access refusal says to go on to the next fallback, whatever its status.
			name:          "access refusal",
			err:           &schemas.BifrostError{StatusCode: status(400), AllowFallbacks: allow(true), Error: &schemas.ErrorField{Type: str("permission_denied"), Message: "not available"}},
			wantFallbacks: true,
		},
		{
			// No key serves the model: the gateway refused it before any provider saw it, and
			// a fallback on another provider or key may serve it.
			name: "no key supports the model",
			err: &schemas.BifrostError{
				Type:       str(schemas.NoKeySupportsModel),
				StatusCode: status(400),
				Error: &schemas.ErrorField{
					Type: str(schemas.NoKeySupportsModel), Message: "no keys found that support model: claude-haiku-4-5",
				},
				ExtraFields: schemas.BifrostErrorExtraFields{ErrorType: schemas.ErrorTypeCallerModelNotAvailable},
			},
			wantFallbacks: true,
		},
		{
			// The engine's own conversion can refuse a request one provider can't take; another
			// provider may.
			name:          "engine refusal",
			err:           &schemas.BifrostError{IsBifrostError: true, StatusCode: status(400), Error: &schemas.ErrorField{Type: str("invalid_request_error"), Message: "max_tokens too low for thinking"}},
			wantFallbacks: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerError(tt.err)
			if fallbacks := got.AllowFallbacks == nil || *got.AllowFallbacks; fallbacks != tt.wantFallbacks {
				t.Fatalf("fallbacks allowed = %v, want %v", fallbacks, tt.wantFallbacks)
			}
		})
	}

	if providerError(nil) != nil {
		t.Fatal("no error became one")
	}
}

func TestProviderErrorTimeout(t *testing.T) {
	status, errType := 504, schemas.RequestTimedOut
	err := &schemas.BifrostError{
		IsBifrostError: true,
		StatusCode:     &status,
		Error: &schemas.ErrorField{
			Type:    &errType,
			Message: schemas.ErrProviderRequestTimedOut,
			Error:   errors.New("fasthttp: timeout"),
		},
	}
	err.ExtraFields.RoutingInfo.Provider = "openai"
	got := providerError(err)
	if got.Error.Error != nil {
		t.Fatalf("the engine's internal error %v is still reported", got.Error.Error)
	}
	msg := got.Error.Message
	if !strings.Contains(msg, "timed out") || !strings.Contains(msg, `"openai"`) || strings.Contains(msg, "network_config") || strings.Contains(msg, "default is 300") {
		t.Fatalf("message %q", msg)
	}
}

func TestMarkToolUseStop(t *testing.T) {
	functionCall, message := schemas.ResponsesMessageTypeFunctionCall, schemas.ResponsesMessageTypeMessage
	completed := func(output []schemas.ResponsesMessage, stop *string, incomplete *schemas.ResponsesResponseIncompleteDetails) *schemas.BifrostResponse {
		return &schemas.BifrostResponse{ResponsesStreamResponse: &schemas.BifrostResponsesStreamResponse{
			Type:     schemas.ResponsesStreamResponseTypeCompleted,
			Response: &schemas.BifrostResponsesResponse{Output: output, StopReason: stop, IncompleteDetails: incomplete},
		}}
	}
	tests := []struct {
		name string
		resp *schemas.BifrostResponse
		want string
	}{
		{
			name: "tool call without a stop reason",
			resp: completed([]schemas.ResponsesMessage{{Type: &message}, {Type: &functionCall}}, nil, nil),
			want: "tool_calls",
		},
		{
			name: "text only",
			resp: completed([]schemas.ResponsesMessage{{Type: &message}}, nil, nil),
			want: "",
		},
		{
			name: "stop reason already set",
			resp: completed([]schemas.ResponsesMessage{{Type: &functionCall}}, new("stop"), nil),
			want: "stop",
		},
		{
			name: "truncated",
			resp: completed([]schemas.ResponsesMessage{{Type: &functionCall}}, nil, &schemas.ResponsesResponseIncompleteDetails{Reason: "max_output_tokens"}),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markToolUseStop(tt.resp)
			got := ""
			if r := tt.resp.ResponsesStreamResponse.Response.StopReason; r != nil {
				got = *r
			}
			if got != tt.want {
				t.Fatalf("stop reason = %q, want %q", got, tt.want)
			}
		})
	}
	markToolUseStop(nil)
	markToolUseStop(&schemas.BifrostResponse{})
}

func TestWithoutUnsupportedReasoning(t *testing.T) {
	chat := func(provider schemas.ModelProvider, model string) *schemas.BifrostRequest {
		return &schemas.BifrostRequest{ChatRequest: &schemas.BifrostChatRequest{
			Provider: provider,
			Model:    model,
			Params:   &schemas.ChatParameters{Reasoning: &schemas.ChatReasoning{Effort: new("medium")}},
		}}
	}
	tests := []struct {
		name     string
		req      *schemas.BifrostRequest
		wantDrop bool
	}{
		{
			name:     "OpenAI model that doesn't reason",
			req:      chat(schemas.OpenAI, "gpt-4o-mini"),
			wantDrop: true,
		},
		{
			name:     "Azure model that doesn't reason",
			req:      chat(schemas.Azure, "gpt-4.1-nano"),
			wantDrop: true,
		},
		{
			name: "OpenAI reasoning model",
			req:  chat(schemas.OpenAI, "gpt-5-mini"),
		},
		{
			name: "another provider",
			req:  chat(schemas.Ollama, "deepseek-r1"),
		},
		{
			name: "no reasoning settings",
			req:  &schemas.BifrostRequest{ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.OpenAI, Model: "gpt-4o-mini", Params: &schemas.ChatParameters{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalParams := tt.req.ChatRequest.Params
			got := withoutUnsupportedReasoning(tt.req)
			if dropped := got.ChatRequest.Params.Reasoning == nil && originalParams.Reasoning != nil; dropped != tt.wantDrop {
				t.Fatalf("dropped = %v, want %v", dropped, tt.wantDrop)
			}
			// The original request keeps its settings: fallback attempts are built from it.
			if tt.req.ChatRequest.Params != originalParams || (tt.wantDrop && originalParams.Reasoning == nil) {
				t.Fatal("the original request was changed")
			}
		})
	}
}
