package mcp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	targetHostname = "tenant.example.com"
	targetSeven    = "vp1seven"
	targetEight    = "vp1eight"
	targetUsers    = "vp1users"
)

// targetFixture is a shared vMCP whose components' URLs come from configuration: a remote constrained by hostname, and
// one whose URL template uses a value a user would enter and one the vMCP fixes. Profiles for agent 7, agent 8 and
// every user configure them through the profile configuration store.
type targetFixture struct {
	storage       kclient.WithWatch
	gateway       *gatewayclient.Client
	configuration *vmcpaccess.ProfileConfigurationStore
	sm            *SessionManager
	key           kclient.ObjectKey
}

func newTargetFixture(t *testing.T) *targetFixture {
	t.Helper()
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: "vmcp1targets", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{
				ID:   "hosted",
				Name: "hosted",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{Hostname: targetHostname},
				}},
			},
			{
				ID:   "templated",
				Name: "templated",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${REGION}.example.com/${TENANT}/mcp"},
					Config:       []types.MCPConfig{{Key: "TENANT", Required: true}, {Key: "REGION", Required: true}},
				}},
				Configuration: []types.VMCPConfigurationPolicy{
					{Key: "TENANT", Policy: types.VMCPConfigurationPolicyUserAllowed},
					{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed},
				},
			},
		},
		Profiles: []types.VMCPProfile{
			profile(targetSeven, types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile(targetEight, types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
			profile(targetUsers, types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		},
	}}}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(vmcp).Build()
	gateway := newGrantTestGateway(t)
	// The value the vMCP fixes for every connection.
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpaccess.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpaccess.StaticConfigurationCredentialName(vmcp),
		Secrets: map[string]string{vmcpaccess.ConfigurationKey("templated", "REGION"): "eu"},
	}))
	sm := &SessionManager{storageClient: storage, gatewayClient: gateway}
	return &targetFixture{storage: storage, gateway: gateway, configuration: vmcpaccess.NewProfileConfigurationStore(gateway), sm: sm, key: kclient.ObjectKeyFromObject(vmcp)}
}

// configure sets values of a profile's configuration by component and then key; nil clears a key.
func (f *targetFixture) configure(t *testing.T, profileID string, values map[string]map[string]*string) {
	t.Helper()
	_, err := f.configuration.Update(t.Context(), f.storage, f.key, profileID, values)
	require.NoError(t, err)
}

func (f *targetFixture) vmcp(t *testing.T) v1.VMCP {
	t.Helper()
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	return vmcp
}

func (f *targetFixture) component(t *testing.T, id string) types.VMCPComponent {
	t.Helper()
	for _, component := range vmcpaccess.ComponentsForInstance(f.vmcp(t), v1.VMCPInstance{}) {
		if component.ID == id {
			return component
		}
	}
	t.Fatalf("no component %s", id)
	return types.VMCPComponent{}
}

func (f *targetFixture) profile(t *testing.T, id string) types.VMCPProfile {
	t.Helper()
	for _, profile := range f.vmcp(t).Spec.Manifest.Profiles {
		if profile.ID == id {
			return profile
		}
	}
	t.Fatalf("no profile %s", id)
	return types.VMCPProfile{}
}

// TestComponentTargetResolvesProfileURLs checks the URL a component's connection reaches for an agent, from its
// profile's configuration and the vMCP's fixed values: an agent's URL never comes from another agent's profile, or
// from the profile that names every user, and what is missing is reported.
func TestComponentTargetResolvesProfileURLs(t *testing.T) {
	f := newTargetFixture(t)
	target := func(agent kuser.Info, componentID string) vmcpaccess.ComponentTarget {
		t.Helper()
		vmcp := f.vmcp(t)
		result, err := f.sm.ComponentTarget(t.Context(), vmcp, vmcpaccess.AgentProfile(agent, vmcp.Spec.Manifest), f.component(t, componentID))
		require.NoError(t, err)
		return result
	}
	seven, eight := asAgent(7, 11), asAgent(8, 12)

	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"__url"}}, target(seven, "hosted"))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"TENANT"}}, target(seven, "templated"))

	// The profile that names every user cannot be configured for agents, and configuration it kept from before agents
	// were named only by ID reaches no agent.
	_, err := f.configuration.Update(t.Context(), f.storage, f.key, targetUsers, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/shared")}})
	require.ErrorContains(t, err, "names no agent")
	kept := f.vmcp(t)
	kept.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{targetUsers: {
		Name: "configuration-users",
		Keys: []string{vmcpaccess.ConfigurationKey("hosted", "__url"), vmcpaccess.ConfigurationKey("templated", "TENANT")},
	}}
	require.NoError(t, f.storage.Status().Update(t.Context(), &kept))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"__url"}}, target(seven, "hosted"))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"TENANT"}}, target(eight, "templated"))

	// Each agent reaches the URL its own profile configures, merged with the value the vMCP fixes.
	f.configure(t, targetSeven, map[string]map[string]*string{
		"hosted":    {"__url": new("https://tenant.example.com/seven")},
		"templated": {"TENANT": new("seven")},
	})
	require.Equal(t, vmcpaccess.ComponentTarget{URL: "https://tenant.example.com/seven", Configured: true}, target(seven, "hosted"))
	require.Equal(t, vmcpaccess.ComponentTarget{URL: "https://eu.example.com/seven/mcp", Configured: true}, target(seven, "templated"))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"__url"}}, target(eight, "hosted"))

	// A profile's grant is bound to the URL its own agents reach.
	resolved, refused, err := vmcpaccess.ProfileGrantTarget(t.Context(), f.sm, f.vmcp(t), f.profile(t, targetSeven), f.component(t, "hosted"))
	require.NoError(t, err)
	require.NoError(t, refused)
	require.Equal(t, "https://tenant.example.com/seven", resolved.URL)
}

// TestComponentTargetRefusesInvalidURLs checks that a configured URL that no longer satisfies the component's hostname
// is refused.
func TestComponentTargetRefusesInvalidURLs(t *testing.T) {
	f := newTargetFixture(t)
	f.configure(t, targetSeven, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/seven")}})

	// The catalog entry's hostname changes after the profile was configured.
	vmcp := f.vmcp(t)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.Hostname = "elsewhere.example.com"
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	vmcp = f.vmcp(t)
	_, err := f.sm.ComponentTarget(t.Context(), vmcp, vmcpaccess.AgentProfile(asAgent(7, 11), vmcp.Spec.Manifest), f.component(t, "hosted"))
	var httpErr *types.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error %v", err)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
}

// TestComponentTargetRefusesValuesThatMoveTheTemplateHost checks that a URL template is expanded with the same host
// check as the MCPServer controller and the runtime apply: a profile's value fills a host label the template leaves to
// it, but one that ends the host early and moves the connection to another host is refused.
func TestComponentTargetRefusesValuesThatMoveTheTemplateHost(t *testing.T) {
	f := newTargetFixture(t)
	vmcp := f.vmcp(t)
	vmcp.Spec.Manifest.Components[1].CatalogEntry.Manifest.RemoteConfig.URLTemplate = "https://${TENANT}.${REGION}.example.com/mcp"
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	target := func() (vmcpaccess.ComponentTarget, error) {
		vmcp := f.vmcp(t)
		return f.sm.ComponentTarget(t.Context(), vmcp, vmcpaccess.AgentProfile(asAgent(7, 11), vmcp.Spec.Manifest), f.component(t, "templated"))
	}

	f.configure(t, targetSeven, map[string]map[string]*string{"templated": {"TENANT": new("acme")}})
	result, err := target()
	require.NoError(t, err)
	require.Equal(t, vmcpaccess.ComponentTarget{URL: "https://acme.eu.example.com/mcp", Configured: true}, result)

	f.configure(t, targetSeven, map[string]map[string]*string{"templated": {"TENANT": new("evil.example/x?")}})
	_, err = target()
	var httpErr *types.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error %v", err)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.NotContains(t, httpErr.Message, "evil.example")
}

// TestAgentReadinessComparesGrantURLsWithTheAgentsURL checks that readiness refuses a grant that was authorized for
// another URL than the one the agent's connection reaches, as the MCP gateway does: changing a profile's URL
// configuration after its grant was authorized makes the grant unusable until it is authorized again, and an agent
// never uses another profile's grant.
func TestAgentReadinessComparesGrantURLsWithTheAgentsURL(t *testing.T) {
	f := newTargetFixture(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(f.gateway)
	grants.SetComponentTargets(f.sm)
	readiness := func(agent kuser.Info) []vmcpaccess.ReadinessReason {
		t.Helper()
		vmcp := f.vmcp(t)
		// Only the hostname-constrained component, so configuration readiness reports nothing else.
		vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[:1]
		result, err := vmcpaccess.AgentReadiness(t.Context(), grants, vmcp, nil, agent)
		require.NoError(t, err)
		return result.Readiness.Reasons
	}
	authorize := func(profileID, url string) {
		t.Helper()
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.ProfileGrant(f.key.Name, profileID), vmcpaccess.ProfileOAuthGrantMCPID(f.key.Name, "hosted"), url, "grant-"+profileID, "1",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", Expiry: time.Now().Add(time.Hour)}))
	}

	f.configure(t, targetSeven, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/seven")}})
	f.configure(t, targetEight, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/eight")}})
	authorize(targetSeven, "https://tenant.example.com/seven")
	require.Empty(t, readiness(asAgent(7, 11)))
	// Agent 8 holds no grant of its own, and agent 7's makes the component one that needs OAuth.
	require.Equal(t, []vmcpaccess.ReadinessReason{{ComponentID: "hosted", Code: types.AgentReadinessOAuthRequired, ProfileID: targetEight}}, readiness(asAgent(8, 12)))
	authorize(targetEight, "https://tenant.example.com/eight")
	require.Empty(t, readiness(asAgent(8, 12)))

	// Changing a profile's URL after its grant was authorized makes the grant unusable for its agents, and its status
	// says so.
	f.configure(t, targetSeven, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/moved")}})
	require.Equal(t, []vmcpaccess.ReadinessReason{{ComponentID: "hosted", Code: types.AgentReadinessOAuthInvalid, ProfileID: targetSeven}}, readiness(asAgent(7, 11)))
	require.Empty(t, readiness(asAgent(8, 12)))
	status, err := grants.ProfileOAuthStatus(t.Context(), f.vmcp(t), targetSeven)
	require.NoError(t, err)
	require.Equal(t, types.VMCPProfileOAuthInvalid, status["hosted"].State)
	require.Contains(t, status["hosted"].InvalidReason, "changed since its grant was authorized")
	status, err = grants.ProfileOAuthStatus(t.Context(), f.vmcp(t), targetEight)
	require.NoError(t, err)
	require.Equal(t, types.VMCPProfileOAuthAuthorized, status["hosted"].State)

	// Authorizing it again for the URL the profile now configures fixes it.
	authorize(targetSeven, "https://tenant.example.com/moved")
	require.Empty(t, readiness(asAgent(7, 11)))
}

// TestComponentHopRefusesAGrantForAnotherURL connects agent 7 to its dedicated server of a hostname-constrained
// component, whose URL comes from its profile's configuration: the profile's grant, authorized for another URL, is
// refused, naming the profile and not the URL, and never used; the grant for the agent's own URL is used.
func TestComponentHopRefusesAGrantForAnotherURL(t *testing.T) {
	f := newTargetFixture(t)
	grants := vmcpaccess.NewProfileOAuthGrantStore(f.gateway)
	grants.SetComponentTargets(f.sm)
	f.sm.SetProfileOAuthGrants(grants)
	f.sm.backend = &kubernetesBackend{}
	f.sm.baseURL = "https://obot.example.com"
	f.sm.remoteURLValidationConfig = RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}
	t.Cleanup(f.sm.Close)
	f.configure(t, targetSeven, map[string]map[string]*string{"hosted": {"__url": new("https://tenant.example.com/seven")}})

	// The agent's instance and its dedicated server, configured with the URL its profile resolves to, as the
	// controllers configure them.
	instance := &v1.VMCPInstance{Name: "vmcpi1seven", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: "agent:7", Manifest: types.VMCPInstanceManifest{VMCPID: f.key.Name}}}
	require.NoError(t, f.storage.Create(t.Context(), instance))
	component := f.component(t, "hosted")
	manifest, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true)
	require.NoError(t, err)
	server := &v1.MCPServer{Name: "ms1vmcpi1seven-hosted", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
		Manifest:        manifest,
		UserID:          "agent:7",
		VMCPInstanceID:  instance.Name,
		VMCPComponentID: "hosted",
	}}
	require.NoError(t, f.storage.Create(t.Context(), server))
	require.NoError(t, f.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: server.CredentialContext(server.Spec.UserID),
		Name:    server.Name,
		Secrets: map[string]string{"__url": "https://tenant.example.com/seven"},
	}))
	authorize := func(profileID, url string) {
		t.Helper()
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.ProfileGrant(f.key.Name, profileID), vmcpaccess.ProfileOAuthGrantMCPID(f.key.Name, "hosted"), url, "grant-"+profileID, "1",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", Expiry: time.Now().Add(time.Hour)}))
	}

	// The grant was authorized before the profile's URL was changed.
	authorize(targetSeven, "https://tenant.example.com/old")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, err = f.sm.ServerForAction(ctx, server.Name, asAgent(7, 11))
	grantErr, ok := errors.AsType[*AgentOAuthGrantError](err)
	require.True(t, ok, "connect error = %v, want an AgentOAuthGrantError", err)
	require.Equal(t, targetSeven, grantErr.ProfileID)
	require.Contains(t, grantErr.Reason, targetSeven)
	require.Contains(t, grantErr.Reason, "changed since its grant was authorized")
	require.NotContains(t, grantErr.Reason, "tenant.example.com", "a configured URL is reported")

	authorize(targetSeven, "https://tenant.example.com/seven")
	_, cfg, err := f.sm.ServerForAction(ctx, server.Name, asAgent(7, 11))
	require.NoError(t, err)
	require.Equal(t, "https://tenant.example.com/seven", cfg.URL)
	require.NotNil(t, cfg.OAuthGrant)
	require.Equal(t, targetSeven, cfg.OAuthGrant.ProfileID)
}
