package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDownstreamOAuthClientName(t *testing.T) {
	const (
		dcrAuthRequestID   = "oar1dcr"
		cimdAuthRequestID  = "oar1cimd"
		dcrClientName      = "oac1cursor"
		qualifiedDCRClient = system.DefaultNamespace + ":" + dcrClientName
		cimdClientID       = "https://claude.ai/oauth/claude-code-client-metadata"
	)

	storage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(
			&v1.OAuthAuthRequest{
				Namespace: system.DefaultNamespace,
				Name:      dcrAuthRequestID,
				Spec: v1.OAuthAuthRequestSpec{
					ClientID: dcrClientName,
				},
			},
			&v1.OAuthAuthRequest{
				Namespace: system.DefaultNamespace,
				Name:      cimdAuthRequestID,
				Spec: v1.OAuthAuthRequestSpec{
					ClientID: cimdClientID,
				},
			},
		).
		Build()

	resolver := func(_ context.Context, _ kclient.Client, clientID string) (v1.OAuthClient, error) {
		switch clientID {
		case qualifiedDCRClient:
			return oauthClientWithName("Cursor"), nil
		case cimdClientID:
			return oauthClientWithName("Claude Code"), nil
		default:
			return v1.OAuthClient{}, fmt.Errorf("client ID does not exist: %s", clientID)
		}
	}

	factory := &MCPOAuthHandlerFactory{
		resolveOAuthClient: resolver,
	}
	req := api.Context{
		Request: httptest.NewRequest(http.MethodGet, "/oauth/callback", nil),
		Storage: storage,
	}

	t.Run("dynamic registration client", func(t *testing.T) {
		name, err := factory.downstreamOAuthClientName(req, dcrAuthRequestID)
		require.NoError(t, err)
		assert.Equal(t, "Cursor", name)
	})

	t.Run("client ID metadata document client", func(t *testing.T) {
		name, err := factory.downstreamOAuthClientName(req, cimdAuthRequestID)
		require.NoError(t, err)
		assert.Equal(t, "Claude Code", name)
	})

	t.Run("Obot request has no downstream client", func(t *testing.T) {
		name, err := factory.downstreamOAuthClientName(req, "")
		require.NoError(t, err)
		assert.Empty(t, name)
	})

	t.Run("missing auth request does not substitute another name", func(t *testing.T) {
		name, err := factory.downstreamOAuthClientName(req, "does-not-exist")
		require.Error(t, err)
		assert.Empty(t, name)
	})

	t.Run("missing resolver does not substitute another name", func(t *testing.T) {
		factory := &MCPOAuthHandlerFactory{}
		name, err := factory.downstreamOAuthClientName(req, dcrAuthRequestID)
		require.Error(t, err)
		assert.Empty(t, name)
	})
}

func oauthClientWithName(name string) v1.OAuthClient {
	return v1.OAuthClient{
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				ClientName: name,
			},
		},
	}
}
