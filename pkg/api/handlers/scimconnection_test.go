package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	scimsetup "github.com/obot-platform/obot/pkg/scim/setup"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *authProviderSCIMTest) scimContext(method, path, body string, principal *user.DefaultInfo) (api.Context, *httptest.ResponseRecorder) {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: rec,
		Request:        request,
		Storage:        s.storage,
		GatewayClient:  s.gateway,
		User:           principal,
	}, rec
}

func TestSCIMConnectionHandlerRoles(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	conn, _, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: s.provider.Namespace,
		AuthProviderName:      s.provider.Name,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	require.NoError(t, err)
	h := NewSCIMConnectionHandler(scimsetup.New(s.gateway, s.storage, s.dispatcher, "https://obot.example.com"))

	bootstrap := &user.DefaultInfo{
		Name:   system.BootstrapName,
		UID:    "1",
		Groups: clienttypes.RoleOwner.Groups(),
		Extra: map[string][]string{
			"auth_provider_name": {system.BootstrapName},
		},
	}
	admin := &user.DefaultInfo{
		Name:   "admin",
		UID:    "2",
		Groups: clienttypes.RoleAdmin.Groups(),
		Extra: map[string][]string{
			"auth_provider_name":      {s.provider.Name},
			"auth_provider_namespace": {s.provider.Namespace},
		},
	}

	// The bootstrap user issues the first token, so the identity provider can be set up before any Owner exists.
	req, rec := s.scimContext(http.MethodPost, "/api/scim-connections/"+conn.ID+"/rotate-token", "", bootstrap)
	req.SetPathValue("id", conn.ID)
	require.NoError(t, h.RotateToken(req))
	var issued clienttypes.SCIMConnection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &issued))
	assert.True(t, issued.HasToken)
	assert.NotEmpty(t, issued.Token)
	assert.Equal(t, "https://obot.example.com/scim/v2/"+conn.ID, issued.BaseURL)

	// Neither the bootstrap user nor an administrator can enforce, and an administrator cannot manage the token.
	for _, principal := range []*user.DefaultInfo{bootstrap, admin} {
		req, _ := s.scimContext(http.MethodPost, "/api/scim-connections/"+conn.ID+"/enforce", "", principal)
		req.SetPathValue("id", conn.ID)
		var httpErr *clienttypes.ErrHTTP
		require.ErrorAs(t, h.Enforce(req), &httpErr)
		assert.Equal(t, http.StatusForbidden, httpErr.Code)
	}
	req, _ = s.scimContext(http.MethodPost, "/api/scim-connections/"+conn.ID+"/revoke-current-token", "", admin)
	req.SetPathValue("id", conn.ID)
	require.Error(t, h.RevokeCurrentToken(req))

	// Administrators can review the connection, and the review never carries a token.
	req, rec = s.scimContext(http.MethodGet, "/api/scim-connections/"+conn.ID+"/review", "", admin)
	req.SetPathValue("id", conn.ID)
	require.NoError(t, h.Review(req))
	var review clienttypes.SCIMConnectionReview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &review))
	assert.Empty(t, review.Connection.Token)
	assert.True(t, review.Connection.HasToken)
	assert.NotEmpty(t, review.EnforceBlockers)

	req, rec = s.scimContext(http.MethodGet, "/api/scim-connections", "", admin)
	require.NoError(t, h.List(req))
	var list clienttypes.SCIMConnectionList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Empty(t, list.Items[0].Token)

	// Unknown connections are not found.
	req, _ = s.scimContext(http.MethodGet, "/api/scim-connections/unknown/review", "", admin)
	req.SetPathValue("id", "unknown")
	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, h.Review(req), &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestNewGroupReferencesMustNameAGroupOfTheSCIMProvider(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	h := &ModelAccessPolicyHandler{}
	owner := &user.DefaultInfo{
		Name:   "owner",
		UID:    "1",
		Groups: clienttypes.RoleOwner.Groups(),
	}
	manifest := func(groupIDs ...string) string {
		m := clienttypes.ModelAccessPolicyManifest{
			DisplayName: "Models",
			Models: []clienttypes.ModelResource{
				{
					ID: "*",
				},
			},
		}
		for _, id := range groupIDs {
			m.Subjects = append(m.Subjects, clienttypes.Subject{
				Type: clienttypes.SubjectTypeGroup,
				ID:   id,
			})
		}
		body, err := json.Marshal(m)
		require.NoError(t, err)
		return string(body)
	}

	// Without a connection, the provider's groups are discovered at sign-in, so any ID is accepted.
	req, rec := s.scimContext(http.MethodPost, "/api/model-access-policies", manifest("okta/00g-before"), owner)
	require.NoError(t, h.Create(req))
	var created clienttypes.ModelAccessPolicy
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	s.connect()

	// With one, a new reference must name an existing group of the provider.
	req, _ = s.scimContext(http.MethodPost, "/api/model-access-policies", manifest("okta/00g-missing"), owner)
	err := h.Create(req)
	require.ErrorContains(t, err, "okta/00g-missing")
	require.ErrorContains(t, err, "push the group")
	var policies v1.ModelAccessPolicyList
	require.NoError(t, s.storage.List(t.Context(), &policies))
	assert.Len(t, policies.Items, 1, "a refused policy was saved")

	// An existing reference to a missing group does not block an unrelated edit, and groups of other providers are
	// not checked.
	req, _ = s.scimContext(http.MethodPut, "/api/model-access-policies/"+created.ID, manifest("okta/00g-before", "entra/engineering"), owner)
	req.SetPathValue("id", created.ID)
	require.NoError(t, h.Update(req))

	var policy v1.ModelAccessPolicy
	require.NoError(t, s.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}, &policy))
	assert.Len(t, policy.Spec.Manifest.Subjects, 2)
}
