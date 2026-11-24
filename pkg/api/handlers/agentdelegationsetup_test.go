package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	setupValue = "alice's secret value"
)

// recordingOAuthStarter records the authorizations the setup API starts.
type recordingOAuthStarter struct {
	pairs       []string
	components  []string
	returnPaths []string
}

// setupFixture is the delegation fixture with the setup API, and a vMCP whose profile naming the agent grants
// components that each need something different from a user:
//   - keyed: a local server with a required value the user enters, and a value the vMCP fixes.
//   - fixed: a local server whose only value the vMCP fixes.
//   - granted: a remote server the vMCP's preconfigured OAuth covers.
//   - account: a remote server without a grant, that the vMCP controller found asks for OAuth.
type setupFixture struct {
	*delegationFixture
	setup  http.Handler
	oauth  *recordingOAuthStarter
	grants *vmcpconfig.PreconfiguredOAuthStore
	agent  uint
}

func (r *recordingOAuthStarter) StartDelegatedSetupOAuth(_ api.Context, _ *v1.VMCP, pair *v1.VMCPInstance, componentID, returnPath string) (types.AgentDelegationSetupOAuth, error) {
	r.pairs = append(r.pairs, pair.Spec.UserID)
	r.components = append(r.components, componentID)
	r.returnPaths = append(r.returnPaths, returnPath)
	return types.AgentDelegationSetupOAuth{AuthURL: "https://provider.example.com/authorize?state=s"}, nil
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	f := &setupFixture{delegationFixture: newDelegationFixture(t), oauth: &recordingOAuthStarter{}}
	f.agent = uintOf(t, f.delegationFixture.agent.ID)

	// The setup is held in vMCP instances, which are found by their owner and vMCP, as the server's storage indexes
	// them.
	var objects []kclient.Object
	for _, list := range []kclient.ObjectList{&v1.VMCPList{}, &v1.ModelAccessPolicyList{}, &v1.SkillAccessRuleList{}, &v1.UserDefaultRoleSettingList{}} {
		require.NoError(t, f.storage.List(t.Context(), list))
		items, err := extractObjects(list)
		require.NoError(t, err)
		objects = append(objects, items...)
	}
	f.storage = fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).
		WithStatusSubresource(&v1.VMCP{}).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		Build()
	f.authorizer = authz.NewAuthorizer(f.gateway, f.storage, f.storage, false, nil, nil, nil)

	local := func(id string, policies ...types.VMCPConfigurationPolicy) types.VMCPComponent {
		component := types.VMCPComponent{
			ID:            id,
			Name:          id,
			Configuration: policies,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "@example/" + id},
			}},
		}
		for _, policy := range policies {
			component.CatalogEntry.Manifest.Config = append(component.CatalogEntry.Manifest.Config, types.MCPConfig{
				Key: policy.Key, Name: policy.Key + " name", Description: policy.Key + " description", Usage: types.Env, Required: true, Sensitive: true,
			})
		}
		return component
	}
	remote := func(id string) types.VMCPComponent {
		return types.VMCPComponent{ID: id, Name: id, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"},
		}}}
	}
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: delegationTestVMCP}, &vmcp))
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{
		local("keyed", types.VMCPConfigurationPolicy{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed}, types.VMCPConfigurationPolicy{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed}),
		local("fixed", types.VMCPConfigurationPolicy{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed}),
		remote("granted"),
		remote("account"),
	}
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowAllComponents: true}
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	account := vmcp.Spec.Manifest.Components[3]
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{account.ID: {
		TargetHash: vmcpconfig.ComponentOAuthTargetHash(vmcp, account),
		Required:   v1.VMCPComponentOAuthRequired,
	}}
	require.NoError(t, f.storage.Status().Update(t.Context(), &vmcp))

	f.grants = vmcpconfig.NewPreconfiguredOAuthStore(f.gateway)
	require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpconfig.PreconfiguredOAuthMCPID(vmcp.Name, "granted"),
		"https://granted.example.com/mcp", "grant-granted", "1", &oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "grant", Expiry: time.Now().Add(time.Hour)}))

	mux := http.NewServeMux()
	NewAgentDelegationSetupHandler(f.grants, f.oauth).RegisterRoutes(func(pattern string, handle api.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !f.authorizer.Authorize(r, f.caller) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			err := handle(api.Context{ResponseWriter: w, Request: r, GatewayClient: f.gateway, Storage: f.storage, User: f.caller})
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
				http.Error(w, httpErr.Message, httpErr.Code)
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	})
	f.setup = mux
	return f
}

// extractObjects returns the items of list as objects.
func extractObjects(list kclient.ObjectList) ([]kclient.Object, error) {
	data, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	var items struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	var objects []kclient.Object
	for _, raw := range items.Items {
		var obj kclient.Object
		switch list.(type) {
		case *v1.VMCPList:
			obj = &v1.VMCP{}
		case *v1.ModelAccessPolicyList:
			obj = &v1.ModelAccessPolicy{}
		case *v1.SkillAccessRuleList:
			obj = &v1.SkillAccessRule{}
		case *v1.UserDefaultRoleSettingList:
			obj = &v1.UserDefaultRoleSetting{}
		}
		if err := json.Unmarshal(raw, obj); err != nil {
			return nil, err
		}
		obj.SetResourceVersion("")
		objects = append(objects, obj)
	}
	return objects, nil
}

// do makes a request of the setup API as caller, from Obot's own pages.
func (f *setupFixture) do(caller user.Info, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	req := newJSONRequest(f.t, method, path, body)
	if method != http.MethodGet {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	f.caller = caller
	defer func() { f.caller = f.admin }()
	rec := httptest.NewRecorder()
	f.setup.ServeHTTP(rec, req)
	require.NotContains(f.t, rec.Body.String(), setupValue, "a response carried a value")
	return rec
}

// pairInstance returns the instance of what the user with uid set up for the vMCP through the agent, or nil.
func (f *setupFixture) pairInstance(uid string) *v1.VMCPInstance {
	f.t.Helper()
	instance, err := vmcpconfig.FindInstance(f.t.Context(), f.storage, system.DefaultNamespace, delegationTestVMCP, principal.DelegatedOwner{AgentID: f.agent, UserID: uintOf(f.t, uid)}.String())
	require.NoError(f.t, err)
	return instance
}

// pairValues returns the values the user with uid set up through the agent, as stored.
func (f *setupFixture) pairValues(uid string) map[string]string {
	f.t.Helper()
	instance := f.pairInstance(uid)
	if instance == nil {
		return nil
	}
	credential, err := f.gateway.RevealCredential(f.t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	if err != nil {
		return nil
	}
	return credential.Secrets
}

func setupComponents(setup types.AgentDelegationSetupVMCP) map[string]types.AgentDelegationSetupComponent {
	components := map[string]types.AgentDelegationSetupComponent{}
	for _, component := range setup.Components {
		components[component.ComponentID] = component
	}
	return components
}

// TestAgentDelegationSetupDuringApproval checks the setup a user does while approving a request: it lists every
// component of the vMCPs the agent is granted, with what each needs and whether it is set, never a value;
// it validates and stores the user's values for the pair of the agent and the user, apart from the user's own; it
// clears them; it starts the OAuth of a component for the pair, returning to the approval page, and refuses it where
// the vMCP's grant covers the component; and it disconnects the pair's account.
func TestAgentDelegationSetupDuringApproval(t *testing.T) {
	f := newSetupFixture(t)
	request := f.requestApproval(delegationSecretA)
	base := "/api/agent-delegation-requests/" + request.RequestID + "/setup"

	setup := decode[types.AgentDelegationSetup](t, f.do(f.alice, http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, setup.VMCPs, 1)
	require.Equal(t, delegationTestVMCP, setup.VMCPs[0].VMCPID)
	require.Equal(t, "Docs", setup.VMCPs[0].DisplayName)
	require.False(t, setup.VMCPs[0].Ready)
	components := setupComponents(setup.VMCPs[0])
	require.Equal(t, types.AgentDelegationSetupComponent{
		ComponentID: "keyed",
		Name:        "keyed",
		Reasons:     []types.AgentReadinessReason{{ComponentID: "keyed", Code: types.AgentReadinessConfigurationRequired, Field: "TOKEN"}},
		Configuration: []types.AgentDelegationSetupField{{
			Key: "TOKEN", Name: "TOKEN name", Description: "TOKEN description", Sensitive: true, Required: true, Usage: types.Env,
		}},
		OAuth: types.AgentDelegationSetupOAuthNotRequired,
	}, components["keyed"])
	require.True(t, components["fixed"].Covered)
	require.Equal(t, types.AgentDelegationSetupCoveredFixed, components["fixed"].CoveredBy)
	require.True(t, components["granted"].Covered)
	require.Equal(t, types.AgentDelegationSetupCoveredPreconfiguredOAuth, components["granted"].CoveredBy)
	require.Equal(t, types.AgentDelegationSetupOAuthPreconfigured, components["granted"].OAuth)
	require.False(t, components["account"].Covered)
	require.Equal(t, types.AgentDelegationSetupOAuthNeedsAuthorization, components["account"].OAuth)
	require.Nil(t, f.pairInstance(f.alice.GetUID()), "reading the setup created it")

	configuration := base + "/vmcps/" + delegationTestVMCP + "/configuration"
	t.Run("values that are not the user's to enter are refused", func(t *testing.T) {
		for _, body := range []types.AgentDelegationSetupConfiguration{
			{Components: map[string]map[string]*string{"keyed": {"REGION": new("elsewhere")}}},
			{Components: map[string]map[string]*string{"keyed": {"REGION": nil}}},
			{Components: map[string]map[string]*string{"keyed": {"UNKNOWN": new("x")}}},
			{Components: map[string]map[string]*string{"elsewhere": {"TOKEN": new("x")}}},
		} {
			requireStatus(t, f.do(f.alice, http.MethodPut, configuration, body), http.StatusBadRequest, "")
		}
		require.Empty(t, f.pairValues(f.alice.GetUID()))
	})

	vmcpSetup := decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, configuration, types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{"keyed": {"TOKEN": new(setupValue)}},
	}), http.StatusOK)
	require.True(t, setupComponents(vmcpSetup)["keyed"].Configuration[0].Set)
	require.True(t, setupComponents(vmcpSetup)["keyed"].Ready)
	pair := f.pairInstance(f.alice.GetUID())
	require.NotNil(t, pair)
	require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("keyed", "TOKEN"): setupValue}, f.pairValues(f.alice.GetUID()))
	require.NotEmpty(t, pair.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation], "the controllers are not told to copy the values")
	// It is the pair's alone: the user has no instance of their own, and another user's setup through the agent is
	// their own.
	own, err := vmcpconfig.FindInstance(t.Context(), f.storage, system.DefaultNamespace, delegationTestVMCP, f.alice.GetUID())
	require.NoError(t, err)
	require.Nil(t, own)
	setup = decode[types.AgentDelegationSetup](t, f.do(f.bob, http.MethodGet, base, nil), http.StatusOK)
	require.False(t, setupComponents(setup.VMCPs[0])["keyed"].Configuration[0].Set, "bob sees alice's setup")

	// Leaving a key out keeps it, and null clears it.
	vmcpSetup = decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, configuration, types.AgentDelegationSetupConfiguration{}), http.StatusOK)
	require.True(t, setupComponents(vmcpSetup)["keyed"].Configuration[0].Set)
	vmcpSetup = decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, configuration, types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{"keyed": {"TOKEN": nil}},
	}), http.StatusOK)
	require.False(t, setupComponents(vmcpSetup)["keyed"].Configuration[0].Set)
	require.Empty(t, f.pairValues(f.alice.GetUID()))

	oauth := func(componentID string) string {
		return base + "/vmcps/" + delegationTestVMCP + "/components/" + componentID + "/oauth"
	}
	started := decode[types.AgentDelegationSetupOAuth](t, f.do(f.alice, http.MethodPost, oauth("account"), nil), http.StatusOK)
	require.NotEmpty(t, started.AuthURL)
	require.Equal(t, []string{pair.Spec.UserID}, f.oauth.pairs)
	require.Equal(t, []string{"account"}, f.oauth.components)
	require.Equal(t, []string{"/agents/approve/" + request.RequestID}, f.oauth.returnPaths)
	requireStatus(t, f.do(f.alice, http.MethodPost, oauth("granted"), nil), http.StatusConflict, "preconfigured OAuth")
	requireStatus(t, f.do(f.alice, http.MethodPost, oauth("keyed"), nil), http.StatusBadRequest, "does not use OAuth")
	requireStatus(t, f.do(f.alice, http.MethodPost, oauth("missing"), nil), http.StatusNotFound, "")

	// The account the user connected through the agent is the pair's; disconnecting removes only it.
	connection := vmcpconfig.PairConnectID(pair.Name, vmcpconfig.ComponentsForInstance(v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "account", CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://account.example.com/mcp"}}}}}}}}, *pair)[0])
	for _, owner := range []string{pair.Spec.UserID, f.alice.GetUID()} {
		require.NoError(t, f.gateway.ReplaceMCPOAuthToken(t.Context(), owner, connection, "https://account.example.com/mcp", "", &oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: owner, Expiry: time.Now().Add(time.Hour)}))
	}
	setup = decode[types.AgentDelegationSetup](t, f.do(f.alice, http.MethodGet, base, nil), http.StatusOK)
	require.Equal(t, types.AgentDelegationSetupOAuthConnected, setupComponents(setup.VMCPs[0])["account"].OAuth)
	require.True(t, setupComponents(setup.VMCPs[0])["account"].Ready)
	vmcpSetup = decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodDelete, oauth("account"), nil), http.StatusOK)
	require.Equal(t, types.AgentDelegationSetupOAuthNeedsAuthorization, setupComponents(vmcpSetup)["account"].OAuth)
	_, err = f.gateway.GetMCPOAuthToken(t.Context(), pair.Spec.UserID, connection, "https://account.example.com/mcp")
	require.Error(t, err)
	_, err = f.gateway.GetMCPOAuthToken(t.Context(), f.alice.GetUID(), connection, "https://account.example.com/mcp")
	require.NoError(t, err, "another owner's account for the same connection name was deleted")

	// Approval is allowed with setup incomplete.
	f.approve(f.alice, request.RequestID)
}

// TestAgentDelegationSetupRefusals checks who may reach the setup routes: during approval, only a signed-in user's
// browser session from Obot's own pages, for a pending request they may approve, and never another user's request
// to approve their delegation again, or an expired or used one; afterwards, only the delegation's own user. An
// administrator reaches neither, nor what a user set up through an agent on the instance routes, which administrators
// otherwise reach for every instance.
func TestAgentDelegationSetupRefusals(t *testing.T) {
	f := newSetupFixture(t)
	admin := f.createUser("root", types.RoleAdmin)
	alices := f.delegate(f.alice)

	managed := "/api/me/agent-delegations/" + alices.DelegationID + "/setup"
	for _, caller := range []user.Info{f.bob, admin} {
		requireStatus(t, f.do(caller, http.MethodGet, managed, nil), http.StatusForbidden, "")
		requireStatus(t, f.do(caller, http.MethodPut, managed+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{}), http.StatusForbidden, "")
		requireStatus(t, f.do(caller, http.MethodPost, managed+"/vmcps/"+delegationTestVMCP+"/components/account/oauth", nil), http.StatusForbidden, "")
		requireStatus(t, f.do(caller, http.MethodDelete, managed+"/vmcps/"+delegationTestVMCP+"/components/account/oauth", nil), http.StatusForbidden, "")
	}
	decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, managed+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{"keyed": {"TOKEN": new(setupValue)}},
	}), http.StatusOK)
	setup := decode[types.AgentDelegationSetup](t, f.do(f.alice, http.MethodGet, managed, nil), http.StatusOK)
	require.True(t, setupComponents(setup.VMCPs[0])["keyed"].Configuration[0].Set)
	// The management page is where its OAuth returns to.
	decode[types.AgentDelegationSetupOAuth](t, f.do(f.alice, http.MethodPost, managed+"/vmcps/"+delegationTestVMCP+"/components/account/oauth", nil), http.StatusOK)
	require.Equal(t, []string{"/agents/delegations/" + alices.DelegationID}, f.oauth.returnPaths)
	// A vMCP the agent is not granted is not found.
	requireStatus(t, f.do(f.alice, http.MethodPut, managed+"/vmcps/vmcp1elsewhere/configuration", types.AgentDelegationSetupConfiguration{}), http.StatusNotFound, "")

	// A request to approve alice's delegation again is alice's alone.
	again := decode[types.AgentDelegationRequestCreated](t, f.asAgent(f.key.Key, http.MethodPost, "/api/agent-delegations", types.AgentDelegationRequestCreate{
		ReturnURL:    delegationTestReturnURL,
		SecretHash:   s256(delegationSecretA),
		DelegationID: alices.DelegationID,
	}), http.StatusCreated)
	requestSetup := "/api/agent-delegation-requests/" + again.RequestID + "/setup"
	requireStatus(t, f.do(f.bob, http.MethodGet, requestSetup, nil), http.StatusForbidden, "")
	requireStatus(t, f.do(f.bob, http.MethodPut, requestSetup+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{}), http.StatusForbidden, "")
	// Alice sees what she set up through the agent before, set but without its value.
	setup = decode[types.AgentDelegationSetup](t, f.do(f.alice, http.MethodGet, requestSetup, nil), http.StatusOK)
	require.True(t, setupComponents(setup.VMCPs[0])["keyed"].Configuration[0].Set)

	// A cross-site request from the user's browser is refused.
	req := newJSONRequest(t, http.MethodPut, requestSetup+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{})
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	f.caller = f.alice
	rec := httptest.NewRecorder()
	f.setup.ServeHTTP(rec, req)
	f.caller = f.admin
	require.Equal(t, http.StatusForbidden, rec.Code)

	// An expired request, and one that was used, are refused.
	require.NoError(t, f.db.Model(&gt.AgentDelegationRequest{}).Where("id = ?", again.RequestID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	requireStatus(t, f.do(f.alice, http.MethodGet, requestSetup, nil), http.StatusForbidden, "")
	used := f.requestApproval(delegationSecretA)
	f.approve(f.alice, used.RequestID)
	requireStatus(t, f.do(f.alice, http.MethodGet, "/api/agent-delegation-requests/"+used.RequestID+"/setup", nil), http.StatusForbidden, "")

	// The pair's instance is reached through no instance route, by an administrator either.
	pair := f.pairInstance(f.alice.GetUID())
	require.NotNil(t, pair)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/vmcp-instances/" + pair.Name},
		{http.MethodPost, "/api/vmcp-instances/" + pair.Name + "/reveal"},
		{http.MethodPost, "/api/vmcp-instances/" + pair.Name + "/configure"},
		{http.MethodPost, "/api/vmcp-instances/" + pair.Name + "/deconfigure"},
		{http.MethodPut, "/api/vmcp-instances/" + pair.Name},
		{http.MethodDelete, "/api/vmcp-instances/" + pair.Name},
	} {
		for _, caller := range []user.Info{admin, f.alice} {
			require.False(t, f.authorizer.Authorize(httptest.NewRequest(route.method, route.path, bytes.NewReader(nil)), caller), "%s %s %s", caller.GetName(), route.method, route.path)
		}
	}
	// A user's own instance is still the administrators' to manage.
	own := &v1.VMCPInstance{Name: "vmcpi1own", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: f.alice.GetUID(), Manifest: types.VMCPInstanceManifest{VMCPID: delegationTestVMCP}}}
	require.NoError(t, f.storage.Create(t.Context(), own))
	require.True(t, f.authorizer.Authorize(httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+own.Name+"/reveal", nil), admin))
}

// TestAgentDelegationSetupStaysWhenTheAgentCanNoLongerUseTheVMCP checks that what a user set up through an agent stays
// when the agent can no longer use the vMCP for them, because the vMCP's profile no longer names the agent, or because
// no profile of the agent matches the user any more: the setup routes no longer reach it, and its values stay stored,
// unused, until the delegation is revoked or the vMCP, the agent or the user is deleted.
func TestAgentDelegationSetupStaysWhenTheAgentCanNoLongerUseTheVMCP(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, f *setupFixture)
	}{
		{
			name: "the vMCP profile no longer names the agent",
			change: func(t *testing.T, f *setupFixture) {
				t.Helper()
				var vmcp v1.VMCP
				require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: delegationTestVMCP}, &vmcp))
				vmcp.Spec.Manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "999"}}
				require.NoError(t, f.storage.Update(t.Context(), &vmcp))
			},
		},
		{
			name: "no profile of the agent matches the user",
			change: func(t *testing.T, f *setupFixture) {
				t.Helper()
				agent, err := f.gateway.GetAgent(t.Context(), f.agent)
				require.NoError(t, err)
				agent.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeGroup, ID: "sales"}}
				_, err = f.gateway.UpdateAgent(t.Context(), *agent)
				require.NoError(t, err)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newSetupFixture(t)
			alices := f.delegate(f.alice)
			managed := "/api/me/agent-delegations/" + alices.DelegationID + "/setup"
			decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, managed+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{
				Components: map[string]map[string]*string{"keyed": {"TOKEN": new(setupValue)}},
			}), http.StatusOK)

			tt.change(t, f)

			setup := decode[types.AgentDelegationSetup](t, f.do(f.alice, http.MethodGet, managed, nil), http.StatusOK)
			require.Empty(t, setup.VMCPs)
			requireStatus(t, f.do(f.alice, http.MethodPut, managed+"/vmcps/"+delegationTestVMCP+"/configuration", types.AgentDelegationSetupConfiguration{}), http.StatusNotFound, "")
			require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("keyed", "TOKEN"): setupValue}, f.pairValues(f.alice.GetUID()))
		})
	}
}

// TestAgentDelegationSetupRefusesValuesThatMoveTheTemplateHost checks that a value the user enters through an agent is
// refused when it changes the host a component's URL template pins, as for a user's own instance, and that nothing is
// stored for it.
func TestAgentDelegationSetupRefusesValuesThatMoveTheTemplateHost(t *testing.T) {
	f := newSetupFixture(t)
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: delegationTestVMCP}, &vmcp))
	vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, types.VMCPComponent{
		ID:            "templated",
		Name:          "templated",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TENANT", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${TENANT}.example.com/mcp"},
			Config:       []types.MCPConfig{{Key: "TENANT", Name: "TENANT", Usage: types.Env, Required: true}},
		}},
	})
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))

	alices := f.delegate(f.alice)
	configuration := "/api/me/agent-delegations/" + alices.DelegationID + "/setup/vmcps/" + delegationTestVMCP + "/configuration"
	requireStatus(t, f.do(f.alice, http.MethodPut, configuration, types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{"templated": {"TENANT": new("evil.example/x?")}},
	}), http.StatusBadRequest, "must not change the URL template's scheme or host")
	require.Empty(t, f.pairValues(f.alice.GetUID()))

	decode[types.AgentDelegationSetupVMCP](t, f.do(f.alice, http.MethodPut, configuration, types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{"templated": {"TENANT": new("acme")}},
	}), http.StatusOK)
	require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("templated", "TENANT"): "acme"}, f.pairValues(f.alice.GetUID()))
}
