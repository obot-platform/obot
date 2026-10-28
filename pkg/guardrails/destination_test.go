package guardrails

import (
	"strings"
	"testing"
)

func TestCheckDestinations(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		config  map[string]any
		wantErr string
	}{
		{
			name:   "public service",
			typ:    "lakera",
			config: map[string]any{"baseURL": "https://8.8.8.8"},
		},
		{
			name:   "self-hosted on a private network",
			typ:    "presidio",
			config: map[string]any{"analyzerURL": "http://10.1.2.3:3000"},
		},
		{
			name:   "private IPv6",
			typ:    "presidio",
			config: map[string]any{"analyzerURL": "http://[fd12:3456::1]:3000"},
		},
		{
			name:   "a host that doesn't resolve yet",
			typ:    "presidio",
			config: map[string]any{"analyzerURL": "http://presidio.invalid:3000"},
		},
		{
			name:    "cloud metadata",
			typ:     "lakera",
			config:  map[string]any{"baseURL": "http://169.254.169.254"},
			wantErr: "link-local",
		},
		{
			name:    "IPv4-mapped cloud metadata",
			typ:     "lakera",
			config:  map[string]any{"baseURL": "http://[::ffff:169.254.169.254]"},
			wantErr: "link-local",
		},
		{
			name:    "IPv6 cloud metadata",
			typ:     "patronus",
			config:  map[string]any{"baseURL": "http://[fd00:ec2::254]"},
			wantErr: "cloud metadata",
		},
		{
			name:    "loopback",
			typ:     "presidio",
			config:  map[string]any{"analyzerURL": "http://127.0.0.1:3000"},
			wantErr: "loopback",
		},
		{
			name:    "localhost",
			typ:     "azure-content-safety",
			config:  map[string]any{"endpoint": "http://localhost:8080"},
			wantErr: "loopback",
		},
		{
			name:    "IPv6 loopback",
			typ:     "crowdstrike-aidr",
			config:  map[string]any{"baseURL": "http://[::1]:8080"},
			wantErr: "loopback",
		},
		{
			name:    "unspecified",
			typ:     "lakera",
			config:  map[string]any{"baseURL": "http://0.0.0.0:8080"},
			wantErr: "unspecified",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := Provider(tt.typ); !ok {
				t.Fatalf("unknown guardrail type %q", tt.typ)
			}
			err := CheckDestinations(t.Context(), tt.typ, tt.config)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("expected an error with %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestEngineClientRefusesBlockedAddresses(t *testing.T) {
	client := New(nil, nil).deps.HTTPClient
	for _, u := range []string{"http://169.254.169.254/v2/guard", "http://[fd00:ec2::254]/", "http://127.0.0.1:1/"} {
		if _, err := client.Post(u, "application/json", strings.NewReader("{}")); err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Errorf("POST %s: expected it to be blocked, got %v", u, err)
		}
	}
}
