package providerconfigurationchange

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type failCleanupCreateClient struct {
	kclient.WithWatch
}

func (f *failCleanupCreateClient) Create(ctx context.Context, object kclient.Object, options ...kclient.CreateOption) error {
	if _, ok := object.(*v1.AuthProviderCleanup); ok {
		return errors.New("injected cleanup intent failure")
	}
	return f.WithWatch.Create(ctx, object, options...)
}

func TestConfigureAuthProviderAppliesAndCleansUp(t *testing.T) {
	provider := &v1.AuthProvider{
		Name:       "auth-provider",
		Namespace:  system.DefaultNamespace,
		Generation: 7,
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
	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         provider.Name,
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: "stage",
		},
	}
	client := newProviderChangeTestClient(provider, change)
	gatewayClient := newProviderChangeTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.StagedProviderCredentialContext,
		Name:    change.Spec.StagedCredentialName,
		Secrets: map[string]string{
			"CLIENT_SECRET": "new-secret",
		},
	}))
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)
	handler := New(gatewayClient, dispatcher.New(nil, client, gatewayClient, licenseProvider, "", "", ""), licenseProvider, "")

	require.NoError(t, handler.Reconcile(router.Request{
		Client:    client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	assert.True(t, change.Status.Applied)

	active, err := gatewayClient.RevealCredential(t.Context(), []string{provider.Name}, provider.Name)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", active.Secrets["CLIENT_SECRET"])
	var updatedProvider v1.AuthProvider
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(provider), &updatedProvider))
	assert.True(t, updatedProvider.Status.Configured)
	assert.Equal(t, provider.Generation, updatedProvider.Status.ObservedGeneration)
	var daemonSync v1.ProviderSync
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ProviderSyncName,
	}, &daemonSync))
	revision := daemonSync.Spec.Revisions[providerDaemonRevisionKey(v1.ProviderTypeAuth, provider.Namespace, provider.Name)]
	assert.Equal(t, v1.ProviderTypeAuth, revision.ProviderType)
	assert.Equal(t, provider.Namespace, revision.ProviderNamespace)
	assert.Equal(t, provider.Name, revision.ProviderName)
	assert.Equal(t, int64(1), revision.Revision)
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
	require.NoError(t, err)

	require.NoError(t, client.Status().Update(t.Context(), change))
	require.NoError(t, handler.Reconcile(router.Request{
		Client:    client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
	require.Error(t, err)
	var deletedChange v1.ProviderConfigurationChange
	err = client.Get(t.Context(), kclient.ObjectKeyFromObject(change), &deletedChange)
	require.True(t, apierrors.IsNotFound(err))
}

func TestConfigureAuthProviderRejectsConflictingConfiguredProvider(t *testing.T) {
	configuredProvider := &v1.AuthProvider{
		Name:      "configured-auth-provider",
		Namespace: system.DefaultNamespace,
		Status: v1.AuthProviderStatus{
			Configured: true,
		},
	}
	targetProvider := &v1.AuthProvider{
		Name:      "target-auth-provider",
		Namespace: system.DefaultNamespace,
	}
	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         targetProvider.Name,
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: "target-stage",
		},
	}
	client := newProviderChangeTestClient(configuredProvider, targetProvider, change)
	gatewayClient := newProviderChangeTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: configuredProvider.Name,
		Name:    configuredProvider.Name,
		Secrets: map[string]string{
			"CLIENT_SECRET": "configured-secret",
		},
	}))
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.StagedProviderCredentialContext,
		Name:    change.Spec.StagedCredentialName,
		Secrets: map[string]string{
			"CLIENT_SECRET": "target-secret",
		},
	}))
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)
	handler := New(gatewayClient, dispatcher.New(nil, client, gatewayClient, licenseProvider, "", "", ""), licenseProvider, "")

	require.NoError(t, handler.Reconcile(router.Request{
		Client:    client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	require.NotEmpty(t, change.Status.Error)
	assert.Contains(t, change.Status.Error, configuredProvider.Name)
	_, err = gatewayClient.RevealCredential(t.Context(), []string{targetProvider.Name}, targetProvider.Name)
	require.ErrorAs(t, err, &gatewayclient.CredentialNotFoundError{})
	configuredCredential, err := gatewayClient.RevealCredential(t.Context(), []string{configuredProvider.Name}, configuredProvider.Name)
	require.NoError(t, err)
	assert.Equal(t, "configured-secret", configuredCredential.Secrets["CLIENT_SECRET"])

	require.NoError(t, client.Status().Update(t.Context(), change))
	require.NoError(t, handler.Reconcile(router.Request{
		Client:    client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
	require.ErrorAs(t, err, &gatewayclient.CredentialNotFoundError{})
	var deletedChange v1.ProviderConfigurationChange
	err = client.Get(t.Context(), kclient.ObjectKeyFromObject(change), &deletedChange)
	require.True(t, apierrors.IsNotFound(err))
}

func TestAuthDeconfigurationPersistsCleanupBeforeCredentialDeletion(t *testing.T) {
	provider := &v1.AuthProvider{
		Name:      "auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "groups/",
			},
		},
	}
	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: provider.Name,
			DesiredState: v1.ProviderDesiredStateDeconfigured,
		},
	}
	baseClient := newProviderChangeTestClient(provider, change)
	client := &failCleanupCreateClient{WithWatch: baseClient}
	gatewayClient := newProviderChangeTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: provider.Name,
		Name:    provider.Name,
		Secrets: map[string]string{"CLIENT_SECRET": "old-secret"},
	}))
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)
	handler := New(gatewayClient, nil, licenseProvider, "")

	err = handler.Reconcile(router.Request{
		Client:    client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil)
	require.ErrorContains(t, err, "persist cleanup intent")
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{provider.Name}, provider.Name)
	require.NoError(t, err)
	assert.Equal(t, "old-secret", credential.Secrets["CLIENT_SECRET"])
}

func TestCleanupOrphanedStagedCredentials(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         "auth-provider",
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: "referenced-old-stage",
		},
	}
	client := newProviderChangeTestClient(change)
	gatewayClient := newProviderChangeTestGateway(t)
	upsertCredential := func(credentialContext, credentialName string, createdAt time.Time) {
		t.Helper()
		require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
			CreatedAt: createdAt,
			Context:   credentialContext,
			Name:      credentialName,
			Secrets: map[string]string{
				"SECRET": credentialName,
			},
		}))
	}

	upsertCredential(system.StagedProviderCredentialContext, "referenced-old-stage", now.Add(-2*OrphanedStagedCredentialGracePeriod))
	upsertCredential(system.StagedProviderCredentialContext, "recent-orphan-stage", now.Add(-OrphanedStagedCredentialGracePeriod+time.Second))
	upsertCredential(system.StagedProviderCredentialContext, "boundary-orphan-stage", now.Add(-OrphanedStagedCredentialGracePeriod))
	upsertCredential(system.StagedProviderCredentialContext, "old-orphan-stage", now.Add(-OrphanedStagedCredentialGracePeriod-time.Second))
	upsertCredential("active-provider-context", "old-active-credential", now.Add(-2*OrphanedStagedCredentialGracePeriod))

	require.NoError(t, CleanupOrphanedStagedCredentials(
		t.Context(),
		client,
		gatewayClient,
		now,
		OrphanedStagedCredentialGracePeriod,
	))

	_, err := gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "referenced-old-stage")
	require.NoError(t, err)
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "recent-orphan-stage")
	require.NoError(t, err)
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "boundary-orphan-stage")
	require.ErrorAs(t, err, &gatewayclient.CredentialNotFoundError{})
	_, err = gatewayClient.RevealCredential(t.Context(), []string{system.StagedProviderCredentialContext}, "old-orphan-stage")
	require.ErrorAs(t, err, &gatewayclient.CredentialNotFoundError{})
	_, err = gatewayClient.RevealCredential(t.Context(), []string{"active-provider-context"}, "old-active-credential")
	require.NoError(t, err)
}

func TestAdvanceDaemonSyncIsMonotonicAndRecreatesSingleton(t *testing.T) {
	client := newProviderChangeTestClient()
	handler := &Handler{}
	authChange := &v1.ProviderConfigurationChange{
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: "auth-provider",
		},
	}
	modelChange := &v1.ProviderConfigurationChange{
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeModel,
			ProviderName: "model-provider",
		},
	}
	require.NoError(t, handler.advanceDaemonSync(t.Context(), client, authChange))

	var daemonSync v1.ProviderSync
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.ProviderSyncName}
	require.NoError(t, client.Get(t.Context(), key, &daemonSync))
	authRevisionKey := providerDaemonRevisionKey(v1.ProviderTypeAuth, authChange.Namespace, authChange.Spec.ProviderName)
	modelRevisionKey := providerDaemonRevisionKey(v1.ProviderTypeModel, modelChange.Namespace, modelChange.Spec.ProviderName)
	assert.Equal(t, int64(1), daemonSync.Spec.Revisions[authRevisionKey].Revision)

	require.NoError(t, handler.advanceDaemonSync(t.Context(), client, authChange))
	require.NoError(t, handler.advanceDaemonSync(t.Context(), client, modelChange))
	require.NoError(t, client.Get(t.Context(), key, &daemonSync))
	assert.Equal(t, int64(2), daemonSync.Spec.Revisions[authRevisionKey].Revision)
	assert.Equal(t, int64(1), daemonSync.Spec.Revisions[modelRevisionKey].Revision)

	require.NoError(t, client.Delete(t.Context(), &daemonSync))
	require.NoError(t, handler.advanceDaemonSync(t.Context(), client, authChange))
	require.NoError(t, client.Get(t.Context(), key, &daemonSync))
	assert.Equal(t, int64(1), daemonSync.Spec.Revisions[authRevisionKey].Revision)
	assert.NotContains(t, daemonSync.Spec.Revisions, modelRevisionKey)
}

func newProviderChangeTestClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(object kclient.Object) []string {
			return []string{strconv.FormatBool(object.(*v1.AuthProvider).Status.Configured)}
		}).
		WithStatusSubresource(
			&v1.AuthProvider{},
			&v1.ModelProvider{},
			&v1.ProviderConfigurationChange{},
		).
		WithObjects(objects...).
		Build()
}

func newProviderChangeTestGateway(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	gateway := gatewayclient.New(ctx, database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}

func TestValidateChange(t *testing.T) {
	tests := []struct {
		name    string
		change  v1.ProviderConfigurationChange
		wantErr string
	}{
		{
			name: "valid model configuration",
			change: v1.ProviderConfigurationChange{
				Name:      "pcc1-model-provider",
				Namespace: system.DefaultNamespace,
				Spec: v1.ProviderConfigurationChangeSpec{
					ProviderType:         v1.ProviderTypeModel,
					ProviderName:         "model-provider",
					DesiredState:         v1.ProviderDesiredStateConfigured,
					StagedCredentialName: "stage",
				},
			},
		},
		{
			name: "configuration requires stage",
			change: v1.ProviderConfigurationChange{
				Name:      system.ProviderChangeAuthName,
				Namespace: system.DefaultNamespace,
				Spec: v1.ProviderConfigurationChangeSpec{
					ProviderType: v1.ProviderTypeAuth,
					ProviderName: "auth-provider",
					DesiredState: v1.ProviderDesiredStateConfigured,
				},
			},
			wantErr: "no staged credential",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateChange(&test.change)
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}
		})
	}
}
