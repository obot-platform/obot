package requestinfo

import (
	"net/http"
	"testing"
)

func TestGetSourceIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xForwardedFor  string
		xRealIP        string
		expectedIP     string
		description    string
	}{
		{
			name:        "no headers - uses RemoteAddr",
			remoteAddr:  "192.168.1.100:54321",
			expectedIP:  "192.168.1.100:54321",
			description: "When no proxy headers are present, falls back to RemoteAddr",
		},
		{
			name:        "no headers - IPv6 RemoteAddr",
			remoteAddr:  "[2001:db8::1]:54321",
			expectedIP:  "[2001:db8::1]:54321",
			description: "IPv6 addresses are returned as-is from RemoteAddr",
		},
		{
			name:           "X-Forwarded-For single IP",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "203.0.113.45",
			expectedIP:     "203.0.113.45",
			description:    "Single IP in X-Forwarded-For is extracted",
		},
		{
			name:           "X-Forwarded-For multiple IPs - rightmost used",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "203.0.113.45, 198.51.100.23, 192.0.2.1",
			expectedIP:     "192.0.2.1",
			description:    "Rightmost IP (non-spoofable proxy-added) is used from chain",
		},
		{
			name:           "X-Forwarded-For with spaces",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "203.0.113.45,  198.51.100.23  , 192.0.2.1 ",
			expectedIP:     "192.0.2.1",
			description:    "Whitespace around IPs is trimmed",
		},
		{
			name:           "X-Forwarded-For IPv6",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "2001:db8::1, 2001:db8::2",
			expectedIP:     "2001:db8::2",
			description:    "IPv6 addresses work in X-Forwarded-For",
		},
		{
			name:           "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "203.0.113.45",
			xRealIP:        "198.51.100.23",
			expectedIP:     "203.0.113.45",
			description:    "X-Forwarded-For is checked before X-Real-IP",
		},
		{
			name:           "X-Real-IP used when X-Forwarded-For empty",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "",
			xRealIP:        "198.51.100.23",
			expectedIP:     "198.51.100.23",
			description:    "Falls back to X-Real-IP when X-Forwarded-For not present",
		},
		{
			name:           "X-Real-IP IPv6",
			remoteAddr:     "10.0.0.1:54321",
			xRealIP:        "2001:db8::1",
			expectedIP:     "2001:db8::1",
			description:    "IPv6 addresses work in X-Real-IP",
		},
		{
			name:        "empty RemoteAddr",
			remoteAddr:  "",
			expectedIP:  "",
			description: "Empty RemoteAddr returns empty string",
		},
		{
			name:           "X-Forwarded-For empty string (not absent)",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "",
			expectedIP:     "10.0.0.1:54321",
			description:    "Empty X-Forwarded-For header falls back to RemoteAddr",
		},
		{
			name:           "X-Forwarded-For with single IP and port",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "203.0.113.45:8080",
			expectedIP:     "203.0.113.45:8080",
			description:    "Port numbers in X-Forwarded-For are preserved",
		},
		{
			name:           "spoofed X-Forwarded-For with multiple entries",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  "attacker-ip, proxy1, proxy2",
			expectedIP:     "proxy2",
			description:    "Rightmost IP prevents client spoofing",
		},
		{
			name:           "X-Forwarded-For with only commas",
			remoteAddr:     "10.0.0.1:54321",
			xForwardedFor:  ",,",
			expectedIP:     "",
			description:    "Malformed X-Forwarded-For with only commas returns empty",
		},
		{
			name:           "X-Real-IP with whitespace",
			remoteAddr:     "10.0.0.1:54321",
			xRealIP:        "  198.51.100.23  ",
			expectedIP:     "  198.51.100.23  ",
			description:    "X-Real-IP is not trimmed (returned as-is)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			result := GetSourceIP(req)
			if result != tt.expectedIP {
				t.Errorf("GetSourceIP() = %q, want %q\nDescription: %s", result, tt.expectedIP, tt.description)
			}
		})
	}
}

func TestGetSourceIP_HeaderCaseSensitivity(t *testing.T) {
	// HTTP headers are case-insensitive, test that our implementation respects this
	tests := []struct {
		name       string
		headerName string
		headerVal  string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-For lowercase",
			headerName: "x-forwarded-for",
			headerVal:  "203.0.113.45",
			expectedIP: "203.0.113.45",
		},
		{
			name:       "X-Forwarded-For uppercase",
			headerName: "X-FORWARDED-FOR",
			headerVal:  "203.0.113.45",
			expectedIP: "203.0.113.45",
		},
		{
			name:       "X-Real-IP lowercase",
			headerName: "x-real-ip",
			headerVal:  "198.51.100.23",
			expectedIP: "198.51.100.23",
		},
		{
			name:       "X-Real-IP uppercase",
			headerName: "X-REAL-IP",
			headerVal:  "198.51.100.23",
			expectedIP: "198.51.100.23",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.RemoteAddr = "10.0.0.1:54321"
			req.Header.Set(tt.headerName, tt.headerVal)

			result := GetSourceIP(req)
			if result != tt.expectedIP {
				t.Errorf("GetSourceIP() with header %q = %q, want %q", tt.headerName, result, tt.expectedIP)
			}
		})
	}
}
