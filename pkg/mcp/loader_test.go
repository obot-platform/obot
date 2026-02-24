package mcp

import "testing"

func TestClientIDIgnoresDynamicFileData(t *testing.T) {
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

	if clientID(serverA) != clientID(serverB) {
		t.Fatalf("expected same client ID when only dynamic file data changes")
	}
}

func TestClientIDIncludesStaticFileData(t *testing.T) {
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

	if clientID(serverA) == clientID(serverB) {
		t.Fatalf("expected different client IDs when static file data changes")
	}
}

func TestClientIDIncludesFileEnvKeys(t *testing.T) {
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

	if clientID(serverA) == clientID(serverB) {
		t.Fatalf("expected different client IDs when file env key changes")
	}
}
