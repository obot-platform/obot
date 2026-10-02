package providerconfigurationchange

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	gateway := newSCIMChangeTestGateway(t, client, encryption)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)

	return &scimChangeTest{
		t:       t,
		client:  client,
		gateway: gateway,
		handler: New(gateway, dispatcher.New(nil, client, gateway, licenseProvider, "", "", ""), licenseProvider, "", client),
	}
}

// newSCIMChangeTestGateway returns a gateway client over a new in-memory database, which records the controller
// objects that sign-ins create in storage.
func newSCIMChangeTestGateway(t *testing.T, storage kclient.Client, encryption *encryptionconfig.EncryptionConfiguration) *gatewayclient.Client {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, storage, encryption, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
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

	// Configuring the provider again resumes the connection, and never stores the directory parameters.
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

	// Providing only some of them is refused too.
	partial := oidcSettings()
	partial[oktaServiceClientParam] = "service-client"
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, partial), oktaServiceKeyParam)
}

func TestConfigureRefusesPartialDirectoryParameters(t *testing.T) {
	s := newSCIMChangeTest(t)

	partial := oidcSettings()
	partial[oktaServiceKeyParam] = "service-key"
	assert.Contains(t, s.apply(v1.ProviderDesiredStateConfigured, partial), "none of them")
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

func TestConfigureRefusesGroupDataInTheGateway(t *testing.T) {
	s := newSCIMChangeTest(t)
	_, err := s.gateway.CreateGroupRoleAssignment(t.Context(), "okta/00g-admins", clienttypes.RoleAdmin, "")
	require.NoError(t, err)

	errMsg := s.apply(v1.ProviderDesiredStateConfigured, oidcSettings())
	assert.Contains(t, errMsg, "okta/00g-admins")
	assert.Contains(t, errMsg, "group role assignment")
	assert.Nil(t, s.connection())
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

func TestUnstageKeepsAConnectionThatWasUsed(t *testing.T) {
	s := newSCIMChangeTest(t)
	s.activateOtherProvider()

	// The provider was configured through SCIM earlier, and provisioned a user, before another provider replaced it.
	conn, _, err := s.gateway.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      oktaProviderName,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	require.NoError(t, err)
	_, err = s.gateway.CreateSCIMUser(t.Context(), conn, gatewayclient.SCIMUserInput{
		UserName:   "user@example.com",
		ExternalID: "00u-user",
	}, gatewayclient.SCIMUserCreateOptions{
		UserLimit: gatewayclient.UserLimit{
			Unlimited: true,
		},
		DefaultRole: clienttypes.RoleBasic,
	})
	require.NoError(t, err)

	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	require.Empty(t, s.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.Equal(t, conn.ID, s.connection().ID)
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

func TestDeconfiguringSuspendsTheConnectionUntilTheProviderIsConfiguredAgain(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	assert.Nil(t, conn.SuspendedAt)

	// The connection and its data survive deconfiguration, but its groups grant nothing until the provider is
	// configured again.
	require.Empty(t, s.apply(v1.ProviderDesiredStateDeconfigured, nil))
	suspended := s.connection()
	require.NotNil(t, suspended)
	assert.Equal(t, conn.ID, suspended.ID)
	assert.NotNil(t, suspended.SuspendedAt)

	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	resumed := s.connection()
	require.NotNil(t, resumed)
	assert.Equal(t, conn.ID, resumed.ID)
	assert.Nil(t, resumed.SuspendedAt)
}

func TestSwitchingBackToASCIMProviderResumesItsConnection(t *testing.T) {
	s := newSCIMChangeTest(t)
	s.activateOtherProvider()

	require.Empty(t, s.apply(v1.ProviderDesiredStateStaged, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)
	// As an earlier switch away from the provider would have left it.
	require.NoError(t, s.gateway.SuspendSCIMConnection(t.Context(), system.DefaultNamespace, oktaProviderName))

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

	resumed := s.connection()
	require.NotNil(t, resumed)
	assert.Equal(t, conn.ID, resumed.ID)
	assert.Nil(t, resumed.SuspendedAt)
}

func TestUnstageKeepsTheConnectionOfTheConfiguredProvider(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	conn := s.connection()
	require.NotNil(t, conn)

	// The configured provider is not staged, so an unstage of it is refused, and its unused connection stays.
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
