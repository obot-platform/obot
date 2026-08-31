package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type failProviderChangeCreateStorage struct {
	kclient.WithWatch
}

func (f *failProviderChangeCreateStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if _, ok := obj.(*v1.ProviderConfigurationChange); ok {
		return errors.New("injected provider change create failure")
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
				Name:      name.SafeConcatName(system.AuthProviderCleanupPrefix, "entra-auth-provider"),
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
				Name:      name.SafeConcatName(system.AuthProviderCleanupPrefix, "old-entra"),
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

func TestConfigureAuthProviderStagesCredentialAndWaitsForChangeDeletion(t *testing.T) {
	authProvider := &v1.AuthProvider{
		Name:      "entra-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
					{
						Name: "CLIENT_SECRET",
					},
				},
			},
		},
	}
	storage := newAuthProviderTestStorage(authProvider)
	gatewayClient := newHandlerTestGateway(t)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)
	providerDispatcher := dispatcher.New(nil, storage, gatewayClient, licenseProvider, "", "", "")
	request := httptest.NewRequest(http.MethodPost, "/api/auth-providers/entra-auth-provider/configure", bytes.NewBufferString(`{"CLIENT_SECRET":"secret","EMPTY":""}`))
	request.SetPathValue("id", authProvider.Name)
	req := api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}

	errC := make(chan error, 1)
	go func() {
		errC <- NewAuthProviderHandler(providerDispatcher, "", licenseProvider).Configure(req)
	}()

	var change v1.ProviderConfigurationChange
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		err := storage.Get(t.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.ProviderChangeAuthName,
		}, &change)
		assert.NoError(collect, err)
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, v1.ProviderTypeAuth, change.Spec.ProviderType)
	assert.Equal(t, v1.ProviderDesiredStateConfigured, change.Spec.DesiredState)
	assert.NotEmpty(t, change.Spec.StagedCredentialName)

	staged, err := gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
	require.NoError(t, err)
	assert.Equal(t, "secret", staged.Secrets["CLIENT_SECRET"])
	assert.NotEmpty(t, staged.Secrets[CookieSecretEnvVar])
	assert.NotContains(t, staged.Secrets, "EMPTY")
	_, err = gatewayClient.RevealCredential(t.Context(), []string{authProvider.Name}, authProvider.Name)
	require.Error(t, err)

	require.NoError(t, storage.Delete(t.Context(), &change))
	require.NoError(t, <-errC)
}

func newAuthProviderTestStorage(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(object kclient.Object) []string {
			return []string{strconv.FormatBool(object.(*v1.AuthProvider).Status.Configured)}
		}).
		Build()
}

func TestSubmitProviderConfigurationChangeReturnsConflict(t *testing.T) {
	existing := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: "existing",
			DesiredState: v1.ProviderDesiredStateDeconfigured,
		},
	}
	req := api.Context{
		Request:       httptest.NewRequest(http.MethodPost, "/", nil),
		Storage:       newFakeStorage(t, existing),
		GatewayClient: newHandlerTestGateway(t),
	}
	err := submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      existing.Name,
		Namespace: existing.Namespace,
		Spec:      existing.Spec,
	})
	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.Code)
}

func TestFailedProviderChangeCreationRemovesOnlyItsStage(t *testing.T) {
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.StagedProviderCredentialContext,
		Name:    "other-stage",
		Secrets: map[string]string{"key": "other"},
	}))
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.StagedProviderCredentialContext,
		Name:    "request-stage",
		Secrets: map[string]string{"key": "request"},
	}))
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPost, "/", nil),
		Storage: &failProviderChangeCreateStorage{
			WithWatch: newFakeStorage(t),
		},
		GatewayClient: gatewayClient,
	}
	err := submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         "provider",
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: "request-stage",
		},
	})
	require.ErrorContains(t, err, "injected provider change create failure")
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "request-stage")
	require.Error(t, err)
	other, err := gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "other-stage")
	require.NoError(t, err)
	assert.Equal(t, "other", other.Secrets["key"])
}

func TestSubmitProviderConfigurationChangeReturnsTerminalError(t *testing.T) {
	const terminalError = "only one authentication provider can be configured at a time"

	storage := newFakeStorage(t)
	req := api.Context{
		Request:       httptest.NewRequest(http.MethodPost, "/", nil),
		Storage:       storage,
		GatewayClient: newHandlerTestGateway(t),
	}

	errC := make(chan error, 1)
	go func() {
		errC <- submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
			Name:      system.ProviderChangeAuthName,
			Namespace: system.DefaultNamespace,
			Spec: v1.ProviderConfigurationChangeSpec{
				ProviderType: v1.ProviderTypeAuth,
				ProviderName: "provider",
				DesiredState: v1.ProviderDesiredStateDeconfigured,
			},
		})
	}()

	// The wait only starts watching once the change exists, so keep writing the
	// terminal status until the watch picks it up.
	var err error
	require.Eventually(t, func() bool {
		var persisted v1.ProviderConfigurationChange
		if getErr := storage.Get(t.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.ProviderChangeAuthName,
		}, &persisted); getErr == nil {
			persisted.Status.Error = terminalError
			// The fake client has no status subresource for this type, so the
			// status has to be written with a regular update.
			_ = storage.Update(t.Context(), &persisted)
		}
		select {
		case err = <-errC:
			return true
		case <-time.After(10 * time.Millisecond):
			return false
		}
	}, 10*time.Second, time.Millisecond)

	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Error(), terminalError)
}
