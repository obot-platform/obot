package registry

import (
	"testing"
)

func TestReverseDNSFromURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expected    string
		shouldError bool
	}{
		{
			name:     "standard domain",
			baseURL:  "https://obot.example.com",
			expected: "com.example.obot",
		},
		{
			name:     "subdomain",
			baseURL:  "https://app.obot.example.com",
			expected: "com.example.obot.app",
		},
		{
			name:     "localhost",
			baseURL:  "http://localhost:8080",
			expected: "local.localhost",
		},
		{
			name:     "IP address",
			baseURL:  "http://192.168.1.100",
			expected: "local.192-168-1-100",
		},
		{
			name:     "single label domain",
			baseURL:  "http://obot",
			expected: "obot",
		},
		{
			name:     "with port",
			baseURL:  "https://obot.ai:443",
			expected: "ai.obot",
		},
		{
			name:        "invalid URL",
			baseURL:     "not a url",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ReverseDNSFromURL(tt.baseURL)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("ReverseDNSFromURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatRegistryServerName(t *testing.T) {
	tests := []struct {
		name       string
		reverseDNS string
		serverName string
		expected   string
	}{
		{
			name:       "standard names",
			reverseDNS: "ai.obot",
			serverName: "filesystem",
			expected:   "ai.obot/filesystem",
		},
		{
			name:       "name with special chars",
			reverseDNS: "com.example",
			serverName: "My_Server-123",
			expected:   "com.example/my-server-123",
		},
		{
			name:       "name with spaces",
			reverseDNS: "io.github",
			serverName: "Weather API Server",
			expected:   "io.github/weather-api-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRegistryServerName(tt.reverseDNS, tt.serverName)
			if result != tt.expected {
				t.Errorf("FormatRegistryServerName() = %v, want %v", result, tt.expected)
			}
		})
	}
}
