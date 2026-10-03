package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oktaServiceClientIDParam   = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"
	oktaServicePrivateKeyParam = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"
	oktaIssuerParam            = "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
)

type authProviderSCIMTest struct {
	t          *testing.T
	provider   *v1.AuthProvider
	storage    kclient.WithWatch
	gateway    *gclient.Client
	db         *gorm.DB
	license    *license.Provider
	dispatcher *dispatcher.Dispatcher
}

func newAuthProviderSCIMTest(t *testing.T) *authProviderSCIMTest {
	t.Helper()

	provider := &v1.AuthProvider{
		Name:      "okta-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: oktaIssuerParam,
						},
						{
							Name:         oktaServiceClientIDParam,
							FriendlyName: "API Services Client ID",
						},
						{
							Name:         oktaServicePrivateKeyParam,
							FriendlyName: "API Services Private Key",
						},
					},
				},
				GroupIDPrefix: "okta/",
			},
		},
	}
	storage := newAuthProviderTestStorage(provider)
	gateway, db := newHandlerTestGatewayWithDB(t)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)

	return &authProviderSCIMTest{
		t:          t,
		provider:   provider,
		storage:    storage,
		gateway:    gateway,
		db:         db,
		license:    licenseProvider,
		dispatcher: dispatcher.New(nil, storage, gateway, licenseProvider, "", "", ""),
	}
}

func (s *authProviderSCIMTest) handler() *AuthProviderHandler {
	return NewAuthProviderHandler(s.dispatcher, "", s.license)
}

func (s *authProviderSCIMTest) context(method, path, body string, groups ...string) (api.Context, *httptest.ResponseRecorder) {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.SetPathValue("id", s.provider.Name)
	rec := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: rec,
		Request:        request,
		Storage:        s.storage,
		GatewayClient:  s.gateway,
		User: &user.DefaultInfo{
			Name:   "caller",
			Groups: groups,
		},
	}, rec
}

func (s *authProviderSCIMTest) connect() {
	s.t.Helper()

	_, _, err := s.gateway.CreateSCIMConnection(s.t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: s.provider.Namespace,
		AuthProviderName:      s.provider.Name,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginMigrated,
	})
	require.NoError(s.t, err)
}

func (s *authProviderSCIMTest) storeCredential(secrets map[string]string) {
	s.t.Helper()

	require.NoError(s.t, s.gateway.UpsertCredential(s.t.Context(), gatewaytypes.Credential{
		Context: s.provider.Name,
		Name:    s.provider.Name,
		Secrets: secrets,
	}))
}

// requireNoChange fails unless no provider configuration change was submitted.
func (s *authProviderSCIMTest) requireNoChange() {
	s.t.Helper()

	var changes v1.ProviderConfigurationChangeList
	require.NoError(s.t, s.storage.List(s.t.Context(), &changes))
	require.Empty(s.t, changes.Items)
}

// configure submits a configuration and returns the credential that it staged, once the change is submitted.
func (s *authProviderSCIMTest) configure(body string) map[string]string {
	s.t.Helper()

	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", body)
	errC := make(chan error, 1)
	go func() {
		errC <- s.handler().Configure(req)
	}()

	var change v1.ProviderConfigurationChange
	require.EventuallyWithT(s.t, func(collect *assert.CollectT) {
		assert.NoError(collect, s.storage.Get(s.t.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.ProviderChangeAuthName,
		}, &change))
	}, time.Second, 10*time.Millisecond)

	staged, err := s.gateway.RevealCredential(s.t.Context(), []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
	require.NoError(s.t, err)

	require.NoError(s.t, s.storage.Delete(s.t.Context(), &change))
	require.NoError(s.t, <-errC)
	return staged.Secrets
}

func TestConfigureValidatesTheEffectiveParameters(t *testing.T) {
	s := newAuthProviderSCIMTest(t)

	// A provider that is not configured and has no connection may omit the directory parameters, which sets it up
	// for SCIM, but only all of them at once.
	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", `{"`+oktaIssuerParam+`":"https://example.okta.com","`+oktaServiceClientIDParam+`":"client"}`)
	err := s.handler().Configure(req)
	require.ErrorContains(t, err, "none of them")
	require.ErrorContains(t, err, oktaServicePrivateKeyParam)
	s.requireNoChange()

	staged := s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com"}`)
	assert.Equal(t, "https://example.okta.com", staged[oktaIssuerParam])
	assert.NotContains(t, staged, oktaServiceClientIDParam)
	assert.NotContains(t, staged, oktaServicePrivateKeyParam)

	// Providing them sets up directory synchronization.
	staged = s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com","` + oktaServiceClientIDParam + `":"client","` + oktaServicePrivateKeyParam + `":"key"}`)
	assert.Equal(t, "client", staged[oktaServiceClientIDParam])

	// Once the provider is configured and synchronizes its directory, they are required, and removing them is
	// refused before anything is staged.
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})
	req, _ = s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", `{"`+oktaIssuerParam+`":"https://example.okta.com"}`)
	err = s.handler().Configure(req)
	require.ErrorContains(t, err, "synchronizes its directory at sign-in")
	require.ErrorContains(t, err, oktaServiceClientIDParam)
	s.requireNoChange()

	// With a connection whose stored credential lacks them, they are not required, and are dropped if submitted.
	s.storeCredential(map[string]string{
		oktaIssuerParam: "https://example.okta.com",
	})
	s.connect()
	staged = s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com","` + oktaServiceClientIDParam + `":"client"}`)
	assert.Equal(t, "https://example.okta.com", staged[oktaIssuerParam])
	assert.NotContains(t, staged, oktaServiceClientIDParam)
}

func TestConfigureWithoutDirectoryParametersRefusesResidualGroupData(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	_, err := s.gateway.CreateGroupRoleAssignment(t.Context(), "okta/00g-legacy", clienttypes.RoleAdmin, "")
	require.NoError(t, err)
	require.NoError(t, s.storage.Create(t.Context(), &v1.ModelAccessPolicy{
		Name:      "models",
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: clienttypes.ModelAccessPolicyManifest{
				DisplayName: "Models",
				Subjects: []clienttypes.Subject{
					{
						Type: clienttypes.SubjectTypeGroup,
						ID:   "okta/00g-legacy",
					},
				},
			},
		},
	}))

	// The refusal lists what remains, and what references it, before anything is staged.
	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", `{"`+oktaIssuerParam+`":"https://example.okta.com"}`)
	err = s.handler().Configure(req)
	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.Code)
	assert.Contains(t, httpErr.Message, "okta/00g-legacy")
	assert.Contains(t, httpErr.Message, `model access policy "Models"`)
	assert.Contains(t, httpErr.Message, "group role assignment")
	s.requireNoChange()

	// The administrator can list it, and provide the directory credentials instead.
	get, rec := s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider/residual-group-data", "", clienttypes.GroupAdmin)
	require.NoError(t, s.handler().ResidualGroupData(get))
	var residual clienttypes.ResidualGroupData
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &residual))
	require.Len(t, residual.Groups, 1)
	assert.Equal(t, "okta/00g-legacy", residual.Groups[0].ID)
	assert.Len(t, residual.Groups[0].References, 2)

	staged := s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com","` + oktaServiceClientIDParam + `":"client","` + oktaServicePrivateKeyParam + `":"key"}`)
	assert.Equal(t, "client", staged[oktaServiceClientIDParam])
}

func TestStagingTheActiveProviderIsRefused(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	// Okta serves sign-ins and synchronizes its directory, so its groups are in use, not left from an earlier
	// configuration.
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})
	_, err := s.gateway.CreateGroupRoleAssignment(t.Context(), "okta/00g-team", clienttypes.RoleAdmin, "")
	require.NoError(t, err)

	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/stage", `{"`+oktaIssuerParam+`":"https://example.okta.com"}`)
	err = s.handler().Stage(req)
	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Message, "already the active authentication provider")
	s.requireNoChange()
}

// TestSignInPageReadsSkipSCIM checks that reads of auth providers by anyone but administrators and auditors, such as
// the sign-in page's, neither read SCIM data nor fail when it cannot be read.
func TestSignInPageReadsSkipSCIM(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.connect()
	require.NoError(t, s.db.Migrator().DropTable(new(gatewaytypes.SCIMConnection)))

	for _, groups := range [][]string{
		nil,
		{clienttypes.GroupBasic, clienttypes.GroupAuthenticated},
	} {
		req, _ := s.context(http.MethodGet, "/api/auth-providers", "", groups...)
		require.NoError(t, s.handler().List(req), "list as %v", groups)
		req, _ = s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", groups...)
		require.NoError(t, s.handler().ByID(req), "get as %v", groups)
	}

	// Administrators are served what depends on SCIM, so their reads still need it.
	req, _ := s.context(http.MethodGet, "/api/auth-providers", "", clienttypes.GroupAdmin)
	require.Error(t, s.handler().List(req))
	req, _ = s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", clienttypes.GroupAdmin)
	require.Error(t, s.handler().ByID(req))
}

func TestConfigureKeepsDirectoryParametersThatAreStillStored(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})
	s.connect()

	// Only one of them is refused, because the Okta provider would not start with it.
	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", `{"`+oktaIssuerParam+`":"https://example.okta.com","`+oktaServiceClientIDParam+`":"client"}`)
	err := s.handler().Configure(req)
	require.ErrorContains(t, err, "none of them")
	require.ErrorContains(t, err, oktaServicePrivateKeyParam)

	// Once a connection exists, the stored directory parameters are optional: they can be kept or removed.
	kept := s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com","` + oktaServiceClientIDParam + `":"client","` + oktaServicePrivateKeyParam + `":"key"}`)
	assert.Equal(t, "client", kept[oktaServiceClientIDParam])
	removed := s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com"}`)
	assert.NotContains(t, removed, oktaServiceClientIDParam)
	assert.NotContains(t, removed, oktaServicePrivateKeyParam)
}

func TestAuthProviderServesEffectiveParametersAndSCIMState(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})

	get := func(groups ...string) clienttypes.AuthProvider {
		t.Helper()
		req, rec := s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", groups...)
		require.NoError(t, s.handler().ByID(req))
		var provider clienttypes.AuthProvider
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provider))
		return provider
	}
	names := func(params []clienttypes.ProviderConfigurationParameter) []string {
		out := make([]string, 0, len(params))
		for _, p := range params {
			out = append(out, p.Name)
		}
		return out
	}

	// Directory synchronization serves the manifest's parameters.
	before := get(clienttypes.GroupAdmin)
	assert.Equal(t, []string{oktaIssuerParam, oktaServiceClientIDParam, oktaServicePrivateKeyParam}, names(before.RequiredConfigurationParameters))
	assert.Empty(t, before.SCIMState)

	// With a connection, the stored directory parameters are served as optional and unused, and only administrators
	// learn the SCIM state.
	s.connect()
	after := get(clienttypes.GroupAdmin)
	assert.Equal(t, []string{oktaIssuerParam}, names(after.RequiredConfigurationParameters))
	assert.Equal(t, []string{oktaServiceClientIDParam, oktaServicePrivateKeyParam}, names(after.OptionalConfigurationParameters))
	for _, p := range after.OptionalConfigurationParameters {
		assert.Contains(t, p.Description, "Not used")
		assert.NotEmpty(t, p.FriendlyName)
	}
	assert.Equal(t, string(gatewaytypes.SCIMConnectionStateConnected), after.SCIMState)
	assert.Empty(t, get().SCIMState)

	// Once the connection has a token, administrators learn when it expires, so the layout can warn Owners.
	assert.Nil(t, after.SCIMTokenExpiresAt)
	conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.provider.Namespace, s.provider.Name)
	require.NoError(t, err)
	rotated, _, err := s.gateway.RotateSCIMConnectionToken(t.Context(), conn.ID)
	require.NoError(t, err)
	withToken := get(clienttypes.GroupAdmin)
	require.NotNil(t, withToken.SCIMTokenExpiresAt)
	assert.WithinDuration(t, *rotated.TokenExpiresAt(), withToken.SCIMTokenExpiresAt.Time, time.Second)
	assert.Nil(t, get().SCIMTokenExpiresAt)
}

func TestAuthProviderServesTheSetupChoice(t *testing.T) {
	s := newAuthProviderSCIMTest(t)

	req, rec := s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", clienttypes.GroupAdmin)
	require.NoError(t, s.handler().ByID(req))
	var provider clienttypes.AuthProvider
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provider))

	// A provider that is not configured may omit the directory parameters, and the form describes the choice they
	// make.
	require.Len(t, provider.RequiredConfigurationParameters, 1)
	assert.Equal(t, oktaIssuerParam, provider.RequiredConfigurationParameters[0].Name)
	require.Len(t, provider.OptionalConfigurationParameters, 2)
	for _, p := range provider.OptionalConfigurationParameters {
		assert.Contains(t, p.Description, "SCIM instead")
		assert.NotEmpty(t, p.FriendlyName)
	}
	require.NotNil(t, provider.SCIM)
	assert.Equal(t, []string{oktaServiceClientIDParam, oktaServicePrivateKeyParam}, provider.SCIM.DirectoryParameters)
	assert.Equal(t, oktaIssuerParam, provider.SCIM.IssuerParameter)
	assert.Empty(t, provider.SCIM.ConnectionIssuer)

	// A connection records the issuer, which the form compares a changed Org URL with.
	_, _, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: s.provider.Namespace,
		AuthProviderName:      s.provider.Name,
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	require.NoError(t, err)
	req, rec = s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", clienttypes.GroupAdmin)
	require.NoError(t, s.handler().ByID(req))
	provider = clienttypes.AuthProvider{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provider))
	assert.Empty(t, provider.OptionalConfigurationParameters)
	require.NotNil(t, provider.SCIM)
	assert.Equal(t, "https://example.okta.com", provider.SCIM.ConnectionIssuer)

	// Anyone else learns nothing about SCIM: not the state, and not the parameters the connection relaxes.
	req, rec = s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "")
	require.NoError(t, s.handler().ByID(req))
	provider = clienttypes.AuthProvider{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provider))
	assert.Nil(t, provider.SCIM)
	assert.Empty(t, provider.SCIMState)
	require.Len(t, provider.RequiredConfigurationParameters, 3)
	for _, p := range provider.RequiredConfigurationParameters {
		assert.NotContains(t, p.Description, "SCIM")
	}
	assert.Empty(t, provider.OptionalConfigurationParameters)

	// Auditors see what administrators see.
	req, rec = s.context(http.MethodGet, "/api/auth-providers/okta-auth-provider", "", clienttypes.GroupAuditor)
	require.NoError(t, s.handler().ByID(req))
	provider = clienttypes.AuthProvider{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provider))
	assert.NotNil(t, provider.SCIM)
	assert.Equal(t, string(gatewaytypes.SCIMConnectionStateConnected), provider.SCIMState)
}
