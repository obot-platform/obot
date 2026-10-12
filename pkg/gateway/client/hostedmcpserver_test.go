package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fixedHostedMCPServerLimit struct {
	limit HostedMCPServerLimit
	err   error
}

func (f fixedHostedMCPServerLimit) HostedMCPServerLimit(context.Context) (HostedMCPServerLimit, error) {
	return f.limit, f.err
}

func hostedServerTestClient(t *testing.T, runtimes ...apitypes.Runtime) *Client {
	t.Helper()

	builder := kfake.NewClientBuilder().WithScheme(storagescheme.Scheme)
	for i, runtime := range runtimes {
		builder = builder.WithObjects(&v1.MCPServer{
			Name:      system.MCPServerPrefix + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				Manifest: apitypes.MCPServerManifest{
					Name:    string(runtime),
					Runtime: runtime,
				},
			},
		})
	}

	c := newTestClient(t)
	c.storageClient = builder.Build()
	return c
}

func TestIsHostedRuntime(t *testing.T) {
	tests := []struct {
		name    string
		runtime apitypes.Runtime
		want    bool
	}{
		{
			name:    "uvx",
			runtime: apitypes.RuntimeUVX,
			want:    true,
		},
		{
			name:    "npx",
			runtime: apitypes.RuntimeNPX,
			want:    true,
		},
		{
			name:    "containerized",
			runtime: apitypes.RuntimeContainerized,
			want:    true,
		},
		{
			name:    "remote runs outside Obot",
			runtime: apitypes.RuntimeRemote,
		},
		{
			name:    "vmcp runs nothing of its own",
			runtime: apitypes.RuntimeVMCP,
		},
		{
			name:    "composite runs nothing of its own",
			runtime: apitypes.RuntimeComposite,
		},
		{
			name:    "unset",
			runtime: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHostedRuntime(tt.runtime); got != tt.want {
				t.Fatalf("IsHostedRuntime(%q) = %t, want %t", tt.runtime, got, tt.want)
			}
		})
	}
}

func TestHostedMCPServerCount(t *testing.T) {
	c := hostedServerTestClient(t,
		apitypes.RuntimeUVX,
		apitypes.RuntimeNPX,
		apitypes.RuntimeContainerized,
		apitypes.RuntimeRemote,
		apitypes.RuntimeVMCP,
		apitypes.RuntimeComposite,
	)

	count, err := c.HostedMCPServerCount(t.Context())
	if err != nil {
		t.Fatalf("HostedMCPServerCount() error = %v, want nil", err)
	}
	if count != 3 {
		t.Fatalf("HostedMCPServerCount() = %d, want 3", count)
	}
}

func TestCreateHostedMCPServer(t *testing.T) {
	tests := []struct {
		name        string
		existing    []apitypes.Runtime
		runtime     apitypes.Runtime
		provider    HostedMCPServerLimitProvider
		wantCreated bool
		wantStatus  int
	}{
		{
			name:        "no provider leaves hosted servers unbounded",
			existing:    []apitypes.Runtime{apitypes.RuntimeUVX, apitypes.RuntimeUVX},
			runtime:     apitypes.RuntimeUVX,
			wantCreated: true,
		},
		{
			name:        "an unlimited entitlement always creates",
			existing:    []apitypes.Runtime{apitypes.RuntimeUVX, apitypes.RuntimeUVX},
			runtime:     apitypes.RuntimeUVX,
			provider:    fixedHostedMCPServerLimit{limit: HostedMCPServerLimit{Unlimited: true}},
			wantCreated: true,
		},
		{
			name:        "below the limit creates",
			existing:    []apitypes.Runtime{apitypes.RuntimeUVX},
			runtime:     apitypes.RuntimeNPX,
			provider:    fixedHostedMCPServerLimit{limit: HostedMCPServerLimit{Maximum: 3}},
			wantCreated: true,
		},
		{
			name:       "at the limit refuses",
			existing:   []apitypes.Runtime{apitypes.RuntimeUVX, apitypes.RuntimeNPX, apitypes.RuntimeContainerized},
			runtime:    apitypes.RuntimeUVX,
			provider:   fixedHostedMCPServerLimit{limit: HostedMCPServerLimit{Maximum: 3}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:        "a remote server is never refused",
			existing:    []apitypes.Runtime{apitypes.RuntimeUVX, apitypes.RuntimeNPX, apitypes.RuntimeContainerized},
			runtime:     apitypes.RuntimeRemote,
			provider:    fixedHostedMCPServerLimit{limit: HostedMCPServerLimit{Maximum: 3}},
			wantCreated: true,
		},
		{
			name:        "a vMCP server is never refused",
			existing:    []apitypes.Runtime{apitypes.RuntimeUVX, apitypes.RuntimeNPX, apitypes.RuntimeContainerized},
			runtime:     apitypes.RuntimeVMCP,
			provider:    fixedHostedMCPServerLimit{limit: HostedMCPServerLimit{Maximum: 3}},
			wantCreated: true,
		},
		{
			name:     "remote servers do not consume the limit",
			existing: []apitypes.Runtime{apitypes.RuntimeRemote, apitypes.RuntimeRemote, apitypes.RuntimeRemote, apitypes.RuntimeVMCP},
			runtime:  apitypes.RuntimeUVX,
			provider: fixedHostedMCPServerLimit{
				limit: HostedMCPServerLimit{Maximum: 1},
			},
			wantCreated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := hostedServerTestClient(t, tt.existing...)
			if tt.provider != nil {
				c.SetHostedMCPServerLimitProvider(tt.provider)
			}

			var created bool
			err := c.CreateHostedMCPServer(t.Context(), tt.runtime, func(context.Context) error {
				created = true
				return nil
			})

			if created != tt.wantCreated {
				t.Fatalf("created = %t, want %t (error %v)", created, tt.wantCreated, err)
			}
			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("CreateHostedMCPServer() error = %v, want nil", err)
				}
				return
			}

			var httpErr *apitypes.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Fatalf("CreateHostedMCPServer() error = %v, want *types.ErrHTTP", err)
			}
			if httpErr.Code != tt.wantStatus {
				t.Fatalf("CreateHostedMCPServer() status = %d, want %d", httpErr.Code, tt.wantStatus)
			}
		})
	}
}

func TestCreateHostedMCPServerFailsWhenTheLimitCannotBeResolved(t *testing.T) {
	c := hostedServerTestClient(t)
	c.SetHostedMCPServerLimitProvider(fixedHostedMCPServerLimit{err: errors.New("license is unreadable")})

	var created bool
	if err := c.CreateHostedMCPServer(t.Context(), apitypes.RuntimeUVX, func(context.Context) error {
		created = true
		return nil
	}); err == nil {
		t.Fatal("CreateHostedMCPServer() error = nil, want a failure")
	}
	if created {
		t.Fatal("created a hosted MCP server without resolving the limit")
	}
}
