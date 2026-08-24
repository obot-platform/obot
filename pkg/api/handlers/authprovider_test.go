package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestEnsureNoPendingAuthProviderCleanup(t *testing.T) {
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/configure", nil),
		Storage: newFakeStorage(t,
			&v1.AuthProviderCleanup{
				Name:      "entra-cleanup",
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: "entra-auth-provider",
				},
			},
			&v1.AuthProviderCleanup{
				Name:      "okta-cleanup",
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: "okta-auth-provider",
				},
			},
		),
	}

	err := ensureNoPendingAuthProviderCleanup(req, "entra-auth-provider")
	require.ErrorContains(t, err, "still being deconfigured")
	require.NoError(t, ensureNoPendingAuthProviderCleanup(req, "github-auth-provider"))
}
