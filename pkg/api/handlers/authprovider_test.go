package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type failAuthProviderCleanupCreateStorage struct {
	kclient.WithWatch
}

type acknowledgeAuthProviderRevisionStorage struct {
	kclient.WithWatch
}

type failAuthProviderUpdateStorage struct {
	kclient.WithWatch
}

func (f *failAuthProviderCleanupCreateStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if _, ok := obj.(*v1.AuthProviderCleanup); ok {
		return errors.New("injected cleanup create failure")
	}
	return f.WithWatch.Create(ctx, obj, opts...)
}

func (a *acknowledgeAuthProviderRevisionStorage) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	if authProvider, ok := obj.(*v1.AuthProvider); ok {
		authProvider.Status.ObservedSyncRevision = authProvider.Annotations[v1.AuthProviderSyncAnnotation]
	}
	return a.WithWatch.Update(ctx, obj, opts...)
}

func (f *failAuthProviderUpdateStorage) Update(context.Context, kclient.Object, ...kclient.UpdateOption) error {
	return errors.New("injected auth provider update failure")
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

func TestUpdateAuthProviderSyncRevisionUsesUniqueUUIDs(t *testing.T) {
	authProvider := &v1.AuthProvider{
		Name:      "entra-auth-provider",
		Namespace: system.DefaultNamespace,
	}
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/configure", nil),
		Storage: newFakeStorage(t, authProvider),
	}

	first, err := updateAuthProviderSyncRevision(req, authProvider.Name)
	require.NoError(t, err)
	firstRevision := first.Annotations[v1.AuthProviderSyncAnnotation]
	_, err = uuid.Parse(firstRevision)
	require.NoError(t, err)

	second, err := updateAuthProviderSyncRevision(req, authProvider.Name)
	require.NoError(t, err)
	secondRevision := second.Annotations[v1.AuthProviderSyncAnnotation]
	_, err = uuid.Parse(secondRevision)
	require.NoError(t, err)
	require.NotEqual(t, firstRevision, secondRevision)
}

func TestRollbackAuthProviderConfigurationPublishesRevision(t *testing.T) {
	const authProviderName = "entra-auth-provider"
	authProvider := &v1.AuthProvider{
		Name:      authProviderName,
		Namespace: system.DefaultNamespace,
	}
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: authProviderName,
		Name:    authProviderName,
		Secrets: map[string]string{"CLIENT_SECRET": "client-secret"},
	}))
	req := api.Context{
		Request:       httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/configure", nil),
		Storage:       newFakeStorage(t, authProvider),
		GatewayClient: gatewayClient,
	}

	cause := errors.New("validation failed")
	err := (&AuthProviderHandler{}).rollbackAuthProviderConfiguration(req, *authProvider, cause)
	require.ErrorIs(t, err, cause)
	_, revealErr := gatewayClient.RevealCredential(t.Context(), []string{authProviderName}, authProviderName)
	require.Error(t, revealErr)

	var updated v1.AuthProvider
	require.NoError(t, req.Get(&updated, authProviderName))
	require.NotEmpty(t, updated.Annotations[v1.AuthProviderSyncAnnotation])
}

func TestDeconfigurePublishesRevisionWhenCredentialIsAlreadyAbsent(t *testing.T) {
	const authProviderName = "entra-auth-provider"
	authProvider := &v1.AuthProvider{
		Name:      authProviderName,
		Namespace: system.DefaultNamespace,
	}
	storage := &acknowledgeAuthProviderRevisionStorage{
		WithWatch: newFakeStorage(t, authProvider),
	}
	gatewayClient := newHandlerTestGateway(t)
	request := httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/deconfigure", nil)
	request.SetPathValue("id", authProviderName)
	req := api.Context{
		Request:       request,
		Storage:       storage,
		GatewayClient: gatewayClient,
	}

	require.NoError(t, (&AuthProviderHandler{}).Deconfigure(req))
	var first v1.AuthProvider
	require.NoError(t, req.Get(&first, authProviderName))
	firstRevision := first.Annotations[v1.AuthProviderSyncAnnotation]
	require.NotEmpty(t, firstRevision)

	require.NoError(t, (&AuthProviderHandler{}).Deconfigure(req))
	var second v1.AuthProvider
	require.NoError(t, req.Get(&second, authProviderName))
	secondRevision := second.Annotations[v1.AuthProviderSyncAnnotation]
	require.NotEmpty(t, secondRevision)
	require.NotEqual(t, firstRevision, secondRevision)
}

func TestDeconfigureReturnsRevisionPublicationFailureAfterCredentialDeletion(t *testing.T) {
	const authProviderName = "entra-auth-provider"
	authProvider := &v1.AuthProvider{
		Name:      authProviderName,
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "entra/",
			},
		},
	}
	storage := &failAuthProviderUpdateStorage{
		WithWatch: newFakeStorage(t, authProvider),
	}
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: authProviderName,
		Name:    authProviderName,
		Secrets: map[string]string{"CLIENT_SECRET": "client-secret"},
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/deconfigure", nil)
	request.SetPathValue("id", authProviderName)
	req := api.Context{
		Request:       request,
		Storage:       storage,
		GatewayClient: gatewayClient,
	}

	err := (&AuthProviderHandler{}).Deconfigure(req)
	require.ErrorContains(t, err, "update auth provider sync revision")
	_, revealErr := gatewayClient.RevealCredential(t.Context(), []string{authProviderName}, authProviderName)
	require.Error(t, revealErr)

	var cleanup v1.AuthProviderCleanup
	require.NoError(t, req.Get(&cleanup, authProviderCleanupName(authProviderName)))
	require.False(t, cleanup.Spec.Ready)
}

func TestAuthProviderStatusAcknowledgedRequiresObservedGeneration(t *testing.T) {
	authProvider := &v1.AuthProvider{
		Generation: 2,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "revision-one",
		},
		Status: v1.AuthProviderStatus{
			ObservedGeneration:   1,
			ObservedSyncRevision: "revision-one",
		},
	}

	require.False(t, authProviderStatusAcknowledged(authProvider))
	authProvider.Status.ObservedGeneration = authProvider.Generation
	require.True(t, authProviderStatusAcknowledged(authProvider))
}
