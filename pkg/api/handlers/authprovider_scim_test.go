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
	gateway := newHandlerTestGateway(t)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	require.NoError(t, err)

	return &authProviderSCIMTest{
		t:          t,
		provider:   provider,
		storage:    storage,
		gateway:    gateway,
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

	// Without a connection, the directory parameters are required, and the server refuses a configuration without
	// them before anything is staged.
	req, _ := s.context(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", `{"`+oktaIssuerParam+`":"https://example.okta.com"}`)
	err := s.handler().Configure(req)
	require.ErrorContains(t, err, "missing required configuration parameters")
	require.ErrorContains(t, err, oktaServiceClientIDParam)
	var changes v1.ProviderConfigurationChangeList
	require.NoError(t, s.storage.List(t.Context(), &changes))
	require.Empty(t, changes.Items)

	// With a connection whose stored credential lacks them, they are not required, and are dropped if submitted.
	s.storeCredential(map[string]string{
		oktaIssuerParam: "https://example.okta.com",
	})
	s.connect()
	staged := s.configure(`{"` + oktaIssuerParam + `":"https://example.okta.com","` + oktaServiceClientIDParam + `":"client"}`)
	assert.Equal(t, "https://example.okta.com", staged[oktaIssuerParam])
	assert.NotContains(t, staged, oktaServiceClientIDParam)
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
}
