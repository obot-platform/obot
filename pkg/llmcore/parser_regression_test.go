//go:build e2e

package llmcore_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamParserDetectsTerminalErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		api      string
		partial  string
		terminal string
		message  string
		kind     string
	}{
		{
			name:     "Responses top level",
			api:      apiResponses,
			partial:  `{"type":"response.output_text.delta","delta":"partial"}`,
			terminal: `{"type":"error","message":"upstream failed","code":"api_error"}`,
			message:  "upstream failed",
			kind:     "api_error",
		},
		{
			name:     "Chat nested",
			api:      apiChat,
			terminal: `{"error":{"message":"chat failed","type":"api_error"}}`,
			message:  "chat failed",
			kind:     "api_error",
		},
		{
			name:     "Messages nested",
			api:      apiMessages,
			terminal: `{"type":"error","error":{"message":"messages failed","type":"api_error"}}`,
			message:  "messages failed",
			kind:     "api_error",
		},
		{
			name:     "Responses failed",
			api:      apiResponses,
			terminal: `{"type":"response.failed","response":{"error":{"message":"response failed","code":"api_error"}}}`,
			message:  "response failed",
			kind:     "api_error",
		},
		{
			name:     "successful Responses",
			api:      apiResponses,
			partial:  `{"type":"response.output_text.delta","delta":"partial"}`,
			terminal: `{"type":"response.completed","response":{}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLLM(tt.api, http.StatusOK, http.Header{"Content-Type": {"text/event-stream"}}, []byte("data: "+tt.partial+"\n\ndata: "+tt.terminal+"\n\n"))
			require.Equal(t, http.StatusOK, result.Status)
			require.Equal(t, tt.message, result.ErrorMsg)
			require.Equal(t, tt.kind, result.ErrorType)
			if tt.partial != "" {
				require.Equal(t, "partial", result.Text)
			}
			if tt.message == "" {
				result.requireOK(t)
			}
		})
	}
}
