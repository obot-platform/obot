package llmengine

import (
	"testing"
)

func TestScrubEngineName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "lowercase",
			in:   "bifrost request failed",
			want: "LLM gateway request failed",
		},
		{
			name: "capitalized",
			in:   "error from Bifrost: provider not found",
			want: "error from LLM gateway: provider not found",
		},
		{
			name: "part of a word is kept",
			in:   "is_bifrost_error",
			want: "is_bifrost_error",
		},
		{
			name: "no engine name",
			in:   "max_tokens is too large",
			want: "max_tokens is too large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScrubEngineName(tt.in); got != tt.want {
				t.Errorf("ScrubEngineName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
