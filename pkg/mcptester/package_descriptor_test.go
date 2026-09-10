package mcptester

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

func TestGitHubPackageDescriptors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "git HTTPS credentials and revision",
			input: "git+https://user:secret@GitHub.com/org/repo.git@v1.2.3?token=secret#subdirectory=private",
			want:  "https://github.com/org/repo",
		},
		{
			name:  "npm shorthand",
			input: "github:owner/repo#private-commit",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "git SSH",
			input: "git+ssh://git@github.com/org/repo.git",
			want:  "https://github.com/org/repo",
		},
		{
			name:  "canonical repository",
			input: "https://github.com/org/repo",
			want:  "https://github.com/org/repo",
		},
		{
			name:  "subpaths",
			input: "https://github.com/org/repo/tree/private/token",
			want:  "https://github.com/org/repo",
		},
		{
			name:  "origin remains accepted",
			input: "https://github.com",
			want:  "https://github.com",
		},
		{
			name:  "other VCS host",
			input: "git+https://user:secret@gitlab.com/group/private/repo.git?token=secret",
			want:  "https://gitlab.com",
		},
		{
			name:  "lookalike host",
			input: "https://github.com.example/org/private",
			want:  "https://github.com.example",
		},
		{
			name:  "nonstandard port",
			input: "https://github.com:8443/org/private",
			want:  "https://github.com:8443",
		},
		{
			name:  "encoded separator",
			input: "https://github.com/org/repo%2Fsecret",
			want:  "",
		},
		{
			name:  "encoded name",
			input: "https://github.com/org/%72epo",
			want:  "",
		},
		{
			name:  "invalid owner",
			input: "github:-owner/repo",
			want:  "",
		},
		{
			name:  "invalid repository",
			input: "github:owner/..",
			want:  "",
		},
		{
			name:  "missing repository",
			input: "github:owner",
			want:  "",
		},
		{
			name:  "unknown SSH host",
			input: "git+ssh://git@example.com/private/repo",
			want:  "",
		},
		{
			name:  "arguments",
			input: "github:owner/repo --token=secret",
			want:  "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, runtime := range []string{"npx", "uvx"} {
				manifest := types.MCPServerManifest{
					Runtime:   types.Runtime(runtime),
					NPXConfig: &types.NPXRuntimeConfig{Package: tt.input},
					UVXConfig: &types.UVXRuntimeConfig{Package: tt.input},
				}

				got, err := DeploymentDescriptor(manifest, mcp.ServerConfig{})
				if tt.want == "" {
					if err == nil {
						t.Fatalf("accepted invalid repository: %q", tt.input)
					}
				} else if err != nil || got.Value != runtime+" "+tt.want {
					t.Fatalf("got %#v, %v; want %q", got, err, runtime+" "+tt.want)
				}
			}
		})
	}
}

func TestRemoteGitHubDescriptorStripsPath(t *testing.T) {
	got, err := DeploymentDescriptor(types.MCPServerManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://github.com/org/private-repo?token=secret"},
	}, mcp.ServerConfig{})
	if err != nil || got.Value != "https://github.com" {
		t.Fatalf("got %#v, %v", got, err)
	}
}
