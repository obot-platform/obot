package oauth

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOAuthConsentClientCredentialSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		client   v1.OAuthClient
		expected string
	}{
		{
			name: "client ID metadata document",
			client: v1.OAuthClient{
				ObjectMeta: metav1.ObjectMeta{Name: "https://client.example/oauth/client.json"},
			},
			expected: "client_id_metadata_document",
		},
		{
			name: "static client credentials",
			client: v1.OAuthClient{
				ObjectMeta: metav1.ObjectMeta{Name: "static-client"},
				Spec:       v1.OAuthClientSpec{Static: true},
			},
			expected: "static_client_credentials",
		},
		{
			name: "dynamic client",
			client: v1.OAuthClient{
				ObjectMeta: metav1.ObjectMeta{Name: "dynamic-client"},
			},
			expected: "dynamic_client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := oauthConsentClientCredentialSource(tt.client); got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestPendingComponentDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   v1.MCPServer
		expected string
	}{
		{
			name: "manifest name",
			server: v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{Name: "component-server-id"},
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{Name: "Component Server"},
				},
			},
			expected: "Component Server",
		},
		{
			name: "server ID when manifest name is empty",
			server: v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{Name: "component-server-id"},
			},
			expected: "component-server-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := pendingComponentDisplayName(tt.server); got != tt.expected {
				t.Fatalf("expected display name %q, got %q", tt.expected, got)
			}
		})
	}
}
