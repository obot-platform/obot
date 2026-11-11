package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	agentIdentityTestVMCP = "vmcp1shared"

	// agentIdentityTestAdminID is the user that makes the requests of agentIdentityFixture.
	agentIdentityTestAdminID = 1
	// agentIdentityTestOwnerID is an active user that agents can be owned by.
	agentIdentityTestOwnerID = 2
	// agentIdentityTestDisabledID is a disabled user.
	agentIdentityTestDisabledID = 3
)

type agentIdentityFixture struct {
	t       *testing.T
	handler http.Handler
	gateway *gclient.Client
	storage kclient.Client
	db      *gorm.DB
	// routes are the patterns of the routes the handler registers.
	routes []string
}

type agentRoute struct {
	method string
	path   string
	body   any
}

// newAgentIdentityFixture serves the admin API of agent identities, as pkg/api/router registers it, from a real
// gateway database. Authorization is not part of it: authorization is checked against the same routes in
// TestAgentIdentityRoutesAreAdminOnly.
func newAgentIdentityFixture(t *testing.T, enabled, authEnabled bool) *agentIdentityFixture {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())

	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.VMCP{
		Name:      agentIdentityTestVMCP,
		Namespace: system.DefaultNamespace,
	}).Build()
	gateway := gclient.New(t.Context(), database, storage, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })

	disabledAt := time.Now()
	for _, u := range []gt.User{
		{
			ID: agentIdentityTestAdminID,
		},
		{
			ID: agentIdentityTestOwnerID,
		},
		{
			ID:         agentIdentityTestDisabledID,
			DisabledAt: &disabledAt,
		},
	} {
		u.Username = "user" + strconv.Itoa(int(u.ID))
		u.HashedUsername = u.Username
		require.NoError(t, services.DB.DB.Create(&u).Error)
	}

	f := &agentIdentityFixture{
		t:       t,
		gateway: gateway,
		storage: storage,
		db:      services.DB.DB,
	}
	mux := http.NewServeMux()
	NewAgentIdentityHandler(enabled, authEnabled).RegisterRoutes(func(pattern string, handle api.HandlerFunc) {
		f.routes = append(f.routes, pattern)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			err := handle(api.Context{
				ResponseWriter: w,
				Request:        r,
				GatewayClient:  gateway,
				Storage:        storage,
				User: &user.DefaultInfo{
					Name:   strconv.Itoa(agentIdentityTestAdminID),
					UID:    strconv.Itoa(agentIdentityTestAdminID),
					Groups: types.RoleAdmin.Groups(),
				},
			})
			// The server writes the error of a handler like this.
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
				http.Error(w, httpErr.Message, httpErr.Code)
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	})
	f.handler = mux
	return f
}

// do makes a request. A body that is not a string is marshaled.
func (f *agentIdentityFixture) do(method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()

	var data []byte
	switch b := body.(type) {
	case nil:
	case string:
		data = []byte(b)
	default:
		var err error
		data, err = json.Marshal(b)
		require.NoError(f.t, err)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(data)))
	return rec
}

// call makes a request that must answer with status, and decodes its body into out, if given.
func (f *agentIdentityFixture) call(status int, method, path string, body, out any) {
	f.t.Helper()

	rec := f.do(method, path, body)
	require.Equal(f.t, status, rec.Code, "%s %s: %s", method, path, rec.Body.String())
	if out != nil {
		require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), out), rec.Body.String())
	}
}

func (f *agentIdentityFixture) createAgent(manifest types.AgentIdentityManifest) types.AgentIdentity {
	f.t.Helper()

	var agent types.AgentIdentity
	f.call(http.StatusCreated, http.MethodPost, "/api/agents", manifest, &agent)
	return agent
}

func (f *agentIdentityFixture) createKey(agentID string, req types.AgentAPIKeyCreateRequest) types.AgentAPIKeyCreateResponse {
	f.t.Helper()

	var key types.AgentAPIKeyCreateResponse
	f.call(http.StatusCreated, http.MethodPost, "/api/agents/"+agentID+"/api-keys", req, &key)
	return key
}

func TestAgentIdentityLifecycleOverHTTP(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)

	created := f.createAgent(types.AgentIdentityManifest{
		DisplayName:  "  nightly build  ",
		Description:  "runs at night",
		OwnerUserIDs: []string{strconv.Itoa(agentIdentityTestOwnerID), strconv.Itoa(agentIdentityTestOwnerID)},
	})
	path := "/api/agents/" + created.ID
	require.NotEmpty(t, created.ID)
	require.Equal(t, "nightly build", created.DisplayName, "the display name is trimmed")
	require.Equal(t, "runs at night", created.Description)
	require.Equal(t, []string{"2"}, created.OwnerUserIDs, "an owner is listed once")
	require.Equal(t, "1", created.CreatorUserID, "the creator is the calling admin")
	require.Equal(t, types.UserStatusActive, created.Status)
	require.Zero(t, created.ActiveAPIKeyCount)

	// Another agent with the same display name: names need not be unique.
	other := f.createAgent(types.AgentIdentityManifest{
		DisplayName: "nightly build",
	})
	require.NotEqual(t, created.ID, other.ID)

	var got types.AgentIdentity
	f.call(http.StatusOK, http.MethodGet, path, nil, &got)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "nightly build", got.DisplayName)

	var list types.AgentIdentityList
	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, &list)
	require.Len(t, list.Items, 2)
	require.Equal(t, created.ID, list.Items[0].ID)

	// Update replaces the metadata, and nothing else.
	f.call(http.StatusOK, http.MethodPost, path+"/disable", types.AgentIdentityDisableRequest{Reason: "leaked key"}, nil)
	var updated types.AgentIdentity
	f.call(http.StatusOK, http.MethodPut, path, types.AgentIdentityManifest{
		DisplayName: "renamed",
	}, &updated)
	require.Equal(t, "renamed", updated.DisplayName)
	require.Empty(t, updated.Description, "an update replaces the metadata, so an omitted description is cleared")
	require.Empty(t, updated.OwnerUserIDs)
	require.Equal(t, "1", updated.CreatorUserID, "the creator never changes")
	require.Equal(t, types.UserStatusDisabled, updated.Status, "an update does not enable an agent")
	require.Equal(t, "leaked key", updated.DisabledReason)

	// Disable and enable are idempotent.
	var disabled types.AgentIdentity
	f.call(http.StatusOK, http.MethodPost, path+"/disable", types.AgentIdentityDisableRequest{Reason: "leaked key"}, &disabled)
	require.Equal(t, types.UserStatusDisabled, disabled.Status)
	require.NotNil(t, disabled.DisabledAt)
	f.call(http.StatusOK, http.MethodPost, path+"/disable", types.AgentIdentityDisableRequest{Reason: "under review"}, &disabled)
	require.Equal(t, "under review", disabled.DisabledReason, "disabling a disabled agent updates the reason")
	var noReason types.AgentIdentity
	f.call(http.StatusOK, http.MethodPost, path+"/disable", nil, &noReason)
	require.Equal(t, types.UserStatusDisabled, noReason.Status, "the reason is optional")
	require.Empty(t, noReason.DisabledReason)

	var enabled, enabledAgain types.AgentIdentity
	f.call(http.StatusOK, http.MethodPost, path+"/enable", nil, &enabled)
	require.Equal(t, types.UserStatusActive, enabled.Status)
	require.Nil(t, enabled.DisabledAt)
	require.Empty(t, enabled.DisabledReason)
	f.call(http.StatusOK, http.MethodPost, path+"/enable", nil, &enabledAgain)
	require.Equal(t, types.UserStatusActive, enabledAgain.Status)

	// The key count is of the keys that can be used.
	key := f.createKey(created.ID, types.AgentAPIKeyCreateRequest{Name: "a", CanAccessLLMProxy: true})
	revoked := f.createKey(created.ID, types.AgentAPIKeyCreateRequest{Name: "b", CanAccessLLMProxy: true})
	f.call(http.StatusNoContent, http.MethodDelete, path+"/api-keys/"+strconv.Itoa(int(revoked.ID)), nil, nil)
	past := types.NewTime(time.Now().Add(-time.Hour))
	require.NoError(t, f.db.Model(&gt.APIKey{}).Where("id = ?", f.createKey(created.ID, types.AgentAPIKeyCreateRequest{Name: "c", CanAccessLLMProxy: true}).ID).Update("expires_at", past.Time).Error)
	f.call(http.StatusOK, http.MethodGet, path, nil, &got)
	require.Equal(t, 1, got.ActiveAPIKeyCount)
	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, &list)
	require.Equal(t, 1, list.Items[0].ActiveAPIKeyCount)
	require.Zero(t, list.Items[1].ActiveAPIKeyCount)

	// Deleting soft-deletes the agent, and revokes its keys.
	f.call(http.StatusNoContent, http.MethodDelete, path, nil, nil)
	stored, err := f.gateway.GetAPIKeyByID(t.Context(), key.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.RevokedAt, "deleting an agent revokes its keys")
	require.Eventually(t, func() bool {
		var deletes v1.AgentIdentityDeleteList
		return f.storage.List(t.Context(), &deletes) == nil && len(deletes.Items) == 1 && deletes.Items[0].Spec.AgentID == uintOf(t, created.ID)
	}, 10*time.Second, 10*time.Millisecond, "deleting an agent emits its delete event")

	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, &list)
	require.Len(t, list.Items, 1)
	require.Equal(t, other.ID, list.Items[0].ID)
	f.call(http.StatusOK, http.MethodGet, "/api/agents?includeDeleted=true", nil, &list)
	require.Len(t, list.Items, 2)
	require.Equal(t, types.UserStatusDeleted, list.Items[0].Status)
	require.NotNil(t, list.Items[0].Deleted)
	require.Zero(t, list.Items[0].ActiveAPIKeyCount)

	// A deleted agent is 404 everywhere else.
	for _, route := range agentRoutes(path, key.ID) {
		require.Equal(t, http.StatusNotFound, f.do(route.method, route.path, route.body).Code, "%s %s", route.method, route.path)
	}
	f.call(http.StatusBadRequest, http.MethodGet, "/api/agents?includeDeleted=maybe", nil, nil)
}

// agentRoutes are the admin API's routes of the agent at path, and of its key with keyID, each with a valid body.
func agentRoutes(path string, keyID uint) []agentRoute {
	keyPath := path + "/api-keys/" + strconv.Itoa(int(keyID))
	return []agentRoute{
		{http.MethodGet, path, nil},
		{http.MethodPut, path, types.AgentIdentityManifest{DisplayName: "x"}},
		{http.MethodDelete, path, nil},
		{http.MethodPost, path + "/disable", nil},
		{http.MethodPost, path + "/enable", nil},
		{http.MethodGet, path + "/api-keys", nil},
		{http.MethodPost, path + "/api-keys", types.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true}},
		{http.MethodGet, keyPath, nil},
		{http.MethodDelete, keyPath, nil},
	}
}

func uintOf(t *testing.T, s string) uint {
	t.Helper()

	n, err := strconv.ParseUint(s, 10, 64)
	require.NoError(t, err)
	return uint(n)
}

func TestAgentIdentityValidation(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})

	for _, tt := range []struct {
		name string
		body any
	}{
		{
			name: "no body",
		},
		{
			name: "not JSON",
			body: "{",
		},
		{
			name: "trailing data",
			body: `{"displayName":"x"} {}`,
		},
		{
			name: "no display name",
			body: types.AgentIdentityManifest{
				Description: "nameless",
			},
		},
		{
			name: "blank display name",
			body: types.AgentIdentityManifest{
				DisplayName: " \t ",
			},
		},
		{
			name: "unknown owner",
			body: types.AgentIdentityManifest{
				DisplayName:  "x",
				OwnerUserIDs: []string{"999"},
			},
		},
		{
			name: "disabled owner",
			body: types.AgentIdentityManifest{
				DisplayName:  "x",
				OwnerUserIDs: []string{strconv.Itoa(agentIdentityTestDisabledID)},
			},
		},
		{
			name: "owner that is not an ID",
			body: types.AgentIdentityManifest{
				DisplayName:  "x",
				OwnerUserIDs: []string{"admin"},
			},
		},
		{
			name: "owner that is an agent's principal",
			body: types.AgentIdentityManifest{
				DisplayName:  "x",
				OwnerUserIDs: []string{"agent:" + agent.ID},
			},
		},
		{
			name: "unknown field",
			body: `{"displayName":"x","status":"disabled"}`,
		},
	} {
		t.Run(tt.name, func(*testing.T) {
			f.call(http.StatusBadRequest, http.MethodPost, "/api/agents", tt.body, nil)
			f.call(http.StatusBadRequest, http.MethodPut, "/api/agents/"+agent.ID, tt.body, nil)
		})
	}

	// Nothing was changed by the refused updates.
	var got types.AgentIdentity
	f.call(http.StatusOK, http.MethodGet, "/api/agents/"+agent.ID, nil, &got)
	require.Equal(t, "agent", got.DisplayName)
	var list types.AgentIdentityList
	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, &list)
	require.Len(t, list.Items, 1)

	f.call(http.StatusBadRequest, http.MethodPost, "/api/agents/"+agent.ID+"/disable", `{"reason":1}`, nil)
	f.call(http.StatusBadRequest, http.MethodPost, "/api/agents/"+agent.ID+"/disable", `{"why":"x"}`, nil)
}

func TestAgentIdentityUnknownAgentIsNotFound(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})
	key := f.createKey(agent.ID, types.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})

	for _, id := range []string{"999", "0", "abc", "agent:1", "-1"} {
		for _, route := range agentRoutes("/api/agents/"+id, key.ID) {
			require.Equal(t, http.StatusNotFound, f.do(route.method, route.path, route.body).Code, "%s %s", route.method, route.path)
		}
	}

	// The key is untouched by the requests for other agents.
	stored, err := f.gateway.GetAPIKeyByID(t.Context(), key.ID)
	require.NoError(t, err)
	require.Nil(t, stored.RevokedAt)
}

func TestAgentAPIKeyScopesRoundTripOverHTTP(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})
	keysPath := "/api/agents/" + agent.ID + "/api-keys"

	for _, tt := range []struct {
		name    string
		request types.AgentAPIKeyCreateRequest
		want    types.AgentAPIKey
	}{
		{
			name: "skills only",
			request: types.AgentAPIKeyCreateRequest{
				Name:            "skills",
				CanAccessSkills: true,
			},
			want: types.AgentAPIKey{
				CanAccessSkills: true,
				MCPServerIDs:    []string{},
			},
		},
		{
			name: "LLM proxy only",
			request: types.AgentAPIKeyCreateRequest{
				CanAccessLLMProxy: true,
			},
			want: types.AgentAPIKey{
				CanAccessLLMProxy: true,
				MCPServerIDs:      []string{},
			},
		},
		{
			name: "one vMCP",
			request: types.AgentAPIKeyCreateRequest{
				Name:         "vmcp",
				Description:  "connects",
				MCPServerIDs: []string{agentIdentityTestVMCP},
			},
			want: types.AgentAPIKey{
				MCPServerIDs: []string{agentIdentityTestVMCP},
			},
		},
		{
			name: "every vMCP and every other scope",
			request: types.AgentAPIKeyCreateRequest{
				CanAccessSkills:   true,
				CanAccessLLMProxy: true,
				MCPServerIDs:      []string{"*"},
			},
			want: types.AgentAPIKey{
				CanAccessSkills:   true,
				CanAccessLLMProxy: true,
				MCPServerIDs:      []string{"*"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(http.MethodPost, keysPath, tt.request)
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			var created types.AgentAPIKeyCreateResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
			require.True(t, strings.HasPrefix(created.Key, "oak1-"+agent.ID+"-"+strconv.Itoa(int(created.ID))+"-"), created.Key)
			require.Equal(t, agent.ID, created.AgentID)

			// The scopes of the response, of a read, and of the list are the requested ones, and an agent's key has
			// neither of the scopes that a user's has and an agent's never does.
			var raw map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
			require.Contains(t, raw, "mcpServerIds", "an empty list of vMCPs must not look like a missing one")
			require.NotContains(t, raw, "canAccessAPI")
			require.NotContains(t, raw, "canAccessDeviceScans")
			require.NotContains(t, raw, "userId")

			var got types.AgentAPIKey
			f.call(http.StatusOK, http.MethodGet, keysPath+"/"+strconv.Itoa(int(created.ID)), nil, &got)
			require.NotContains(t, f.do(http.MethodGet, keysPath+"/"+strconv.Itoa(int(created.ID)), nil).Body.String(), `"key"`, "the secret is only shown on creation")
			for _, key := range []types.AgentAPIKey{created.AgentAPIKey, got} {
				require.Equal(t, tt.want.CanAccessSkills, key.CanAccessSkills)
				require.Equal(t, tt.want.CanAccessLLMProxy, key.CanAccessLLMProxy)
				require.Equal(t, tt.want.MCPServerIDs, key.MCPServerIDs)
				require.Equal(t, tt.request.Name, key.Name)
				require.Equal(t, tt.request.Description, key.Description)
			}

			// The key that was stored is the one that was requested: neither API nor device scan access.
			stored, err := f.gateway.GetAPIKeyByID(t.Context(), created.ID)
			require.NoError(t, err)
			require.False(t, stored.CanAccessAPI)
			require.False(t, stored.CanAccessDeviceScans)
			require.Equal(t, tt.want.CanAccessSkills, stored.CanAccessSkills)
			require.Equal(t, tt.want.CanAccessLLMProxy, stored.CanAccessLLMProxy)
		})
	}

	var list types.AgentAPIKeyList
	f.call(http.StatusOK, http.MethodGet, keysPath, nil, &list)
	require.Len(t, list.Items, 4)
	require.NotContains(t, f.do(http.MethodGet, keysPath, nil).Body.String(), `"key"`)
}

func TestAgentAPIKeyRequestsAreRefused(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})
	keysPath := "/api/agents/" + agent.ID + "/api-keys"

	for _, tt := range []struct {
		name string
		body any
	}{
		{
			name: "no body",
		},
		{
			name: "no scope",
			body: types.AgentAPIKeyCreateRequest{
				Name: "nothing",
			},
		},
		{
			name: "false scopes",
			body: `{"name":"nothing","canAccessSkills":false,"canAccessLLMProxy":false,"mcpServerIds":[]}`,
		},
		{
			name: "an unknown vMCP",
			body: types.AgentAPIKeyCreateRequest{
				MCPServerIDs: []string{"vmcp1missing"},
			},
		},
		{
			name: "an MCP server that is not a vMCP",
			body: types.AgentAPIKeyCreateRequest{
				MCPServerIDs: []string{"ms1server"},
			},
		},
		{
			// Never granted, and never silently ignored: ignoring it would answer a key that lacks what was asked for.
			name: "API access",
			body: `{"canAccessLLMProxy":true,"canAccessAPI":true}`,
		},
		{
			name: "device scan access",
			body: `{"canAccessLLMProxy":true,"canAccessDeviceScans":true}`,
		},
	} {
		t.Run(tt.name, func(*testing.T) {
			f.call(http.StatusBadRequest, http.MethodPost, keysPath, tt.body, nil)
		})
	}

	var list types.AgentAPIKeyList
	f.call(http.StatusOK, http.MethodGet, keysPath, nil, &list)
	require.Empty(t, list.Items, "no refused request created a key")
	require.Contains(t, f.do(http.MethodGet, keysPath, nil).Body.String(), `"items":[]`)
}

func TestAgentAPIKeysBelongToTheirAgent(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})
	other := f.createAgent(types.AgentIdentityManifest{DisplayName: "other"})
	otherKey := f.createKey(other.ID, types.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	userKey, err := f.gateway.CreateAPIKey(t.Context(), agentIdentityTestOwnerID, "user's", "", nil, gt.APIKeyScopes{CanAccessLLMProxy: true})
	require.NoError(t, err)

	agentKeysPath := "/api/agents/" + agent.ID + "/api-keys"
	for _, key := range []uint{otherKey.ID, userKey.ID, 999} {
		path := agentKeysPath + "/" + strconv.Itoa(int(key))
		f.call(http.StatusNotFound, http.MethodGet, path, nil, nil)
		// A revocation of a key of another owner would succeed without revoking it.
		f.call(http.StatusNotFound, http.MethodDelete, path, nil, nil)
	}
	f.call(http.StatusNotFound, http.MethodGet, agentKeysPath+"/abc", nil, nil)
	for _, id := range []uint{otherKey.ID, userKey.ID} {
		stored, err := f.gateway.GetAPIKeyByID(t.Context(), id)
		require.NoError(t, err)
		require.Nil(t, stored.RevokedAt)
	}

	// A key of the agent is revoked once, and revoking it again changes nothing.
	key := f.createKey(agent.ID, types.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	path := agentKeysPath + "/" + strconv.Itoa(int(key.ID))
	f.call(http.StatusNoContent, http.MethodDelete, path, nil, nil)
	first, err := f.gateway.GetAPIKeyByID(t.Context(), key.ID)
	require.NoError(t, err)
	require.NotNil(t, first.RevokedAt)
	f.call(http.StatusNoContent, http.MethodDelete, path, nil, nil)
	again, err := f.gateway.GetAPIKeyByID(t.Context(), key.ID)
	require.NoError(t, err)
	require.True(t, first.RevokedAt.Equal(*again.RevokedAt), "the first revocation time is kept")

	var list types.AgentAPIKeyList
	f.call(http.StatusOK, http.MethodGet, agentKeysPath, nil, &list)
	require.Empty(t, list.Items)
	f.call(http.StatusOK, http.MethodGet, agentKeysPath+"?show_revoked=true", nil, &list)
	require.Len(t, list.Items, 1)
	require.NotNil(t, list.Items[0].RevokedAt)
	f.call(http.StatusBadRequest, http.MethodGet, agentKeysPath+"?show_revoked=maybe", nil, nil)
	f.call(http.StatusOK, http.MethodGet, path, nil, nil)
}

func TestDisabledAgentGetsKeysThatWaitForEnable(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "agent"})
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+agent.ID+"/disable", types.AgentIdentityDisableRequest{Reason: "rotating"}, nil)

	key := f.createKey(agent.ID, types.AgentAPIKeyCreateRequest{Name: "next", CanAccessLLMProxy: true})

	// It does not authenticate while the agent is disabled, and does once it is enabled.
	validated, err := f.gateway.ValidateAPIKey(t.Context(), key.Key)
	require.NoError(t, err)
	require.Equal(t, types.UserStatusDisabled, validated.OwnerStatus)
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+agent.ID+"/enable", nil, nil)
	validated, err = f.gateway.ValidateAPIKey(t.Context(), key.Key)
	require.NoError(t, err)
	require.Equal(t, types.UserStatusActive, validated.OwnerStatus)
}

func TestAgentIdentityRoutesAreUnavailableWithoutTheFeature(t *testing.T) {
	for _, tt := range []struct {
		name        string
		enabled     bool
		authEnabled bool
		wantStatus  int
	}{
		{
			name:        "the feature is off",
			authEnabled: true,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:       "the feature is off and authentication is too",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "authentication is off",
			enabled:    true,
			wantStatus: http.StatusNotImplemented,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentIdentityFixture(t, tt.enabled, tt.authEnabled)
			require.Len(t, f.routes, 11)

			for _, pattern := range f.routes {
				method, path, _ := strings.Cut(pattern, " ")
				path = strings.NewReplacer("{agent_id}", "1", "{key_id}", "1").Replace(path)
				rec := f.do(method, path, `{"displayName":"x","canAccessLLMProxy":true}`)
				require.Equal(t, tt.wantStatus, rec.Code, "%s: %s", pattern, rec.Body.String())
			}

			if tt.wantStatus == http.StatusNotImplemented {
				require.Contains(t, f.do(http.MethodGet, "/api/agents", nil).Body.String(), "require authentication")
			}
			var agents int64
			require.NoError(t, f.db.Model(&gt.Agent{}).Count(&agents).Error)
			require.Zero(t, agents, "no request created an agent")
		})
	}
}

// TestAgentIdentityRoutesAreAdminOnly checks every registered route against the authorizer of the server: admins and
// owners reach all of them, auditors only the reads, and no other user, no owner of an agent, and no agent does.
func TestAgentIdentityRoutesAreAdminOnly(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	require.Len(t, f.routes, 11)
	authorizer := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil)

	// An agent with every scope its key can have.
	scopes := gt.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	}
	agent := gclient.AgentUserInfo(&gt.Agent{ID: 7, DisplayName: "agent"}, scopes.Groups(nil), map[string][]string{
		"authorized_mcp_ids": scopes.MCPServerIDs,
	})

	for _, tt := range []struct {
		name      string
		user      user.Info
		readsOnly bool
		denied    bool
	}{
		{
			name: "owner",
			user: &user.DefaultInfo{UID: "1", Groups: types.RoleOwner.Groups()},
		},
		{
			name: "admin",
			user: &user.DefaultInfo{UID: "1", Groups: types.RoleAdmin.Groups()},
		},
		{
			name: "admin who is an auditor",
			user: &user.DefaultInfo{UID: "1", Groups: (types.RoleAdmin | types.RoleAuditor).Groups()},
		},
		{
			name:      "auditor",
			user:      &user.DefaultInfo{UID: "1", Groups: (types.RoleBasic | types.RoleAuditor).Groups()},
			readsOnly: true,
		},
		{
			// An owner of an agent is a user like any other, and has no rights over the agent.
			name:   "basic user",
			user:   &user.DefaultInfo{UID: strconv.Itoa(agentIdentityTestOwnerID), Groups: types.RoleBasic.Groups()},
			denied: true,
		},
		{
			name:   "power user",
			user:   &user.DefaultInfo{UID: "1", Groups: types.RolePowerUser.Groups()},
			denied: true,
		},
		{
			name:   "power user plus",
			user:   &user.DefaultInfo{UID: "1", Groups: types.RolePowerUserPlus.Groups()},
			denied: true,
		},
		{
			name:   "an agent",
			user:   agent,
			denied: true,
		},
		{
			name:   "unauthenticated",
			user:   &user.DefaultInfo{UID: "", Groups: []string{authz.UnauthenticatedGroup}},
			denied: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pattern := range f.routes {
				method, path, _ := strings.Cut(pattern, " ")
				path = strings.NewReplacer("{agent_id}", "7", "{key_id}", "3").Replace(path)

				want := !tt.denied && (!tt.readsOnly || method == http.MethodGet)
				got := authorizer.Authorize(httptest.NewRequest(method, path, nil), tt.user)
				require.Equal(t, want, got, "%s %s", method, path)
			}
		})
	}
}
