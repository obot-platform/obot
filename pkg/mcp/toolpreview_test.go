package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeToolPreviewBackend implements the backend methods tool previews use; any other call panics.
type fakeToolPreviewBackend struct {
	backend

	ensureErr     error
	ensured       []ServerConfig
	shutdowns     []string
	previews      []toolPreviewServer
	listErr       error
	shutdownError error
}

func (f *fakeToolPreviewBackend) ensureServerDeployment(_ context.Context, server ServerConfig) (ServerConfig, error) {
	f.ensured = append(f.ensured, server)
	if f.ensureErr != nil {
		// Like the real backends, a failed launch returns an empty config.
		return ServerConfig{}, f.ensureErr
	}
	return server, nil
}

func (f *fakeToolPreviewBackend) shutdownServer(_ context.Context, id string, _ bool) error {
	f.shutdowns = append(f.shutdowns, id)
	return f.shutdownError
}

func (f *fakeToolPreviewBackend) listToolPreviewServers(context.Context) ([]toolPreviewServer, error) {
	return f.previews, f.listErr
}

func TestGenerateToolPreviewsCleansUpWhenLaunchFails(t *testing.T) {
	launchErrors := []struct {
		name string
		err  error
	}{
		{
			name: "readiness timeout",
			err:  ErrHealthCheckTimeout,
		},
		{
			name: "crash loop",
			err:  ErrPodCrashLoopBackOff,
		},
		{
			name: "request canceled",
			err:  context.Canceled,
		},
	}

	for _, tt := range launchErrors {
		t.Run(tt.name, func(t *testing.T) {
			fb := &fakeToolPreviewBackend{ensureErr: tt.err}
			sm := &SessionManager{backend: fb}

			_, err := sm.GenerateToolPreviews(t.Context(), v1.MCPServer{}, ServerConfig{
				Runtime:       types.RuntimeContainerized,
				MCPServerName: "tool-preview-abc",
			})
			if !errors.Is(err, tt.err) {
				t.Fatalf("GenerateToolPreviews() error = %v, want %v", err, tt.err)
			}

			if len(fb.ensured) != 1 || !fb.ensured[0].ToolPreview {
				t.Fatalf("launched configs = %+v, want one marked as a tool preview", fb.ensured)
			}
			launched := fb.ensured[0].MCPServerName
			if !strings.HasPrefix(launched, "tool-preview-abc-") {
				t.Fatalf("launched server %q, want a name of its own derived from tool-preview-abc", launched)
			}
			if !slices.Equal(fb.shutdowns, []string{launched}) {
				t.Fatalf("shutdownServer() calls = %q, want the launched server %q removed", fb.shutdowns, launched)
			}
		})
	}
}

func TestGenerateToolPreviewsLaunchesAServerPerPreview(t *testing.T) {
	fb := &fakeToolPreviewBackend{ensureErr: ErrHealthCheckFailed}
	sm := &SessionManager{backend: fb}
	server := ServerConfig{
		Runtime:       types.RuntimeNPX,
		MCPServerName: "tool-preview-abc",
	}

	for range 2 {
		if _, err := sm.GenerateToolPreviews(t.Context(), v1.MCPServer{}, server); !errors.Is(err, ErrHealthCheckFailed) {
			t.Fatalf("GenerateToolPreviews() error = %v", err)
		}
	}

	if len(fb.ensured) != 2 || fb.ensured[0].MCPServerName == fb.ensured[1].MCPServerName {
		t.Fatalf("launched %+v, want two previews of one configuration on two servers", fb.ensured)
	}
}

func TestGenerateToolPreviewsKeepsRemoteServerName(t *testing.T) {
	fb := &fakeToolPreviewBackend{ensureErr: ErrHealthCheckFailed}
	sm := &SessionManager{backend: fb}

	_, err := sm.GenerateToolPreviews(t.Context(), v1.MCPServer{}, ServerConfig{
		Runtime:       types.RuntimeRemote,
		MCPServerName: "tool-preview-remote",
		URL:           "https://mcp.example.com/mcp",
		TunnelName:    "tunnel",
	})
	if !errors.Is(err, ErrHealthCheckFailed) {
		t.Fatalf("GenerateToolPreviews() error = %v", err)
	}

	// The handler checks OAuth and removes OAuth tokens under the name it chose, so a remote
	// preview, which deploys nothing, keeps it.
	if len(fb.ensured) != 1 || fb.ensured[0].MCPServerName != "tool-preview-remote" {
		t.Fatalf("launched %+v, want the remote server under the name it was given", fb.ensured)
	}
	if !slices.Equal(fb.shutdowns, []string{"tool-preview-remote"}) {
		t.Fatalf("shutdownServer() calls = %q, want tool-preview-remote", fb.shutdowns)
	}
}

func TestSweepToolPreviews(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	fb := &fakeToolPreviewBackend{
		previews: []toolPreviewServer{
			{
				Name:    "expired",
				Created: now.Add(-toolPreviewTTL),
			},
			{
				Name:    "long-expired",
				Created: now.Add(-24 * time.Hour),
			},
			{
				Name:    "recent",
				Created: now.Add(-toolPreviewTTL + time.Second),
			},
		},
	}
	sm := &SessionManager{backend: fb}

	sm.sweepToolPreviews(t.Context(), now)

	if want := []string{"expired", "long-expired"}; !slices.Equal(fb.shutdowns, want) {
		t.Fatalf("swept %q, want %q", fb.shutdowns, want)
	}
}

func TestSweepToolPreviewsContinuesPastFailures(t *testing.T) {
	now := time.Now()
	fb := &fakeToolPreviewBackend{
		shutdownError: errors.New("boom"),
		previews: []toolPreviewServer{
			{
				Name:    "a",
				Created: now.Add(-24 * time.Hour),
			},
			{
				Name:    "b",
				Created: now.Add(-24 * time.Hour),
			},
		},
	}
	sm := &SessionManager{backend: fb}

	sm.sweepToolPreviews(t.Context(), now)

	if want := []string{"a", "b"}; !slices.Equal(fb.shutdowns, want) {
		t.Fatalf("attempted to sweep %q, want %q", fb.shutdowns, want)
	}
}

func TestToolPreviewTTLOutlastsPreviews(t *testing.T) {
	if toolPreviewMaxDuration <= MaxMCPServerStartupTimeout {
		t.Fatalf("toolPreviewMaxDuration %s leaves no time to list tools after the longest startup %s", toolPreviewMaxDuration, MaxMCPServerStartupTimeout)
	}
	if toolPreviewTTL <= toolPreviewMaxDuration {
		t.Fatalf("toolPreviewTTL %s would sweep previews that may still be running (max %s)", toolPreviewTTL, toolPreviewMaxDuration)
	}
}

func TestK8sObjectsLabelToolPreviews(t *testing.T) {
	for _, toolPreview := range []bool{false, true} {
		k := newTestKubernetesBackend(t)
		server := testK8sServerConfig()
		server.Runtime = types.RuntimeNPX
		server.ToolPreview = toolPreview

		objs, err := k.k8sObjects(t.Context(), server)
		if err != nil {
			t.Fatalf("k8sObjects() error = %v", err)
		}

		for _, obj := range objs {
			if _, ok := obj.GetLabels()[ToolPreviewLabel]; ok != toolPreview {
				t.Errorf("toolPreview=%t: %T %s has tool preview label = %t", toolPreview, obj, obj.GetName(), ok)
			}
			if dep, isDeployment := obj.(*appsv1.Deployment); isDeployment {
				if _, ok := dep.Spec.Template.Labels[ToolPreviewLabel]; ok != toolPreview {
					t.Errorf("toolPreview=%t: pod template has tool preview label = %t", toolPreview, ok)
				}
			}
		}
	}
}

func TestKubernetesListToolPreviewServers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	created := metav1.NewTime(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC))
	deployment := func(name, namespace string, labels map[string]string) kclient.Object {
		return &appsv1.Deployment{
			Name:              name,
			Namespace:         namespace,
			Labels:            labels,
			CreationTimestamp: created,
		}
	}
	previewLabels := map[string]string{ToolPreviewLabel: "true"}

	k := &kubernetesBackend{
		mcpNamespace: "obot-mcp",
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			deployment("tool-preview-a", "obot-mcp", previewLabels),
			deployment("ms1server", "obot-mcp", map[string]string{"app": "ms1server"}),
			deployment("tool-preview-elsewhere", "other", previewLabels),
		).Build(),
	}

	servers, err := k.listToolPreviewServers(t.Context())
	if err != nil {
		t.Fatalf("listToolPreviewServers() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "tool-preview-a" || !servers[0].Created.Equal(created.Time) {
		t.Fatalf("listToolPreviewServers() = %+v, want only tool-preview-a created at %s", servers, created.Time)
	}
}

func TestDockerListToolPreviewServers(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("all") != "1" {
			t.Errorf("container list all = %q, want stopped containers included", r.URL.Query().Get("all"))
		}
		if filters := r.URL.Query().Get("filters"); !strings.Contains(filters, `"label":{"`+ToolPreviewLabel+`":true}`) {
			t.Errorf("container list filters = %s, want the tool preview label", filters)
		}
		_ = json.NewEncoder(w).Encode([]container.Summary{
			{
				ID:      "1",
				Created: 1_794_571_200,
				Labels: map[string]string{
					"mcp.server.id":  "tool-preview-a",
					ToolPreviewLabel: "true",
				},
			},
			{
				ID:     "2",
				Labels: map[string]string{ToolPreviewLabel: "true"},
			},
		})
	}))
	t.Cleanup(api.Close)
	cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	servers, err := (&dockerBackend{client: cli}).listToolPreviewServers(t.Context())
	if err != nil {
		t.Fatalf("listToolPreviewServers() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "tool-preview-a" || !servers[0].Created.Equal(time.Unix(1_794_571_200, 0)) {
		t.Fatalf("listToolPreviewServers() = %+v, want only tool-preview-a", servers)
	}
}
