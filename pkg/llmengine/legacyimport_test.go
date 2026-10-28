package llmengine

import (
	"testing"

	"github.com/obot-platform/obot/pkg/system"
)

func TestGenericResponsesImportBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		legacy string
		want   string
	}{
		{
			name:   "drops the API version the legacy provider needed",
			legacy: "https://api.openai.com/v1",
			want:   "https://api.openai.com",
		},
		{
			name:   "with a trailing slash",
			legacy: "https://api.openai.com/v1/",
			want:   "https://api.openai.com",
		},
		{
			name:   "keeps a path before the version",
			legacy: "https://llm.example.com/openai/v1",
			want:   "https://llm.example.com/openai",
		},
		{
			name:   "keeps a URL without the version",
			legacy: "https://llm.example.com",
			want:   "https://llm.example.com",
		},
		{
			name:   "keeps a version that isn't v1",
			legacy: "https://llm.example.com/v1beta",
			want:   "https://llm.example.com/v1beta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _ := legacyImportPlans[system.GenericResponsesModelProvider].build(map[string]string{
				"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": tt.legacy,
				"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY":  "key",
			})
			network, _ := provider["network_config"].(map[string]any)
			if got := network["base_url"]; got != tt.want {
				t.Errorf("base_url = %v, want %s", got, tt.want)
			}
		})
	}
}
