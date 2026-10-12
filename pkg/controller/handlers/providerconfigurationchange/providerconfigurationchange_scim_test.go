package providerconfigurationchange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/controller/handlers/cleanup"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/scim/setup"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oktaProviderName        = "okta-auth-provider"
	oktaIssuerParam         = "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
	oktaClientIDParam       = "OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID"
	oktaServiceClientParam  = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"
	oktaServiceKeyParam     = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"
	activeProviderName      = "github-auth-provider"
	activeProviderParameter = "GITHUB_CLIENT_SECRET"
)

// scimChangeTest reconciles provider configuration changes of the Okta provider, which supports SCIM, next to
// another provider that serves logins when a test stages Okta as its replacement.
type scimChangeTest struct {
	t       *testing.T
	client  kclient.WithWatch
	gateway *gatewayclient.Client
	handler *Handler
	// errorCode is the HTTP status that the last change applied asked the API to refuse it with.
	errorCode int
	// failSCIMConnectionDeletes makes every deletion of a SCIM connection fail while it is set.
	failSCIMConnectionDeletes *atomic.Bool
}

// fakeResponse is the response of a cleanup reconcile, which asks to be retried while the cleanup is not ready.
type fakeResponse struct {
	router.Response
}

// keyedTransformer encrypts with a key that can change, as rotating or losing an encryption key does. Credentials
// encrypted with an earlier key can no longer be decrypted.
type keyedTransformer struct {
	key string
}

func (k *keyedTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte(k.key+":"), data...), nil
}

func (k *keyedTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	out, ok := bytes.CutPrefix(data, []byte(k.key+":"))
	if !ok {
		return nil, false, errors.New("the credential was encrypted with another key")
	}
	return out, false, nil
}

func (*fakeResponse) RetryAfter(_ time.Duration) {}

func newSCIMChangeTest(t *testing.T, objects ...kclient.Object) *scimChangeTest {
	t.Helper()
	return newSCIMChangeTestWithEncryption(t, nil, objects...)
}

// newSCIMChangeTestWithEncryption is newSCIMChangeTest with the gateway encrypting credentials with encryption.
func newSCIMChangeTestWithEncryption(t *testing.T, encryption *encryptionconfig.EncryptionConfiguration, objects ...kclient.Object) *scimChangeTest {
	t.Helper()

	okta := &v1.AuthProvider{
		Name:      oktaProviderName,
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Name: "Okta",
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: oktaClientIDParam,
						},
						{
							Name: oktaIssuerParam,
						},
						{
							Name: oktaServiceClientParam,
						},
						{
							Name: oktaServiceKeyParam,
						},
					},
				},
				GroupIDPrefix: "okta/",
			},
		},
	}
	active := &v1.AuthProvider{
		Name:      activeProviderName,
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: activeProviderParameter,
						},
					},
				},
				GroupIDPrefix: "github/",
			},
		},
	}

	defaultRole := &v1.UserDefaultRoleSetting{
		Name:      system.DefaultRoleSettingName,
		Namespace: system.DefaultNamespace,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: clienttypes.RoleBasic,
		},
	}
	client := newProviderChangeTestClient(append([]kclient.Object{okta, active, defaultRole}, objects...)...)
	gateway, failSCIMConnectionDeletes := newSCIMChangeTestGateway(t, client, encryption)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)

	return &scimChangeTest{
		t:                         t,
		client:                    client,
		gateway:                   gateway,
		handler:                   New(gateway, dispatcher.New(nil, client, gateway, licenseProvider, "", "", ""), licenseProvider, "", client),
		failSCIMConnectionDeletes: failSCIMConnectionDeletes,
	}
}

// newSCIMChangeTestGateway returns a gateway client over a new in-memory database, which records the controller
// objects that sign-ins create in storage, and a switch that makes every deletion of a SCIM connection fail while it
// is set.
func newSCIMChangeTestGateway(t *testing.T, storage kclient.Client, encryption *encryptionconfig.EncryptionConfiguration) (*gatewayclient.Client, *atomic.Bool) {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	// The callback is registered before the client starts the background loops that use the database.
	failSCIMConnectionDeletes := new(atomic.Bool)
	require.NoError(t, services.DB.DB.Callback().Delete().Before("gorm:delete").Register("test:fail_scim_connection_deletes", func(tx *gorm.DB) {
		if failSCIMConnectionDeletes.Load() && tx.Statement.Table == "scim_connections" {
			_ = tx.AddError(errors.New("the SCIM connection could not be deleted"))
		}
	}))
	gateway := gatewayclient.New(t.Context(), database, storage, encryption, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway, failSCIMConnectionDeletes
}

// oidcSettings returns Okta's OIDC settings, without the directory parameters.
func oidcSettings() map[string]string {
	return map[string]string{
		oktaClientIDParam: "oidc-client",
		oktaIssuerParam:   "https://example.okta.com/",
	}
}

// directorySettings returns Okta's OIDC settings with the directory parameters.
func directorySettings() map[string]string {
	settings := oidcSettings()
	settings[oktaServiceClientParam] = "service-client"
	settings[oktaServiceKeyParam] = "service-key"
	return settings
}

// activateOtherProvider makes the other provider the configured one.
func (s *scimChangeTest) activateOtherProvider() {
	s.t.Helper()

	require.NoError(s.t, s.gateway.UpsertCredential(s.t.Context(), gatewaytypes.Credential{
		Context: activeProviderName,
		Name:    activeProviderName,
		Secrets: map[string]string{
			activeProviderParameter: "secret",
		},
	}))
}

// apply reconciles a change of the Okta provider to desiredState with secrets, and returns the change's error.
func (s *scimChangeTest) apply(desiredState v1.ProviderDesiredState, secrets map[string]string) string {
	s.t.Helper()

	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: oktaProviderName,
			DesiredState: desiredState,
		},
	}
	if secrets != nil {
		change.Spec.StagedCredentialName = "stage-" + string(desiredState)
		require.NoError(s.t, s.gateway.UpsertCredential(s.t.Context(), gatewaytypes.Credential{
			Context: system.StagedProviderCredentialContext,
			Name:    change.Spec.StagedCredentialName,
			Secrets: secrets,
		}))
	}
	require.NoError(s.t, s.client.Create(s.t.Context(), change))
	s.t.Cleanup(func() {
		_ = s.client.Delete(s.t.Context(), change)
	})

	require.NoError(s.t, s.handler.Reconcile(router.Request{
		Client:    s.client,
		Object:    change,
		Ctx:       s.t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	require.NoError(s.t, s.client.Delete(s.t.Context(), change))
	s.errorCode = change.Status.ErrorCode
	return change.Status.Error
}

// connection returns the Okta provider's SCIM connection, or nil.
func (s *scimChangeTest) connection() *gatewaytypes.SCIMConnection {
	s.t.Helper()

	conn, err := s.gateway.SCIMConnectionForAuthProvider(s.t.Context(), system.DefaultNamespace, oktaProviderName)
	require.NoError(s.t, err)
	return conn
}

// credential returns the Okta provider's credential in the credential context, or nil when it has none.
func (s *scimChangeTest) credential(context string) map[string]string {
	s.t.Helper()

	cred, err := s.gateway.RevealCredential(s.t.Context(), []string{context}, oktaProviderName)
	if err != nil {
		require.ErrorAs(s.t, err, &gatewayclient.CredentialNotFoundError{})
		return nil
	}
	return cred.Secrets
}

func (s *scimChangeTest) status() v1.AuthProviderStatus {
	s.t.Helper()

	var provider v1.AuthProvider
	require.NoError(s.t, s.client.Get(s.t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: oktaProviderName}, &provider))
	return provider.Status
}

// daemonRevision returns the Okta provider's daemon revision, which a change advances to restart the daemon, and zero
// before any change created the revisions.
func (s *scimChangeTest) daemonRevision() int64 {
	s.t.Helper()

	var sync v1.ProviderSync
	err := s.client.Get(s.t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.ProviderSyncName}, &sync)
	if apierrors.IsNotFound(err) {
		return 0
	}
	require.NoError(s.t, err)
	return sync.Spec.Revisions[providerDaemonRevisionKey(v1.ProviderTypeAuth, system.DefaultNamespace, oktaProviderName)].Revision
}

// directoryStub stands in for the Okta provider daemon, and counts the requests to its directory endpoints.
func directoryStub(t *testing.T) (*atomic.Int32, string) {
	t.Helper()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/obot-list-") || strings.HasPrefix(r.URL.Path, "/obot-get-auth-groups") || strings.HasPrefix(r.URL.Path, "/obot-get-group-migration-mapping") {
			requests.Add(1)
		}
		http.Error(w, "the directory is not available", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	return &requests, srv.URL
}

// signIn signs the native Okta user in with the Owner role, as Owner Setup and a staged switch's verification do.
func (s *scimChangeTest) signIn(providerURL, nativeID string) *gatewaytypes.User {
	s.t.Helper()

	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(s.t.Context(), providerURL), "okta/"), "access-token")
	user, err := s.gateway.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      oktaProviderName,
		ProviderUsername:      nativeID,
		ProviderUserID:        nativeID,
		Email:                 nativeID + "@example.com",
	}, "", clienttypes.RoleOwner, gatewayclient.UserLimit{
		Unlimited: true,
	})
	require.NoError(s.t, err)
	return user
}

func TestConfigureWithoutDirectoryParametersSetsUpSCIM(t *testing.T) {
	s := newSCIMChangeTest(t)

	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))

	conn := s.connection()
	require.NotNil(t, conn)
	assert.Equal(t, gatewaytypes.SCIMConnectionOriginSCIMFirst, conn.Origin)
	assert.Equal(t, gatewaytypes.SCIMConnectionStateConnected, conn.State)
	assert.Equal(t, "https://example.okta.com", conn.Issuer)
	assert.False(t, conn.HasToken())

	// The stored credential holds no directory parameters, and the provider is configured without them.
	stored := s.credential(oktaProviderName)
	assert.Equal(t, "oidc-client", stored[oktaClientIDParam])
	assert.NotContains(t, stored, oktaServiceClientParam)
	assert.NotContains(t, stored, oktaServiceKeyParam)
	assert.True(t, s.status().Configured)

	// The first Owner signs in without any request to the directory.
	requests, providerURL := directoryStub(t)
	owner := s.signIn(providerURL, "00u-owner")
	assert.True(t, owner.Role.HasRole(clienttypes.RoleOwner))
	assert.Zero(t, requests.Load())

	// Reconfiguring the configured provider keeps its connection, and never stores the directory parameters.
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.NotContains(t, s.credential(oktaProviderName), oktaServiceClientParam)
}

func TestConfigureWithDirectoryParametersSynchronizesTheDirectory(t *testing.T) {
	s := newSCIMChangeTest(t)

	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
	assert.Nil(t, s.connection())
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
	assert.True(t, s.status().Configured)

	// Once it synchronizes its directory, removing the directory parameters is refused, and nothing changes.
	errMsg := s.apply(v1.ProviderDesiredStateConfigured, oidcSettings())
	assert.Contains(t, errMsg, "synchronizes its directory at sign-in")
	assert.Nil(t, s.connection())
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
}

func TestConfigureRefusesPartialDirectoryParameters(t *testing.T) {
	s := newSCIMChangeTest(t)

	partial := oidcSettings()
	partial[oktaServiceKeyParam] = "service-key"
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, partial), "none of them")
	assert.Zero(t, s.errorCode, "a refusal of the configuration itself is a bad request")
	assert.Nil(t, s.connection())
	assert.Nil(t, s.credential(oktaProviderName))
}

func TestConfigureRefusesResidualGroupDataUntilTheCleanup(t *testing.T) {
	s := newSCIMChangeTest(t, &v1.AccessControlRule{
		Name:      "servers",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			Manifest: clienttypes.AccessControlRuleManifest{
				DisplayName: "Servers",
				Subjects: []clienttypes.Subject{
					{
						Type: clienttypes.SubjectTypeGroup,
						ID:   "okta/00g-legacy",
					},
				},
			},
		},
	})

	errMsg := s.apply(v1.ProviderDesiredStateConfigured, oidcSettings())
	assert.Contains(t, errMsg, "still has group data")
	// The API refuses it with a conflict too, when it finds the data first.
	assert.Equal(t, http.StatusConflict, s.errorCode)
	assert.Contains(t, errMsg, "okta/00g-legacy")
	assert.Contains(t, errMsg, `access control rule "Servers"`)
	assert.Nil(t, s.connection())
	assert.Nil(t, s.credential(oktaProviderName), "a refused configuration promoted its credential")

	// Deconfiguring the provider runs its auth provider cleanup, which removes exactly that data.
	require.Empty(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil))
	var cleanups v1.AuthProviderCleanupList
	require.NoError(t, s.client.List(t.Context(), &cleanups))
	require.Len(t, cleanups.Items, 1)

	// While the cleanup is pending, configuring is refused.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()), "still being deconfigured")
	assert.Nil(t, s.connection())

	cleaner := cleanup.NewAuthProviderCleanup(s.gateway)
	for range 3 {
		var pending v1.AuthProviderCleanup
		if err := s.client.Get(t.Context(), kclient.ObjectKeyFromObject(&cleanups.Items[0]), &pending); err != nil {
			break
		}
		require.NoError(t, cleaner.Cleanup(router.Request{
			Client:    s.client,
			Object:    &pending,
			Ctx:       t.Context(),
			Namespace: pending.Namespace,
			Name:      pending.Name,
		}, &fakeResponse{}))
	}
	require.NoError(t, s.client.List(t.Context(), &cleanups))
	require.Empty(t, cleanups.Items)

	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	require.NotNil(t, s.connection())
}

func TestConfigureReusesTheConnectionOfAFailedAttempt(t *testing.T) {
	s := newSCIMChangeTest(t)

	// An earlier attempt created the connection, and failed before promoting the credential.
	conn, _, err := s.gateway.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      oktaProviderName,
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	require.NoError(t, err)

	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.True(t, s.status().Configured)
}

func TestConfigureRefusesASecondConnection(t *testing.T) {
	s := newSCIMChangeTest(t)
	_, _, err := s.gateway.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: "other",
		AuthProviderName:      oktaProviderName,
		GroupIDPrefix:         "okta-other/",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	require.NoError(t, err)

	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()), "only SCIM connection")
	assert.Nil(t, s.connection())
	assert.Nil(t, s.credential(oktaProviderName))
}

func TestStageWithoutDirectoryParametersSetsUpSCIMAndUnstageDeletesIt(t *testing.T) {
	s := newSCIMChangeTest(t)
	s.activateOtherProvider()

	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	assert.Equal(t, gatewaytypes.SCIMConnectionOriginSCIMFirst, conn.Origin)
	assert.NotContains(t, s.credential(system.ReplacementAuthProviderCredentialContext), oktaServiceClientParam)
	assert.Nil(t, s.credential(oktaProviderName))

	// The verifying Owner of the switch signs in without any request to the directory.
	requests, providerURL := directoryStub(t)
	s.signIn(providerURL, "00u-verifier")
	assert.Zero(t, requests.Load())

	// Staging again reuses the connection.
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	assert.Equal(t, conn.ID, s.connection().ID)

	// Unstaging deletes the connection, which never served a request, with the staged settings.
	require.Empty(t, s.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.Nil(t, s.connection())
	assert.Nil(t, s.credential(system.ReplacementAuthProviderCredentialContext))

	// Staging with the directory parameters sets up directory synchronization instead.
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, directorySettings()))
	assert.Nil(t, s.connection())
	assert.Equal(t, "service-client", s.credential(system.ReplacementAuthProviderCredentialContext)[oktaServiceClientParam])
}

func TestSwitchToAStagedSCIMProvider(t *testing.T) {
	s := newSCIMChangeTest(t)
	s.activateOtherProvider()

	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         oktaProviderName,
			DesiredState:         v1.ProviderDesiredStateSwitched,
			ReplacesProviderName: activeProviderName,
		},
	}
	require.NoError(t, s.client.Create(t.Context(), change))
	require.NoError(t, s.handler.Reconcile(router.Request{
		Client:    s.client,
		Object:    change,
		Ctx:       t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	require.Empty(t, change.Status.Error)

	// Activation needs nothing further: the connection that staging created serves the switched provider.
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.Equal(t, "oidc-client", s.credential(oktaProviderName)[oktaClientIDParam])
	assert.NotContains(t, s.credential(oktaProviderName), oktaServiceClientParam)
	assert.True(t, s.status().Configured)
}

// switchProvider reconciles a switch to the staged provider providerName from the configured provider replaces, and
// returns the change's error.
func (s *scimChangeTest) switchProvider(providerName, replaces string) string {
	s.t.Helper()

	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         providerName,
			DesiredState:         v1.ProviderDesiredStateSwitched,
			ReplacesProviderName: replaces,
		},
	}
	require.NoError(s.t, s.client.Create(s.t.Context(), change))
	require.NoError(s.t, s.handler.Reconcile(router.Request{
		Client:    s.client,
		Object:    change,
		Ctx:       s.t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	require.NoError(s.t, s.client.Delete(s.t.Context(), change))
	return change.Status.Error
}

// switchAwayFromOkta stages the other provider, and switches to it from Okta.
func (s *scimChangeTest) switchAwayFromOkta() {
	s.t.Helper()

	require.NoError(s.t, s.gateway.UpsertCredential(s.t.Context(), gatewaytypes.Credential{
		Context: system.ReplacementAuthProviderCredentialContext,
		Name:    activeProviderName,
		Secrets: map[string]string{
			activeProviderParameter: "secret",
		},
	}))
	require.Empty(s.t, s.switchProvider(activeProviderName, oktaProviderName))
	configured, err := s.handler.dispatcher.GetConfiguredAuthProvider(s.t.Context())
	require.NoError(s.t, err)
	require.Equal(s.t, activeProviderName, configured)
}

// runAuthProviderCleanups runs the pending auth provider cleanups until they finish.
func (s *scimChangeTest) runAuthProviderCleanups() {
	s.t.Helper()

	cleaner := cleanup.NewAuthProviderCleanup(s.gateway)
	for range 10 {
		var cleanups v1.AuthProviderCleanupList
		require.NoError(s.t, s.client.List(s.t.Context(), &cleanups))
		if len(cleanups.Items) == 0 {
			return
		}
		for i := range cleanups.Items {
			require.NoError(s.t, cleaner.Cleanup(router.Request{
				Client:    s.client,
				Object:    &cleanups.Items[i],
				Ctx:       s.t.Context(),
				Namespace: cleanups.Items[i].Namespace,
				Name:      cleanups.Items[i].Name,
			}, &fakeResponse{}))
		}
	}
	s.t.Fatal("the auth provider cleanups did not finish")
}

// provisionUser provisions the native Okta user through the connection.
func (s *scimChangeTest) provisionUser(conn *gatewaytypes.SCIMConnection, nativeID string) *gatewayclient.SCIMUser {
	s.t.Helper()

	user, err := s.gateway.CreateSCIMUser(s.t.Context(), conn, gatewayclient.SCIMUserInput{
		UserName:   nativeID + "@example.com",
		ExternalID: nativeID,
	}, gatewayclient.SCIMUserCreateOptions{
		UserLimit: gatewayclient.UserLimit{
			Unlimited: true,
		},
		DefaultRole: clienttypes.RoleBasic,
	})
	require.NoError(s.t, err)
	return user
}

// deactivateUser has Okta deactivate the user it provisioned through SCIM, as it does in production.
func (s *scimChangeTest) deactivateUser(conn *gatewaytypes.SCIMConnection, user *gatewayclient.SCIMUser) {
	s.t.Helper()

	_, err := s.gateway.UpdateSCIMUser(s.t.Context(), conn, user.ID, func(current gatewayclient.SCIMUser) (gatewayclient.SCIMUserInput, error) {
		return gatewayclient.SCIMUserInput{
			UserName:   current.UserName,
			ExternalID: current.ExternalID,
			Active:     new(false),
			Profile:    current.Profile,
		}, nil
	})
	require.NoError(s.t, err)
}

func TestDeconfiguringDeletesTheSCIMConnection(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	provisioned := s.provisionUser(conn, "00u-alice")

	// The connection goes with the credential. The user stays.
	require.Empty(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil))
	assert.Nil(t, s.connection())
	assert.Nil(t, s.credential(oktaProviderName))
	_, err := s.gateway.UserByID(t.Context(), fmt.Sprint(provisioned.UserID))
	require.NoError(t, err)

	// A retried deconfiguration finds nothing more to delete.
	require.Empty(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil))

	// Configuring again waits for the cleanup, and then starts SCIM over with a new connection.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()), "still being deconfigured")
	s.runAuthProviderCleanups()
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	again := s.connection()
	require.NotNil(t, again)
	assert.NotEqual(t, conn.ID, again.ID)
}

func TestSwitchingAwayFromASCIMProviderDeletesItsSCIMData(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	// Okta provisions a member of a group, which has a role and is a subject of a policy, and a user it deactivates.
	// A user who never was provisioned is disabled when the member, who signed in, enforces SCIM.
	_, providerURL := directoryStub(t)
	unprovisioned := s.signIn(providerURL, "00u-carol")
	member := s.provisionUser(conn, "00u-alice")
	s.signIn(providerURL, "00u-alice")
	group, err := s.gateway.CreateSCIMGroup(t.Context(), conn, gatewayclient.SCIMGroupInput{
		DisplayName: "Team",
		MemberIDs:   []string{member.ID},
	})
	require.NoError(t, err)
	_, err = s.gateway.CreateGroupRoleAssignment(t.Context(), group.GroupID, clienttypes.RoleAdmin, "")
	require.NoError(t, err)
	rule := &v1.AccessControlRule{
		Name:      "servers",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			Manifest: clienttypes.AccessControlRuleManifest{
				DisplayName: "Servers",
				Subjects: []clienttypes.Subject{
					{
						Type: clienttypes.SubjectTypeGroup,
						ID:   group.GroupID,
					},
				},
			},
		},
	}
	require.NoError(t, s.client.Create(t.Context(), rule))
	deactivated := s.provisionUser(conn, "00u-bob")
	s.deactivateUser(conn, deactivated)
	enforced, err := s.gateway.EnforceSCIMConnection(t.Context(), conn.ID, gatewayclient.EnforceSCIMOptions{
		Actor: gatewayclient.SCIMEnforceActor{
			UserID:                member.UserID,
			AuthProviderNamespace: system.DefaultNamespace,
			AuthProviderName:      oktaProviderName,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint{unprovisioned.ID}, enforced.DisabledUserIDs)

	s.switchAwayFromOkta()

	// Okta's connection goes with its credential, and so do its groups and their role assignments. Its users stay, and
	// those SCIM disabled stay disabled, until an administrator enables them.
	assert.Nil(t, s.credential(oktaProviderName), "the outgoing provider kept its credential")
	assert.Nil(t, s.connection())
	granted, err := s.gateway.ListGroupIDsForUser(t.Context(), member.UserID)
	require.NoError(t, err)
	assert.Empty(t, granted)
	_, err = s.gateway.GetGroupRoleAssignment(t.Context(), group.GroupID)
	assert.ErrorIs(t, err, gatewayclient.ErrGroupRoleAssignmentNotFound)
	for _, tt := range []struct {
		userID       uint
		wantDisabled bool
	}{
		{
			userID: member.UserID,
		},
		{
			userID:       deactivated.UserID,
			wantDisabled: true,
		},
		{
			userID:       unprovisioned.ID,
			wantDisabled: true,
		},
	} {
		user, err := s.gateway.UserByID(t.Context(), fmt.Sprint(tt.userID))
		require.NoError(t, err)
		assert.Equal(t, tt.wantDisabled, user.DisabledAt != nil, "user %d", tt.userID)
	}

	// The cleanup removes the group from policies, and reconciles the provider's users, as for any provider.
	s.runAuthProviderCleanups()
	require.NoError(t, s.client.Get(t.Context(), kclient.ObjectKeyFromObject(rule), rule))
	assert.Empty(t, rule.Spec.Manifest.Subjects)
	var roleChanges v1.UserRoleChangeList
	require.NoError(t, s.client.List(t.Context(), &roleChanges))
	var groupChanges v1.UserGroupChangeList
	require.NoError(t, s.client.List(t.Context(), &groupChanges))
	for _, userID := range []uint{member.UserID, deactivated.UserID, unprovisioned.ID} {
		assert.True(t, slices.ContainsFunc(roleChanges.Items, func(change v1.UserRoleChange) bool {
			return change.Spec.UserID == userID
		}), "no role change for user %d", userID)
		assert.True(t, slices.ContainsFunc(groupChanges.Items, func(change v1.UserGroupChange) bool {
			return change.Spec.UserID == userID
		}), "no group change for user %d", userID)
	}
}

func TestSwitchingBackToASCIMProviderSetsSCIMUpAgain(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	_, _, err := s.gateway.RotateSCIMConnectionToken(t.Context(), conn.ID)
	require.NoError(t, err)

	// Okta provisions the Owner, who enforces SCIM.
	_, providerURL := directoryStub(t)
	owner := s.signIn(providerURL, "00u-owner")
	require.Equal(t, owner.ID, s.provisionUser(conn, "00u-owner").UserID)
	_, err = s.gateway.EnforceSCIMConnection(t.Context(), conn.ID, gatewayclient.EnforceSCIMOptions{
		Actor: gatewayclient.SCIMEnforceActor{
			UserID:                owner.ID,
			AuthProviderNamespace: system.DefaultNamespace,
			AuthProviderName:      oktaProviderName,
		},
	})
	require.NoError(t, err)
	setup := setup.New(s.gateway, s.client, s.handler.dispatcher, "https://obot.example.com")

	s.switchAwayFromOkta()

	// Staging Okta again waits for its cleanup, and then sets SCIM up from scratch.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()), "still being deconfigured")
	assert.Nil(t, s.connection())
	s.runAuthProviderCleanups()
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	again := s.connection()
	require.NotNil(t, again)
	assert.NotEqual(t, conn.ID, again.ID)
	assert.Equal(t, gatewaytypes.SCIMConnectionStateConnected, again.State)
	assert.False(t, again.HasToken())

	// An Owner who joined while the other provider served sign-ins, and whom Okta never provisioned, can verify the
	// staged provider: the new connection is not enforced.
	s.signIn(providerURL, "00u-later")

	// No token is issued until the switch, which then needs nothing more.
	_, err = setup.RotateToken(t.Context(), again.ID)
	assert.ErrorContains(t, err, "not the configured auth provider")
	require.Empty(t, s.switchProvider(oktaProviderName, activeProviderName))
	assert.Equal(t, again.ID, s.connection().ID)
	issued, err := setup.RotateToken(t.Context(), again.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, issued.Token)
}

func TestSwitchingBackToAMigratedProviderSynchronizesItsDirectoryAgain(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
	require.Nil(t, s.connection())
	require.Empty(t, s.apply(v1.ProviderDesiredStateMigrated, nil))
	conn := s.connection()
	require.NotNil(t, conn)
	assert.Equal(t, gatewaytypes.SCIMConnectionOriginMigrated, conn.Origin)

	s.switchAwayFromOkta()
	assert.Nil(t, s.connection())
	s.runAuthProviderCleanups()

	// Staged with its directory parameters, Okta synchronizes its directory at sign-in, as before SCIM was enabled.
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, directorySettings()))
	assert.Nil(t, s.connection())
	requests, providerURL := directoryStub(t)
	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), providerURL), "okta/"), "access-token")
	// The stub's directory fails, which fails the sign-in once it asked.
	_, err := s.gateway.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      oktaProviderName,
		ProviderUsername:      "00u-verifier",
		ProviderUserID:        "00u-verifier",
		Email:                 "00u-verifier@example.com",
	}, "", clienttypes.RoleOwner, gatewayclient.UserLimit{
		Unlimited: true,
	})
	assert.ErrorContains(t, err, "the directory is not available")
	assert.NotZero(t, requests.Load())

	require.Empty(t, s.switchProvider(oktaProviderName, activeProviderName))
	assert.Nil(t, s.connection())
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
}

func TestReconfiguringAfterSCIMCanSynchronizeTheDirectory(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		// migrate enables SCIM for a provider configured with its directory parameters.
		migrate    bool
		wantOrigin gatewaytypes.SCIMConnectionOrigin
	}{
		{
			name:       "SCIM set up without directory parameters",
			settings:   oidcSettings(),
			wantOrigin: gatewaytypes.SCIMConnectionOriginSCIMFirst,
		},
		{
			name:       "SCIM enabled for a provider that synchronized its directory",
			settings:   directorySettings(),
			migrate:    true,
			wantOrigin: gatewaytypes.SCIMConnectionOriginMigrated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSCIMChangeTest(t)
			require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, tt.settings))
			if tt.migrate {
				require.Empty(t, s.apply(v1.ProviderDesiredStateMigrated, nil))
			}
			conn := s.connection()
			require.NotNil(t, conn)
			require.Equal(t, tt.wantOrigin, conn.Origin)

			require.Empty(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil))
			require.Nil(t, s.connection())
			s.runAuthProviderCleanups()

			// Configured with its directory parameters, Okta synchronizes its directory at sign-in again.
			require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
			assert.Nil(t, s.connection())
			assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
			assert.True(t, s.status().Configured)

			requests, providerURL := directoryStub(t)
			ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), providerURL), "okta/"), "access-token")
			// The stub's directory fails, which fails the sign-in once it asked.
			_, err := s.gateway.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
				AuthProviderNamespace: system.DefaultNamespace,
				AuthProviderName:      oktaProviderName,
				ProviderUsername:      "00u-alice",
				ProviderUserID:        "00u-alice",
				Email:                 "00u-alice@example.com",
			}, "", clienttypes.RoleBasic, gatewayclient.UserLimit{
				Unlimited: true,
			})
			assert.ErrorContains(t, err, "the directory is not available")
			assert.NotZero(t, requests.Load())
		})
	}
}

func TestASwitchThatFailedToDeleteTheSCIMConnectionFinishesWhenRetried(t *testing.T) {
	tests := []struct {
		name string
		// stagedGone deletes the staged configuration before the retry, as an attempt that failed after its last
		// step would have.
		stagedGone bool
	}{
		{
			name: "the staged configuration remains",
		},
		{
			name:       "the staged configuration is gone",
			stagedGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testRetriedSwitch(t, tt.stagedGone)
		})
	}
}

func testRetriedSwitch(t *testing.T, stagedGone bool) {
	t.Helper()
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	deactivated := s.provisionUser(conn, "00u-bob")
	s.deactivateUser(conn, deactivated)

	require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.ReplacementAuthProviderCredentialContext,
		Name:    activeProviderName,
		Secrets: map[string]string{
			activeProviderParameter: "secret",
		},
	}))
	change := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         activeProviderName,
			DesiredState:         v1.ProviderDesiredStateSwitched,
			ReplacesProviderName: oktaProviderName,
		},
	}
	require.NoError(t, s.client.Create(t.Context(), change))
	t.Cleanup(func() {
		_ = s.client.Delete(t.Context(), change)
	})
	reconcile := func() error {
		t.Helper()
		return s.handler.Reconcile(router.Request{
			Client:    s.client,
			Object:    change,
			Ctx:       t.Context(),
			Namespace: change.Namespace,
			Name:      change.Name,
		}, nil)
	}

	// The first attempt promotes the incoming provider and deletes Okta's credential, and then fails to delete Okta's
	// SCIM connection.
	s.failSCIMConnectionDeletes.Store(true)
	require.ErrorContains(t, reconcile(), "the SCIM connection could not be deleted")
	assert.Nil(t, s.credential(oktaProviderName), "the credential goes before the SCIM connection")
	require.NotNil(t, s.connection())
	configured, err := s.handler.dispatcher.GetConfiguredAuthProvider(t.Context())
	require.NoError(t, err)
	require.Equal(t, activeProviderName, configured)

	if stagedGone {
		_, err := s.gateway.DeleteCredential(t.Context(), system.ReplacementAuthProviderCredentialContext, activeProviderName)
		require.NoError(t, err)
	}

	// The retry finds the incoming provider configured, and finishes the switch.
	s.failSCIMConnectionDeletes.Store(false)
	require.NoError(t, reconcile())
	require.Empty(t, change.Status.Error)
	assert.True(t, change.Status.Applied)
	assert.Nil(t, s.connection())
	user, err := s.gateway.UserByID(t.Context(), fmt.Sprint(deactivated.UserID))
	require.NoError(t, err)
	assert.NotNil(t, user.DisabledAt, "a user that SCIM disabled was enabled")
	staged, err := s.gateway.HasCredential(t.Context(), []string{system.ReplacementAuthProviderCredentialContext}, activeProviderName)
	require.NoError(t, err)
	assert.False(t, staged, "the staged configuration was kept")
	assert.False(t, s.status().Configured)
	var cleanups v1.AuthProviderCleanupList
	require.NoError(t, s.client.List(t.Context(), &cleanups))
	require.Len(t, cleanups.Items, 1)
	assert.True(t, cleanups.Items[0].Spec.Ready)
}

func TestASwitchInterruptedWhileBothProvidersAreConfiguredFinishesWhenRetried(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	require.NotNil(t, s.connection())

	// An earlier attempt promoted the incoming provider, and failed before Okta's credential went. The incoming
	// provider is listed first, so it is the one the dispatcher reports as configured.
	secrets := map[string]string{
		activeProviderParameter: "secret",
	}
	for _, context := range []string{system.ReplacementAuthProviderCredentialContext, activeProviderName} {
		require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: context,
			Name:    activeProviderName,
			Secrets: secrets,
		}))
	}
	var incoming v1.AuthProvider
	require.NoError(t, s.client.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: activeProviderName}, &incoming))
	incoming.Status.Configured = true
	require.NoError(t, s.client.Status().Update(t.Context(), &incoming))
	configured, err := s.handler.dispatcher.GetConfiguredAuthProvider(t.Context())
	require.NoError(t, err)
	require.Equal(t, activeProviderName, configured)

	require.Empty(t, s.switchProvider(activeProviderName, oktaProviderName))
	assert.Nil(t, s.credential(oktaProviderName))
	assert.Nil(t, s.connection())
	assert.False(t, s.status().Configured)
	staged, err := s.gateway.HasCredential(t.Context(), []string{system.ReplacementAuthProviderCredentialContext}, activeProviderName)
	require.NoError(t, err)
	assert.False(t, staged, "the staged configuration was kept")
}

func TestAStaleSwitchLeavesAProviderStagedAgainAlone(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	s.switchAwayFromOkta()
	s.runAuthProviderCleanups()

	// Okta is staged again, which sets SCIM up for it, when a request for the switch that finished arrives again.
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	assert.Contains(t, s.switchProvider(activeProviderName, oktaProviderName), "is staged again")
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.NotNil(t, s.credential(system.ReplacementAuthProviderCredentialContext))
}

func TestDeconfiguringAStagedProviderIsRefused(t *testing.T) {
	s := newSCIMChangeTest(t)
	s.activateOtherProvider()
	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	// The staging is discarded instead, which deletes the connection with it. Deconfiguring would delete only the
	// connection, and leave a staging that sign-ins would serve without one.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil), "is staged as a replacement")
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.NotNil(t, s.credential(system.ReplacementAuthProviderCredentialContext))
}

func TestUnstageKeepsTheConnectionOfTheConfiguredProvider(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	// The configured provider is not staged, so an unstage of it is refused, and its connection stays.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateUnstaged, nil), "no staged configuration")
	assert.Equal(t, conn.ID, s.connection().ID)

	// Even with a staged credential, a provider with an active configuration keeps its connection.
	require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.ReplacementAuthProviderCredentialContext,
		Name:    oktaProviderName,
		Secrets: oidcSettings(),
	}))
	require.Empty(t, s.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.Equal(t, conn.ID, s.connection().ID)
	assert.True(t, s.status().Configured)
}

func TestConfigureNeverTreatsAProviderWithAnActiveConfigurationAsNew(t *testing.T) {
	s := newSCIMChangeTest(t)

	// The provider synchronizes its directory, but its stored configuration lacks a parameter, so the dispatcher does
	// not report it as the configured provider, as it would not while its credential could not be read.
	stored := directorySettings()
	delete(stored, oktaClientIDParam)
	require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: oktaProviderName,
		Name:    oktaProviderName,
		Secrets: stored,
	}))

	// Its active configuration still counts, so leaving out the directory parameters is refused rather than taken as
	// a new SCIM setup.
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()), "synchronizes its directory at sign-in")
	assert.Nil(t, s.connection())
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
}

// On main, configuring or unstaging a provider never decrypted its existing credentials, so a credential that can no
// longer be decrypted is replaced or discarded. Checking SCIM setup must not change that, or the one provider
// configuration change would retry forever and block every later one.
func TestProviderChangesNeverDecryptCredentialsTheyDoNotNeed(t *testing.T) {
	key := &keyedTransformer{
		key: "old",
	}
	s := newSCIMChangeTestWithEncryption(t, &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			{
				Group:    "obot.obot.ai",
				Resource: "credentials",
			}: key,
		},
	})
	upsert := func(context, name string, secrets map[string]string) {
		t.Helper()
		require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: context,
			Name:    name,
			Secrets: secrets,
		}))
	}

	// Okta is configured through SCIM, and settings for the other provider, which has no SCIM rules, are staged.
	// Then the encryption key changes.
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	require.NotNil(t, s.connection())
	upsert(system.ReplacementAuthProviderCredentialContext, activeProviderName, map[string]string{
		activeProviderParameter: "staged",
	})
	key.key = "new"

	// Unstaging the provider without SCIM rules discards its staged settings without reading them.
	unstage := &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: activeProviderName,
			DesiredState: v1.ProviderDesiredStateUnstaged,
		},
	}
	require.NoError(t, s.client.Create(t.Context(), unstage))
	require.NoError(t, s.handler.Reconcile(router.Request{
		Client:    s.client,
		Object:    unstage,
		Ctx:       t.Context(),
		Namespace: unstage.Namespace,
		Name:      unstage.Name,
	}, nil))
	require.Empty(t, unstage.Status.Error)
	require.NoError(t, s.client.Delete(t.Context(), unstage))
	staged, err := s.gateway.HasCredential(t.Context(), []string{system.ReplacementAuthProviderCredentialContext}, activeProviderName)
	require.NoError(t, err)
	assert.False(t, staged)

	// Reconfiguring Okta, which has its connection, replaces its credential without reading the old one.
	upsert(oktaProviderName, oktaProviderName, oidcSettings())
	key.key = "newer"
	assert.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	assert.Equal(t, "oidc-client", s.credential(oktaProviderName)[oktaClientIDParam])
}
