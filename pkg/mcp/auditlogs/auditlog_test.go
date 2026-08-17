package auditlogs

import (
	"testing"
	"time"
)

func TestProcessingTimeMilliseconds(t *testing.T) {
	for _, tt := range []struct {
		name    string
		elapsed time.Duration
		want    int64
	}{
		{name: "rounds up sub-millisecond duration", elapsed: 500 * time.Microsecond, want: 1},
		{name: "preserves whole milliseconds", elapsed: 10 * time.Millisecond, want: 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := processingTimeMilliseconds(tt.elapsed); got != tt.want {
				t.Fatalf("processingTimeMilliseconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRedactAPIKey(t *testing.T) {
	for _, tt := range []struct {
		name   string
		apiKey string
		want   string
	}{
		{name: "standard API key", apiKey: "ok1-123-456-secretABC", want: "ok1-123-456-"},
		{name: "longer API key", apiKey: "ok1-12345-67890-secret-with-dashes-and-more", want: "ok1-12345-67890-"},
		{name: "short key", apiKey: "ab", want: "a"},
		{name: "single character", apiKey: "a", want: ""},
		{name: "empty string", apiKey: "", want: ""},
		{name: "four characters", apiKey: "ok1-", want: "ok"},
		{name: "only two hyphens", apiKey: "ok1-abc-defghijklmnop", want: "ok1-abc-defg"},
		{name: "no hyphens", apiKey: "abcdefghijklmnopqrst", want: "abcdefghijkl"},
		{name: "exactly 12 characters", apiKey: "exactly12chr", want: "exactl"},
		{name: "short prefix with three hyphens", apiKey: "a-b-c-secretdata", want: "a-b-c-se"},
		{name: "many hyphens short segments", apiKey: "a-b-c-d-e-f-g-h", want: "a-b-c-d"},
		{name: "eleven characters", apiKey: "abcdefghijk", want: "abcde"},
		{name: "19 characters one hyphen", apiKey: "prefix-secretsecret", want: "prefix-se"},
		{name: "prefix exactly 12 chars", apiKey: "ok1-12345678-x-secret", want: "ok1-12345678-x-"},
		{name: "short key with three hyphens prefers prefix", apiKey: "ok1-1-1-a", want: "ok1-1-1-"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactAPIKey(tt.apiKey); got != tt.want {
				t.Errorf("RedactAPIKey(%q) = %q, want %q", tt.apiKey, got, tt.want)
			}
		})
	}
}
