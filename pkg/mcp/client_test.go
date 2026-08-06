package mcp

import (
	"strings"
	"testing"
)

func TestServerIDIgnoresDynamicFileData(t *testing.T) {
	serverA := ServerConfig{
		Runtime:       "containerized",
		MCPServerName: "test-server",
		Files: []File{{
			EnvKey:  "TLS_CERT",
			Data:    "cert-a",
			Dynamic: true,
		}},
	}

	serverB := serverA
	serverB.Files = []File{{
		EnvKey:  "TLS_CERT",
		Data:    "cert-b",
		Dynamic: true,
	}}

	if serverID(serverA) != serverID(serverB) {
		t.Fatalf("expected same server ID when only dynamic file data changes")
	}
}

func TestServerIDIncludesStaticFileData(t *testing.T) {
	serverA := ServerConfig{
		Runtime:       "containerized",
		MCPServerName: "test-server",
		Files: []File{{
			EnvKey:  "TLS_CERT",
			Data:    "cert-a",
			Dynamic: false,
		}},
	}

	serverB := serverA
	serverB.Files = []File{{
		EnvKey:  "TLS_CERT",
		Data:    "cert-b",
		Dynamic: false,
	}}

	if serverID(serverA) == serverID(serverB) {
		t.Fatalf("expected different client IDs when static file data changes")
	}
}

func TestServerIDIncludesFileEnvKeys(t *testing.T) {
	serverA := ServerConfig{
		Runtime:       "containerized",
		MCPServerName: "test-server",
		Files: []File{{
			EnvKey:  "TLS_CERT",
			Data:    "cert",
			Dynamic: true,
		}},
	}

	serverB := serverA
	serverB.Files = []File{{
		EnvKey:  "TLS_KEY",
		Data:    "cert",
		Dynamic: true,
	}}

	if serverID(serverA) == serverID(serverB) {
		t.Fatalf("expected different client IDs when file env key changes")
	}
}

func TestServerIDIncludesPassThroughHeaderNames(t *testing.T) {
	serverA := ServerConfig{
		Runtime:                "remote",
		MCPServerName:          "test-server",
		URL:                    "https://example.com/mcp",
		PassthroughHeaderNames: []string{"X-Test-Header"},
	}

	serverB := serverA
	serverB.PassthroughHeaderNames = []string{"X-Other-Header"}

	if serverID(serverA) == serverID(serverB) {
		t.Fatalf("expected different client IDs when passthrough header names change")
	}
}

func TestServerIDIgnoresPassThroughHeaderValues(t *testing.T) {
	serverA := ServerConfig{
		Runtime:                 "remote",
		MCPServerName:           "test-server",
		URL:                     "https://example.com/mcp",
		PassthroughHeaderNames:  []string{"X-Test-Header"},
		PassthroughHeaderValues: []string{"value-a"},
	}

	serverB := serverA
	serverB.PassthroughHeaderValues = []string{"value-b"}

	if serverID(serverA) != serverID(serverB) {
		t.Fatalf("expected same server ID when only passthrough header values change")
	}
}

func TestValidateRemoteMCPURL(t *testing.T) {
	tests := []struct {
		name              string
		rawURL            string
		allowLocalhostMCP bool
		allowPrivateIPMCP bool
		allowLinkLocalMCP bool
		wantErr           string
	}{
		{
			name:   "allows empty URL",
			rawURL: "",
		},
		{
			name:    "rejects loopback",
			rawURL:  "http://127.0.0.1:8080/mcp",
			wantErr: "localhost URL",
		},
		{
			name:              "allows loopback when configured",
			rawURL:            "http://127.0.0.1:8080/mcp",
			allowLocalhostMCP: true,
			allowPrivateIPMCP: false,
			allowLinkLocalMCP: false,
		},
		{
			name:    "rejects private IPv4",
			rawURL:  "http://10.0.0.1:8080/mcp",
			wantErr: "private IP address",
		},
		{
			name:    "rejects private IPv6",
			rawURL:  "http://[fc00::1]:8080/mcp",
			wantErr: "private IP address",
		},
		{
			name:              "allows private IP when configured",
			rawURL:            "http://10.0.0.1:8080/mcp",
			allowLocalhostMCP: false,
			allowPrivateIPMCP: true,
			allowLinkLocalMCP: false,
		},
		{
			name:              "rejects IPv4 link-local",
			rawURL:            "http://169.254.169.254/latest/meta-data",
			allowPrivateIPMCP: true,
			wantErr:           "link-local address",
		},
		{
			name:              "rejects arbitrary IPv4 link-local",
			rawURL:            "http://169.254.1.1/mcp",
			allowPrivateIPMCP: true,
			wantErr:           "link-local address",
		},
		{
			name:              "rejects IPv6 link-local",
			rawURL:            "http://[fe80::1]/mcp",
			allowPrivateIPMCP: true,
			wantErr:           "link-local address",
		},
		{
			name:              "allows link-local when configured",
			rawURL:            "http://169.254.169.254/latest/meta-data",
			allowLocalhostMCP: false,
			allowPrivateIPMCP: true,
			allowLinkLocalMCP: true,
		},
		{
			name:   "allows public IP",
			rawURL: "http://8.8.8.8:8080/mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemoteMCPURL(t.Context(), tt.rawURL, RemoteMCPURLValidationConfig{
				AllowLocalhostMCP: tt.allowLocalhostMCP,
				AllowPrivateIPMCP: tt.allowPrivateIPMCP,
				AllowLinkLocalMCP: tt.allowLinkLocalMCP,
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
