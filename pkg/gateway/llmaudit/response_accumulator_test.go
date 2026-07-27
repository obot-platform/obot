package llmaudit

import "testing"

func TestResponseFormatForRequestPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want responseFormat
	}{
		{path: "/v1/messages", want: responseFormatAnthropicMessages},
		{path: "/anthropic/v1/messages", want: responseFormatAnthropicMessages},
		{path: "/api/llm-proxy/aws-bedrock/anthropic/v1/messages", want: responseFormatAnthropicMessages},
		{path: "/v1/responses", want: responseFormatOpenAIResponses},
		{path: "/openai/v1/responses", want: responseFormatOpenAIResponses},
		{path: "/api/llm-proxy/aws-bedrock/openai/v1/responses", want: responseFormatOpenAIResponses},
		{path: "/v1/models", want: responseFormatUnknown},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := responseFormatForRequestPath(tc.path); got != tc.want {
				t.Fatalf("responseFormatForRequestPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
