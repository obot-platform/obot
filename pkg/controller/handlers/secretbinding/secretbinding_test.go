package secretbinding

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	obotNamespace = "obot"
	allowedLabel  = "obot.obot.ai/allow-secret-binding"
)

func boundServer(name string, config ...types.MCPConfig) *v1.MCPServer {
	return &v1.MCPServer{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{Runtime: types.RuntimeNPX, Config: config},
		},
	}
}

func boundConfig(key, secretName string) types.MCPConfig {
	return types.MCPConfig{
		Usage:         types.Env,
		Key:           key,
		Required:      true,
		SecretBinding: &types.MCPSecretBinding{Name: secretName, Key: "value"},
	}
}

func allowedSecret(name, value string) *corev1.Secret {
	return &corev1.Secret{
		Name:      name,
		Namespace: obotNamespace,
		Labels:    map[string]string{allowedLabel: "true"},
		Data:      map[string][]byte{"value": []byte(value)},
	}
}

func newStorageClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(objects...).
		Build()
}

func newSecretClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func getServer(t *testing.T, client kclient.Client, name string) v1.MCPServer {
	t.Helper()
	var server v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &server))
	return server
}

func TestSyncMCPServerStatus(t *testing.T) {
	tests := []struct {
		name           string
		secrets        []kclient.Object
		noSecretClient bool
		previous       []string
		want           []string
	}{
		{
			name:    "records bindings whose secret is missing or not allowed",
			secrets: []kclient.Object{allowedSecret("present", "x"), &corev1.Secret{Name: "unlabeled", Namespace: obotNamespace, Data: map[string][]byte{"value": []byte("x")}}},
			want:    []string{"MISSING", "UNLABELED"},
		},
		{
			name:     "clears bindings that now resolve",
			secrets:  []kclient.Object{allowedSecret("present", "x"), allowedSecret("missing", "x"), allowedSecret("unlabeled", "x")},
			previous: []string{"MISSING", "UNLABELED"},
		},
		{
			name:           "records every binding without a secret client",
			noSecretClient: true,
			want:           []string{"MISSING", "PRESENT", "UNLABELED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := boundServer("server",
				boundConfig("PRESENT", "present"),
				boundConfig("MISSING", "missing"),
				boundConfig("UNLABELED", "unlabeled"),
				types.MCPConfig{Usage: types.Env, Key: "USER_VALUE", Required: true},
			)
			server.Status.UnresolvedSecretBindings = tt.previous
			storageClient := newStorageClient(t, server)

			var secretClient kclient.Client
			if !tt.noSecretClient {
				secretClient = newSecretClient(t, tt.secrets...)
			}
			handler := New(storageClient, secretClient, obotNamespace, allowedLabel)

			err := handler.SyncMCPServerStatus(router.Request{
				Ctx:    t.Context(),
				Client: storageClient,
				Object: server,
			}, nil)
			require.NoError(t, err)
			status := getServer(t, storageClient, "server").Status
			require.Equal(t, tt.want, status.UnresolvedSecretBindings)
			require.Equal(t, mcp.SecretBindingsCheckHash(server.Spec.Manifest.Config, allowedLabel), status.SecretBindingsCheckHash)
		})
	}
}

func TestSyncMCPServersForSecretUpdatesBoundServers(t *testing.T) {
	bound := boundServer("bound", boundConfig("TOKEN", "shared"))
	other := boundServer("other", boundConfig("TOKEN", "unrelated"))
	storageClient := newStorageClient(t, bound, other)

	// The shared Secret was deleted, so only the server bound to it changes.
	handler := New(storageClient, newSecretClient(t), obotNamespace, allowedLabel)
	err := handler.SyncMCPServersForSecret(router.Request{
		Ctx:       t.Context(),
		Namespace: obotNamespace,
		Name:      "shared",
	}, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"TOKEN"}, getServer(t, storageClient, "bound").Status.UnresolvedSecretBindings)
	require.Empty(t, getServer(t, storageClient, "other").Status.UnresolvedSecretBindings)
}
