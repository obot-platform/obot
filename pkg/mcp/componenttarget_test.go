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
	targetFixedURL = "https://tenant.example.com/fixed"
	targetSeven    = "vp1seven"
	targetEight    = "vp1eight"
	targetUsers    = "vp1users"
)

// targetFixture is a shared vMCP whose components' URLs come from configuration: a remote constrained by hostname whose
// URL the vMCP fixes, one whose URL it does not fix, and one whose URL template uses a value the vMCP fixes and one a
// user would enter. Profiles name agent 7, agent 8 and every user.
type targetFixture struct {
	storage kclient.WithWatch
	gateway *gatewayclient.Client
	sm      *SessionManager
	key     kclient.ObjectKey
}

func newTargetFixture(t *testing.T) *targetFixture {
	t.Helper()
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	hostnameRemote := func(id string) types.VMCPComponent {
		return types.VMCPComponent{
			ID:   id,
			Name: id,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{Hostname: targetHostname},
				// An optional value a user would enter, which agents are never given, keeps the server dedicated.
				Config: []types.MCPConfig{{Key: "VERBOSE", Usage: types.Interpolated}},
			}},
			Configuration: []types.VMCPConfigurationPolicy{{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		}
	}
	hosted := hostnameRemote("hosted")
	hosted.Configuration = append(hosted.Configuration, types.VMCPConfigurationPolicy{Key: "__url", Policy: types.VMCPConfigurationPolicyFixed})
	vmcp := &v1.VMCP{Name: "vmcp1targets", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			hosted,
			hostnameRemote("unfixed"),
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
	f := &targetFixture{storage: storage, gateway: newGrantTestGateway(t), key: kclient.ObjectKeyFromObject(vmcp)}
	f.sm = &SessionManager{storageClient: storage, gatewayClient: f.gateway}
	f.fixURL(t, targetFixedURL)
	return f
}

// fixURL sets the values the vMCP fixes for every connection: url for the hostname-constrained component, and the
// region of the templated one.
func (f *targetFixture) fixURL(t *testing.T, url string) {
	t.Helper()
	vmcp := f.vmcp(t)
	require.NoError(t, f.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpaccess.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpaccess.StaticConfigurationCredentialName(&vmcp),
		Secrets: map[string]string{
			vmcpaccess.ConfigurationKey("hosted", "__url"):     url,
			vmcpaccess.ConfigurationKey("templated", "REGION"): "eu",
		},
	}))
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

// TestComponentTargetResolvesFixedURLs checks the URL an agent's connection to a component reaches: the one built
// from the values the vMCP fixes. A value a user would enter is never one of them, so a URL that needs one, or a
// hostname-constrained URL the vMCP does not fix, is missing, with the keys it needs.
func TestComponentTargetResolvesFixedURLs(t *testing.T) {
	f := newTargetFixture(t)
	target := func(componentID string) vmcpaccess.ComponentTarget {
		t.Helper()
		result, err := f.sm.ComponentTarget(t.Context(), f.vmcp(t), f.component(t, componentID))
		require.NoError(t, err)
		return result
	}

	require.Equal(t, vmcpaccess.ComponentTarget{URL: targetFixedURL}, target("hosted"))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"__url"}}, target("unfixed"))
	require.Equal(t, vmcpaccess.ComponentTarget{Missing: []string{"TENANT"}}, target("templated"))

	// Once the vMCP fixes the value, the URL is built from it.
	vmcp := f.vmcp(t)
	vmcp.Spec.Manifest.Components[2].Configuration[0].Policy = types.VMCPConfigurationPolicyFixed
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	require.NoError(t, f.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpaccess.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpaccess.StaticConfigurationCredentialName(&vmcp),
		Secrets: map[string]string{
			vmcpaccess.ConfigurationKey("hosted", "__url"):     targetFixedURL,
			vmcpaccess.ConfigurationKey("templated", "REGION"): "eu",
			vmcpaccess.ConfigurationKey("templated", "TENANT"): "acme",
		},
	}))
	require.Equal(t, vmcpaccess.ComponentTarget{URL: "https://eu.example.com/acme/mcp"}, target("templated"))

	// A grant is bound to the same URL.
	resolved, refused, err := vmcpaccess.ResolveComponentTarget(t.Context(), f.sm, f.vmcp(t), f.component(t, "hosted"))
	require.NoError(t, err)
	require.NoError(t, refused)
	require.Equal(t, targetFixedURL, resolved.URL)
}

// TestComponentTargetRefusesInvalidURLs checks that a fixed URL that no longer satisfies the component's hostname is
// refused, without the URL.
func TestComponentTargetRefusesInvalidURLs(t *testing.T) {
	f := newTargetFixture(t)

	// The catalog entry's hostname changes after the vMCP fixed the URL.
	vmcp := f.vmcp(t)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.Hostname = "elsewhere.example.com"
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	_, err := f.sm.ComponentTarget(t.Context(), f.vmcp(t), f.component(t, "hosted"))
	var httpErr *types.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error %v", err)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.NotContains(t, httpErr.Message, targetFixedURL)
}

// TestComponentTargetChecksTheTemplateHost checks that a URL template is expanded with the same host check as the
// MCPServer controller applies (resolvedComponentURL): on a shared vMCP, which only admins create, a fixed value that
// forms the domain is text the admin wrote, but on a personal vMCP, whose owner may be any user, the same value is
// refused, without the value.
func TestComponentTargetChecksTheTemplateHost(t *testing.T) {
	f := newTargetFixture(t)
	fix := func(vmcp v1.VMCP) {
		t.Helper()
		require.NoError(t, f.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: vmcpaccess.StaticConfigurationCredentialContext(vmcp.Name),
			Name:    vmcpaccess.StaticConfigurationCredentialName(&vmcp),
			Secrets: map[string]string{
				vmcpaccess.ConfigurationKey("templated", "TENANT"): "acme",
				vmcpaccess.ConfigurationKey("templated", "REGION"): "datadoghq.eu",
			},
		}))
	}
	vmcp := f.vmcp(t)
	vmcp.Spec.Manifest.Components[2].CatalogEntry.Manifest.RemoteConfig.URLTemplate = "https://${TENANT}.mcp.${REGION}/mcp"
	vmcp.Spec.Manifest.Components[2].Configuration[0].Policy = types.VMCPConfigurationPolicyFixed
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	fix(f.vmcp(t))
	result, err := f.sm.ComponentTarget(t.Context(), f.vmcp(t), f.component(t, "templated"))
	require.NoError(t, err)
	require.Equal(t, vmcpaccess.ComponentTarget{URL: "https://acme.mcp.datadoghq.eu/mcp"}, result)

	vmcp = f.vmcp(t)
	vmcp.Spec.UserID = "1"
	require.NoError(t, f.storage.Update(t.Context(), &vmcp))
	fix(f.vmcp(t))
	_, err = f.sm.ComponentTarget(t.Context(), f.vmcp(t), f.component(t, "templated"))
	var httpErr *types.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error %v", err)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.NotContains(t, httpErr.Message, "datadoghq.eu")
}

// TestAgentReadinessComparesGrantURLsWithTheAgentsURL checks that readiness refuses the vMCP's grant when it was
// authorized for another URL than the one agents' connections reach, as the MCP gateway does: changing the URL the vMCP
// fixes after the grant was authorized makes it unusable, for every agent granted the component, until it is
// authorized again. The component's preconfigured OAuth state reports the same.
func TestAgentReadinessComparesGrantURLsWithTheAgentsURL(t *testing.T) {
	f := newTargetFixture(t)
	grants := vmcpaccess.NewPreconfiguredOAuthStore(f.gateway)
	grants.SetComponentTargets(f.sm)
	readiness := func(agent kuser.Info) []vmcpaccess.ReadinessReason {
		t.Helper()
		vmcp := f.vmcp(t)
		// Only the component whose URL the vMCP fixes, so configuration readiness reports nothing else.
		vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[:1]
		result, err := vmcpaccess.AgentReadiness(t.Context(), grants, vmcp, nil, agent)
		require.NoError(t, err)
		return result.Readiness.Reasons
	}
	// hostedOAuth is the hosted component's preconfigured OAuth state, as an administrator reads it.
	hostedOAuth := func() types.VMCPPreconfiguredOAuth {
		t.Helper()
		status, err := grants.Status(t.Context(), f.vmcp(t), "hosted")
		require.NoError(t, err)
		return status
	}
	authorize := func(url string) {
		t.Helper()
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), principal.PreconfiguredOAuthOwner(f.key.Name), vmcpaccess.PreconfiguredOAuthMCPID(f.key.Name, "hosted"), url, "grant-"+url, "1",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", Expiry: time.Now().Add(time.Hour)}))
	}

	// One grant, authorized once on the vMCP, makes every agent granted the component ready.
	authorize(targetFixedURL)
	require.Empty(t, readiness(asAgent(7, 11)))
	require.Empty(t, readiness(asAgent(8, 12)))

	// Changing the URL the vMCP fixes after the grant was authorized makes the grant unusable for both, and the agent
	// component's preconfigured OAuth state says so, without the URL.
	const moved = "https://tenant.example.com/moved"
	f.fixURL(t, moved)
	invalid := []vmcpaccess.ReadinessReason{{ComponentID: "hosted", Code: types.AgentReadinessOAuthInvalid}}
	require.Equal(t, invalid, readiness(asAgent(7, 11)))
	require.Equal(t, invalid, readiness(asAgent(8, 12)))
	hosted := hostedOAuth()
	require.Equal(t, types.VMCPPreconfiguredOAuthInvalid, hosted.State)
	require.Equal(t, vmcpaccess.GrantTargetMismatch, hosted.InvalidReason)
	require.NotContains(t, hosted.InvalidReason, "tenant.example.com")
	require.Empty(t, hosted.Target, "a configured URL is returned")
	require.Equal(t, targetHostname, hosted.TargetHostname)

	// Authorizing it again for the URL the vMCP now fixes fixes it for both.
	authorize(moved)
	require.Empty(t, readiness(asAgent(7, 11)))
	require.Empty(t, readiness(asAgent(8, 12)))
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, hostedOAuth().State)
}

// TestComponentHopRefusesAGrantForAnotherURL connects agent 7 to its dedicated server of a hostname-constrained
// component, whose URL the vMCP fixes: the vMCP's grant, authorized for another URL, is refused, naming the vMCP and the
// component and not the URL, and never used; the grant for the URL the vMCP fixes is used.
func TestComponentHopRefusesAGrantForAnotherURL(t *testing.T) {
	f := newTargetFixture(t)
	grants := vmcpaccess.NewPreconfiguredOAuthStore(f.gateway)
	grants.SetComponentTargets(f.sm)
	f.sm.SetPreconfiguredOAuth(grants)
	f.sm.backend = &kubernetesBackend{}
	f.sm.baseURL = "https://obot.example.com"
	f.sm.remoteURLValidationConfig = RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}
	t.Cleanup(f.sm.Close)

	// The agent's instance and its dedicated server, configured with the URL the vMCP fixes, as the controllers
	// configure them.
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
		Secrets: map[string]string{"__url": targetFixedURL},
	}))
	owner, mcpID := principal.PreconfiguredOAuthOwner(f.key.Name), vmcpaccess.PreconfiguredOAuthMCPID(f.key.Name, "hosted")
	authorize := func(url string) {
		t.Helper()
		require.NoError(t, f.gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, url, "grant-"+url, "1",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", Expiry: time.Now().Add(time.Hour)}))
	}

	// The grant was authorized before the vMCP's URL was changed.
	authorize("https://tenant.example.com/old")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, err = f.sm.ServerForAction(ctx, server.Name, asAgent(7, 11))
	grantErr, ok := errors.AsType[*PreconfiguredOAuthError](err)
	require.True(t, ok, "connect error = %v, want an PreconfiguredOAuthError", err)
	require.Equal(t, f.key.Name, grantErr.VMCPID)
	require.Equal(t, "hosted", grantErr.ComponentID)
	require.Equal(t, vmcpaccess.GrantTargetMismatch, grantErr.Reason)
	require.NotContains(t, grantErr.Error(), "tenant.example.com", "a configured URL is reported")

	authorize(targetFixedURL)
	_, cfg, err := f.sm.ServerForAction(ctx, server.Name, asAgent(7, 11))
	require.NoError(t, err)
	require.Equal(t, targetFixedURL, cfg.URL)
	require.Equal(t, &OAuthGrantRef{ComponentID: "hosted", Owner: owner, MCPID: mcpID}, cfg.OAuthGrant)
}
