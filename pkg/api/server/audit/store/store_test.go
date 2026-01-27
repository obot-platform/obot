package store

import (
	"strings"
	"testing"
)

func TestFilename(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		compress bool
		wantHost string
		wantExt  string
	}{
		{
			name:     "simple host without compression",
			host:     "example",
			compress: false,
			wantHost: "example",
			wantExt:  ".log",
		},
		{
			name:     "simple host with compression",
			host:     "example",
			compress: true,
			wantHost: "example",
			wantExt:  ".log.gz",
		},
		{
			name:     "host with dots without compression",
			host:     "api.example.com",
			compress: false,
			wantHost: "api_example_com",
			wantExt:  ".log",
		},
		{
			name:     "host with dots with compression",
			host:     "api.example.com",
			compress: true,
			wantHost: "api_example_com",
			wantExt:  ".log.gz",
		},
		{
			name:     "host with multiple dots",
			host:     "sub.api.example.com",
			compress: false,
			wantHost: "sub_api_example_com",
			wantExt:  ".log",
		},
		{
			name:     "localhost",
			host:     "localhost",
			compress: false,
			wantHost: "localhost",
			wantExt:  ".log",
		},
		{
			name:     "IP address with compression",
			host:     "192.168.1.1",
			compress: true,
			wantHost: "192_168_1_1",
			wantExt:  ".log.gz",
		},
		{
			name:     "empty host",
			host:     "",
			compress: false,
			wantHost: "",
			wantExt:  ".log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filename(tt.host, tt.compress)

			// Check that it starts with the expected host (with dots replaced)
			if !strings.HasPrefix(result, tt.wantHost) {
				t.Errorf("filename() result %q does not start with expected host %q", result, tt.wantHost)
			}

			// Check that it ends with the expected extension
			if !strings.HasSuffix(result, tt.wantExt) {
				t.Errorf("filename() result %q does not end with expected extension %q", result, tt.wantExt)
			}

			// Check that it contains a timestamp separator (hyphen after host)
			if tt.host != "" && !strings.Contains(result, "-") {
				t.Errorf("filename() result %q does not contain timestamp separator", result)
			}

			// Check overall format: host-timestamp.log[.gz]
			parts := strings.Split(result, "-")
			if tt.host != "" && len(parts) < 2 {
				t.Errorf("filename() result %q does not have expected format 'host-timestamp.ext'", result)
			}
		})
	}
}

func TestFilenameFormat(t *testing.T) {
	// Test that filename generates valid RFC3339 timestamps
	host := "test.example.com"
	result := filename(host, false)

	// Expected format: test_example_com-2026-01-16T10:30:00Z.log
	// Extract timestamp part (between first hyphen and extension)
	withoutHost := strings.TrimPrefix(result, "test_example_com-")
	withoutExt := strings.TrimSuffix(withoutHost, ".log")

	// Check that timestamp part looks like RFC3339 (contains T and colons)
	if !strings.Contains(withoutExt, "T") {
		t.Errorf("timestamp in filename %q does not appear to be RFC3339 format (missing 'T')", result)
	}

	if strings.Count(withoutExt, ":") < 2 {
		t.Errorf("timestamp in filename %q does not appear to be RFC3339 format (insufficient colons)", result)
	}
}
