package client

import (
	"testing"
)

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{
			name:  "short text is kept",
			input: "displayName is required",
			limit: 100,
			want:  "displayName is required",
		},
		{
			name:  "long text is cut at the limit",
			input: "abcdef",
			limit: 4,
			want:  "abcd",
		},
		{
			name:  "a character that the limit would split is left out",
			input: "ab" + "é" + "cd",
			limit: 3,
			want:  "ab",
		},
		{
			name:  "a character that ends at the limit is kept",
			input: "ab" + "é" + "cd",
			limit: 4,
			want:  "abé",
		},
		{
			name:  "invalid UTF-8 is replaced",
			input: "bad \xff byte",
			limit: 100,
			want:  "bad � byte",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateUTF8(tt.input, tt.limit); got != tt.want {
				t.Fatalf("truncateUTF8(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}
