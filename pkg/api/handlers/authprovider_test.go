package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type failAuthProviderCleanupCreateStorage struct {
	kclient.WithWatch
}

func (f *failAuthProviderCleanupCreateStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if _, ok := obj.(*v1.AuthProviderCleanup); ok {
		return errors.New("injected cleanup create failure")
	}
	return f.WithWatch.Create(ctx, obj, opts...)
}

func TestEnsureNoPendingAuthProviderCleanup(t *testing.T) {
	authProvider := v1.AuthProvider{
		Name: "entra-auth-provider",
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "entra/",
			},
		},
	}
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/configure", nil),
		Storage: newFakeStorage(t,
			&v1.AuthProviderCleanup{
				Name:      authProviderCleanupName("entra-auth-provider"),
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: "entra-auth-provider",
				},
			},
		),
	}

	err := ensureNoPendingAuthProviderCleanup(req, authProvider)
	require.ErrorContains(t, err, "still being deconfigured")
	require.NoError(t, ensureNoPendingAuthProviderCleanup(req, v1.AuthProvider{Name: "github-auth-provider"}))
}

func TestEnsureNoPendingAuthProviderCleanupBlocksPrefixReuse(t *testing.T) {
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/auth-providers/new-entra/configure", nil),
		Storage: newFakeStorage(t,
			&v1.AuthProviderCleanup{
				Name:      authProviderCleanupName("old-entra"),
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: "old-entra",
					GroupIDPrefix:    "entra/",
				},
			},
		),
	}
	authProvider := v1.AuthProvider{
		Name: "new-entra",
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "entra/",
			},
		},
	}

	err := ensureNoPendingAuthProviderCleanup(req, authProvider)
	require.ErrorContains(t, err, "still being deconfigured")
}

func TestEnsureAuthProviderCleanupIsDeterministicAndIdempotent(t *testing.T) {
	authProvider := v1.AuthProvider{
		Name:      "entra-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "entra/",
			},
		},
	}
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/deconfigure", nil),
		Storage: newFakeStorage(t),
	}

	first, err := ensureAuthProviderCleanup(req, authProvider)
	require.NoError(t, err)
	require.Equal(t, authProviderCleanupName(authProvider.Name), first.Name)
	require.False(t, first.Spec.Ready)

	second, err := ensureAuthProviderCleanup(req, authProvider)
	require.NoError(t, err)
	require.Equal(t, first.Name, second.Name)

	var cleanups v1.AuthProviderCleanupList
	require.NoError(t, req.List(&cleanups))
	require.Len(t, cleanups.Items, 1)

	require.NoError(t, markAuthProviderCleanupReady(req, second))
	var ready v1.AuthProviderCleanup
	require.NoError(t, req.Get(&ready, second.Name))
	require.True(t, ready.Spec.Ready)
	require.NoError(t, markAuthProviderCleanupReady(req, second))
}

func TestDeconfigurePersistsCleanupIntentBeforeSideEffects(t *testing.T) {
	authProvider := &v1.AuthProvider{
		Name:      "entra-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "entra/",
			},
		},
	}
	storage := &failAuthProviderCleanupCreateStorage{
		WithWatch: newFakeStorage(t, authProvider),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/deconfigure", nil)
	request.SetPathValue("id", authProvider.Name)
	req := api.Context{
		Request: request,
		Storage: storage,
	}

	err := (&AuthProviderHandler{}).Deconfigure(req)
	require.ErrorContains(t, err, "persist cleanup intent")
}
