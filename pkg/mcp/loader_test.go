package mcp

import "testing"

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
