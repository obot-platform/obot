package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/providerconfigurationchange"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	scimsetup "github.com/obot-platform/obot/pkg/scim/setup"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
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

// settleChange waits for the handler to submit the auth provider configuration change, applies it with apply, and
// settles it with the status that apply returns until the handler returns what errC receives. The status is written
// until the handler sees it, because the handler may start watching the change after the first write.
func (s *authProviderSCIMTest) settleChange(errC <-chan error, apply func(*v1.ProviderConfigurationChange) v1.ProviderConfigurationChangeStatus) error {
	s.t.Helper()

	var change v1.ProviderConfigurationChange
	require.EventuallyWithT(s.t, func(collect *assert.CollectT) {
		assert.NoError(collect, s.storage.Get(s.t.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.ProviderChangeAuthName,
		}, &change))
	}, time.Second, 10*time.Millisecond)
	status := apply(&change)

	deadline := time.After(5 * time.Second)
	for {
		if err := s.storage.Get(s.t.Context(), kclient.ObjectKey{Namespace: change.Namespace, Name: change.Name}, &change); err == nil {
			change.Status = status
			require.NoError(s.t, s.storage.Update(s.t.Context(), &change))
		}
		select {
		case err := <-errC:
			return err
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			s.t.Fatal("the handler did not return once the change settled")
		}
	}
}

// reconcileChange applies a provider configuration change with the provider configuration change controller, and
// returns the status it records.
func (s *authProviderSCIMTest) reconcileChange(change *v1.ProviderConfigurationChange) v1.ProviderConfigurationChangeStatus {
	s.t.Helper()

	controller := providerconfigurationchange.New(s.gateway, s.dispatcher, s.license, "", s.storage)
	require.NoError(s.t, controller.Reconcile(router.Request{
		Client:    s.storage,
		Object:    change,
		Ctx:       s.t.Context(),
		Namespace: change.Namespace,
		Name:      change.Name,
	}, nil))
	return change.Status
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
	// The provider was configured without directory credentials.
	s.storeCredential(map[string]string{
		oktaIssuerParam: "https://example.okta.com",
	})
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
	assert.Equal(t, "https://obot.example.com/scim/v2", issued.BaseURL)

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

// TestHandlersRefuseNewReferencesToMissingSCIMGroups checks that every handler that saves group subjects runs them
// through the group reference guard.
func TestHandlersRefuseNewReferencesToMissingSCIMGroups(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.connect()
	require.NoError(t, s.storage.Create(t.Context(), &v1.MCPCatalog{
		Name:      "default",
		Namespace: system.DefaultNamespace,
	}))
	owner := &user.DefaultInfo{
		Name:   "owner",
		UID:    "1",
		Groups: clienttypes.RoleOwner.Groups(),
	}
	missing := []clienttypes.Subject{
		{
			Type: clienttypes.SubjectTypeGroup,
			ID:   "okta/00g-missing",
		},
	}

	tests := []struct {
		name       string
		path       string
		pathValues map[string]string
		body       any
		create     func(api.Context) error
		// saved lists the objects of the kind the handler saves, which must stay empty.
		saved kclient.ObjectList
	}{
		{
			name: "access control rule",
			path: "/api/mcp-catalogs/default/access-control-rules",
			pathValues: map[string]string{
				"catalog_id": "default",
			},
			body: clienttypes.AccessControlRuleManifest{
				DisplayName: "Rule",
				Subjects:    missing,
			},
			create: (&AccessControlRuleHandler{}).Create,
			saved:  &v1.AccessControlRuleList{},
		},
		{
			name: "hosted agent access rule",
			path: "/api/hosted-agent-access-rules",
			body: clienttypes.HostedAgentAccessRuleManifest{
				DisplayName: "Rule",
				Subjects:    missing,
				Resources: []clienttypes.HostedAgentResource{
					{
						Type: clienttypes.HostedAgentResourceTypeSelector,
						ID:   "*",
					},
				},
			},
			create: (&HostedAgentAccessRuleHandler{}).Create,
			saved:  &v1.HostedAgentAccessRuleList{},
		},
		{
			name: "message policy",
			path: "/api/message-policies",
			body: clienttypes.MessagePolicyManifest{
				DisplayName: "Policy",
				Definition:  "Be polite.",
				Direction:   clienttypes.PolicyDirectionUserMessage,
				Subjects:    missing,
			},
			create: (&MessagePolicyHandler{}).Create,
			saved:  &v1.MessagePolicyList{},
		},
		{
			name: "model access policy",
			path: "/api/model-access-policies",
			body: clienttypes.ModelAccessPolicyManifest{
				DisplayName: "Models",
				Subjects:    missing,
				Models: []clienttypes.ModelResource{
					{
						ID: "*",
					},
				},
			},
			create: (&ModelAccessPolicyHandler{}).Create,
			saved:  &v1.ModelAccessPolicyList{},
		},
		{
			name: "skill access rule",
			path: "/api/skill-access-rules",
			body: clienttypes.SkillAccessRuleManifest{
				DisplayName: "Rule",
				Subjects:    missing,
				Resources: []clienttypes.SkillResource{
					{
						Type: clienttypes.SkillResourceTypeSelector,
						ID:   "*",
					},
				},
			},
			create: (&SkillAccessRuleHandler{}).Create,
			saved:  &v1.SkillAccessRuleList{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			require.NoError(t, err)
			req, _ := s.scimContext(http.MethodPost, tt.path, string(body), owner)
			for name, value := range tt.pathValues {
				req.SetPathValue(name, value)
			}

			err = tt.create(req)
			require.ErrorContains(t, err, "okta/00g-missing")
			require.ErrorContains(t, err, "push the group")
			require.NoError(t, s.storage.List(t.Context(), tt.saved))
			assert.Zero(t, meta.LenList(tt.saved), "a refused object was saved")
		})
	}
}

func TestSCIMEnableHandler(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})
	h := NewSCIMConnectionHandler(scimsetup.New(s.gateway, s.storage, s.dispatcher, "https://obot.example.com"))

	owner := &user.DefaultInfo{
		Name:   "owner",
		UID:    "1",
		Groups: clienttypes.RoleOwner.Groups(),
	}
	admin := &user.DefaultInfo{
		Name:   "admin",
		UID:    "2",
		Groups: clienttypes.RoleAdmin.Groups(),
	}

	// Administrators can preview enabling SCIM for the provider, which synchronizes its directory.
	req, rec := s.scimContext(http.MethodGet, "/api/scim-connections/enable-preview", "", admin)
	require.NoError(t, h.EnablePreview(req))
	var preview clienttypes.SCIMEnablePreview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	assert.Equal(t, s.provider.Name, preview.AuthProviderName)
	assert.Empty(t, preview.Blockers)

	// Only Owners can enable it.
	req, _ = s.scimContext(http.MethodPost, "/api/scim-connections", "", admin)
	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, h.Enable(req), &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.Code)
	s.requireNoChange()

	req, rec = s.scimContext(http.MethodPost, "/api/scim-connections", "", owner)
	errC := make(chan error, 1)
	go func() {
		errC <- h.Enable(req)
	}()

	// The connection is created by a provider configuration change, which the controller applies.
	require.NoError(t, s.settleChange(errC, func(change *v1.ProviderConfigurationChange) v1.ProviderConfigurationChangeStatus {
		assert.Equal(t, v1.ProviderDesiredStateMigrated, change.Spec.DesiredState)
		assert.Equal(t, s.provider.Name, change.Spec.ProviderName)
		assert.Empty(t, change.Spec.StagedCredentialName)
		return s.reconcileChange(change)
	}))
	require.NoError(t, s.storage.Delete(t.Context(), &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: system.DefaultNamespace,
	}))

	// The response carries the token, once.
	var result clienttypes.SCIMEnableResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.NotEmpty(t, result.Connection.Token)
	assert.Equal(t, string(gatewaytypes.SCIMConnectionOriginMigrated), result.Connection.Origin)
	assert.Equal(t, "https://obot.example.com/scim/v2", result.Connection.BaseURL)
	assert.Empty(t, result.DeletionError)

	// Enabling again is blocked before any change is submitted.
	req, _ = s.scimContext(http.MethodPost, "/api/scim-connections", "", owner)
	err := h.Enable(req)
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Message, "already provisions users and groups through SCIM")
	s.requireNoChange()

	// Only Owners can delete the unreferenced groups.
	req, _ = s.scimContext(http.MethodPost, "/api/scim-connections/"+result.Connection.ID+"/delete-unreferenced-groups", "", admin)
	req.SetPathValue("id", result.Connection.ID)
	require.ErrorAs(t, h.DeleteUnreferencedGroups(req), &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.Code)
	req, rec = s.scimContext(http.MethodPost, "/api/scim-connections/"+result.Connection.ID+"/delete-unreferenced-groups", "", owner)
	req.SetPathValue("id", result.Connection.ID)
	require.NoError(t, h.DeleteUnreferencedGroups(req))
	var deleted clienttypes.SCIMGroupDeletionResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &deleted))
	assert.Zero(t, deleted.DeletedGroupCount)
}

func TestSCIMEnableHandlerReportsTheControllersRefusal(t *testing.T) {
	s := newAuthProviderSCIMTest(t)
	s.storeCredential(map[string]string{
		oktaIssuerParam:            "https://example.okta.com",
		oktaServiceClientIDParam:   "client",
		oktaServicePrivateKeyParam: "key",
	})
	h := NewSCIMConnectionHandler(scimsetup.New(s.gateway, s.storage, s.dispatcher, "https://obot.example.com"))
	owner := &user.DefaultInfo{
		Name:   "owner",
		UID:    "1",
		Groups: clienttypes.RoleOwner.Groups(),
	}

	req, _ := s.scimContext(http.MethodPost, "/api/scim-connections", "", owner)
	errC := make(chan error, 1)
	go func() {
		errC <- h.Enable(req)
	}()

	// The controller finds a blocker under the serialization of provider configuration changes, such as a switch
	// staged after the preview was read.
	err := s.settleChange(errC, func(change *v1.ProviderConfigurationChange) v1.ProviderConfigurationChangeStatus {
		require.NoError(t, s.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: system.ReplacementAuthProviderCredentialContext,
			Name:    "github-auth-provider",
			Secrets: map[string]string{
				"GITHUB_CLIENT_SECRET": "secret",
			},
		}))
		return s.reconcileChange(change)
	})

	var httpErr *clienttypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Message, "is staged")
	conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.provider.Namespace, s.provider.Name)
	require.NoError(t, err)
	assert.Nil(t, conn)
}

func TestWaitForProviderConfigurationChangeAnswersWithTheRecordedStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   v1.ProviderConfigurationChangeStatus
		wantCode int
	}{
		{
			name: "a refusal without a status is a bad request",
			status: v1.ProviderConfigurationChangeStatus{
				Error: "refused",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "a refusal with a status is answered with it",
			status: v1.ProviderConfigurationChangeStatus{
				Error:     "the provider still has group data",
				ErrorCode: http.StatusConflict,
			},
			wantCode: http.StatusConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAuthProviderSCIMTest(t)
			change := &v1.ProviderConfigurationChange{
				Name:      system.ProviderChangeAuthName,
				Namespace: system.DefaultNamespace,
				Status:    tt.status,
			}
			require.NoError(t, s.storage.Create(t.Context(), change))

			req, _ := s.scimContext(http.MethodPost, "/api/auth-providers/okta-auth-provider/configure", "", nil)
			var httpErr *clienttypes.ErrHTTP
			require.ErrorAs(t, waitForProviderConfigurationChange(req, change), &httpErr)
			assert.Equal(t, tt.wantCode, httpErr.Code)
			assert.Equal(t, tt.status.Error, httpErr.Message)
		})
	}
}
