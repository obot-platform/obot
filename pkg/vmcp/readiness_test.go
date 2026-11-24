package vmcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// readinessAgent7Profile names agent 7, and readinessUsersProfile every user.
	readinessAgent7Profile = "vp1agent7"
	readinessUsersProfile  = "vp1users"
)

// failingGrants is a store of grants that cannot be read.
type failingGrants struct {
	*memoryGrants
}

func (failingGrants) MCPOAuthGrantMetadata(context.Context, []string, []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error) {
	return nil, errors.New("store unavailable")
}

// agentReadiness returns whether agent can use vmcp, which it has not connected to, on a vMCP that holds no grant.
func agentReadiness(t *testing.T, vmcp v1.VMCP, agent kuser.Info) (Readiness, error) {
	t.Helper()
	return readinessWith(t, NewPreconfiguredOAuthStore(newMemoryGrants()), vmcp, agent)
}

// readinessWith returns whether agent can use vmcp, which it has not connected to, with the grants of store.
func readinessWith(t *testing.T, store *PreconfiguredOAuthStore, vmcp v1.VMCP, agent kuser.Info) (Readiness, error) {
	t.Helper()
	report, err := AgentReadiness(t.Context(), store, vmcp, nil, agent)
	return report.Readiness, err
}

// readinessComponent returns a component that needs nothing from its caller.
func readinessComponent(id string) types.VMCPComponent {
	return types.VMCPComponent{ID: id, Name: id, MCPServerCatalogEntryID: id + "-entry"}
}

// configuredComponent returns a component with a required and an optional value a user would enter, and a required
// value the administrator fixed.
func configuredComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Config = []types.MCPConfig{
		{Key: "REGION", Required: true},
		{Key: "VERBOSE"},
		{Key: "ADMIN_TOKEN", Required: true},
	}
	component.Configuration = []types.VMCPConfigurationPolicy{
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
	}
	return component
}

// staticOAuthComponent returns a remote component that requires OAuth with administrator static credentials.
func staticOAuthComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{
		FixedURL:            "https://" + id + ".example.com/mcp",
		StaticOAuthRequired: true,
	}
	return component
}

// staticOAuthConfiguredComponent returns a static OAuth component that also needs a value a user would enter.
func staticOAuthConfiguredComponent(id string) types.VMCPComponent {
	component := staticOAuthComponent(id)
	configured := configuredComponent(id)
	component.CatalogEntry.Manifest.Config = configured.CatalogEntry.Manifest.Config
	component.Configuration = configured.Configuration
	return component
}

// agentNamingProfileFor returns a profile that names the agent agentID and grants every tool of components.
func agentNamingProfileFor(id, agentID string, components ...string) types.VMCPProfile {
	allowed := map[string]types.VMCPComponentSet{}
	for _, component := range components {
		allowed[component] = types.VMCPComponentSet{AllowedTools: []string{"*"}}
	}
	return types.VMCPProfile{
		ID:          id,
		Name:        id,
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: agentID}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: allowed},
	}
}

func readinessVMCP(components []types.VMCPComponent, profiles ...types.VMCPProfile) v1.VMCP {
	return v1.VMCP{
		Name: "vmcp1readiness",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Readiness",
			Components:  components,
			Profiles:    profiles,
		}},
	}
}

func TestAgentReadiness(t *testing.T) {
	agent := agentInfo(7, nil)
	for _, tt := range []struct {
		name        string
		vmcp        v1.VMCP
		wantReasons []ReadinessReason
		wantError   string
	}{
		{
			name: "components that need nothing",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "plain"),
				everyUserProfile(readinessUsersProfile),
			),
		},
		{
			// Nothing supplies the required value a user would enter, so it blocks the agent until the vMCP fixes it.
			// Optional and fixed values do not.
			name: "required configuration",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{configuredComponent("configured")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "configured"),
			),
			wantReasons: []ReadinessReason{{
				ComponentID: "configured",
				Code:        types.AgentReadinessConfigurationRequired,
				Field:       "REGION",
			}},
			wantError: AgentConfigurationRequiredError,
		},
		{
			name: "static OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), readinessComponent("plain")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "oauth", "plain"),
			),
			wantReasons: []ReadinessReason{{
				ComponentID: "oauth",
				Code:        types.AgentReadinessOAuthRequired,
			}},
			wantError: AgentOAuthRequiredError,
		},
		{
			// One reason of each kind is reported as configuration, which an administrator fixes first.
			name: "configuration and OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "oauth", "configured"),
			),
			wantReasons: []ReadinessReason{
				{
					ComponentID: "oauth",
					Code:        types.AgentReadinessOAuthRequired,
				},
				{
					ComponentID: "configured",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// A component that needs both is reported for each.
			name: "configuration and OAuth for one component",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthConfiguredComponent("both")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "both"),
			),
			wantReasons: []ReadinessReason{
				{
					ComponentID: "both",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
				},
				{
					ComponentID: "both",
					Code:        types.AgentReadinessOAuthRequired,
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// An agent named by two profiles is checked for every component either grants: the union, as for a user.
			name: "two profiles that name the agent",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "plain", "configured"),
				agentNamingProfileFor("vp1agent7oauth", "7", "oauth"),
			),
			wantReasons: []ReadinessReason{
				{
					ComponentID: "oauth",
					Code:        types.AgentReadinessOAuthRequired,
				},
				{
					ComponentID: "configured",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// Components that no profile naming the agent grants need no setup, whatever another agent's profile needs.
			name: "components the agent is not granted",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain")},
				agentNamingProfileFor(readinessAgent7Profile, "7", "plain"),
				agentNamingProfileFor("vp1agent8", "8", "oauth", "configured"),
			),
		},
		{
			// A profile for every user never applies to an agent: the components it grants are not the agent's, and
			// the agent, which no profile names, has nothing enabled. The caller refuses it access.
			name: "only a profile for every user",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured")},
				everyUserProfile(readinessUsersProfile),
			),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness, err := agentReadiness(t, tt.vmcp, agent)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(readiness.Reasons, tt.wantReasons) {
				t.Errorf("reasons = %#v, want %#v", readiness.Reasons, tt.wantReasons)
			}
			if readiness.Ready() != (len(tt.wantReasons) == 0) {
				t.Errorf("ready = %v with reasons %v", readiness.Ready(), readiness.Reasons)
			}
			if len(tt.wantReasons) > 0 {
				notReady := &AgentNotReadyError{Readiness: readiness}
				if failure := notReady.Failure(); failure.Error != tt.wantError || !reflect.DeepEqual(failure.Reasons, tt.wantReasons) {
					t.Errorf("failure = %#v, want error %q with the reasons", failure, tt.wantError)
				}
			}
		})
	}
}

// TestAgentReadinessChecksOnlyEnabledComponents checks that readiness reports configuration and OAuth reasons only for
// the components enabled for the agent, selects the vMCP's grants for them with one query, and fails when the grants
// cannot be read.
func TestAgentReadinessChecksOnlyEnabledComponents(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{configuredComponent("configured"), staticOAuthComponent("oauth"), staticOAuthConfiguredComponent("disabled")},
		agentNamingProfileFor(readinessAgent7Profile, "7", "configured", "oauth"),
	)
	instance := v1.VMCPInstance{Name: "vmcpi1agent", Spec: v1.VMCPInstanceSpec{UserID: "agent:7"}}

	grants := newMemoryGrants()
	report, err := AgentReadiness(t.Context(), NewPreconfiguredOAuthStore(grants), vmcp, &instance, agent)
	if err != nil {
		t.Fatal(err)
	}
	want := []ReadinessReason{
		{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"},
		{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired},
	}
	if !reflect.DeepEqual(report.Readiness.Reasons, want) {
		t.Fatalf("reasons = %#v, want %#v", report.Readiness.Reasons, want)
	}
	if report.Readiness.ErrorCode() != AgentConfigurationRequiredError {
		t.Fatalf("error = %q, want %q", report.Readiness.ErrorCode(), AgentConfigurationRequiredError)
	}
	var selected []string
	for _, grant := range report.Grants {
		selected = append(selected, grant.ComponentID)
	}
	if !reflect.DeepEqual(selected, []string{"configured", "oauth"}) {
		t.Fatalf("grants were selected for %v, want only the enabled components", selected)
	}
	if grants.queries != 1 {
		t.Fatalf("readiness read grants %d times, want once for every component", grants.queries)
	}

	if _, err := AgentReadiness(t.Context(), NewPreconfiguredOAuthStore(failingGrants{newMemoryGrants()}), vmcp, &instance, agent); err == nil {
		t.Fatal("a store that cannot be read did not fail readiness")
	}
	if _, err := AgentReadiness(t.Context(), nil, vmcp, &instance, agent); err == nil {
		t.Fatal("readiness was computed without a store of grants")
	}
}

// TestAgentReadinessAgreesWithPreconfiguredOAuthState checks that each agent's readiness reports, for the components
// its profiles grant and no others, the reasons that follow from each component's preconfigured OAuth state, as an
// administrator reads it, and from the configuration agents cannot be given; and that a store that cannot be read fails
// the state rather than reporting a component as not needing OAuth.
func TestAgentReadinessAgreesWithPreconfiguredOAuthState(t *testing.T) {
	vmcp := readinessVMCP(
		[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain"), configuredComponent("users-only")},
		agentNamingProfileFor(readinessAgent7Profile, "7", "plain", "configured"),
		agentNamingProfileFor("vp1agent7oauth", "7", "oauth"),
		agentNamingProfileFor("vp1agent8", "8", "oauth", "plain"),
		everyUserProfile(readinessUsersProfile),
	)
	store := NewPreconfiguredOAuthStore(newMemoryGrants())
	status, err := store.Status(t.Context(), vmcp, "oauth")
	if err != nil {
		t.Fatal(err)
	}
	if want := (types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "oauth", State: types.VMCPPreconfiguredOAuthNeedsAuthorization}); !reflect.DeepEqual(status, want) {
		t.Fatalf("status = %#v, want %#v", status, want)
	}

	oauthRequired := ReadinessReason{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired}
	configurationRequired := ReadinessReason{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}
	for agent, want := range map[uint][]ReadinessReason{
		7: {oauthRequired, configurationRequired},
		8: {oauthRequired},
	} {
		readiness, err := readinessWith(t, store, vmcp, agentInfo(agent, nil))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(readiness.Reasons, want) {
			t.Fatalf("agent %d readiness = %#v, want %#v", agent, readiness.Reasons, want)
		}
	}

	if _, err := NewPreconfiguredOAuthStore(failingGrants{newMemoryGrants()}).Status(t.Context(), vmcp, "oauth"); err == nil {
		t.Fatal("a store that cannot be read did not fail the state")
	}
}

// TestAgentReadinessPreconfiguredComponents checks that readiness lists as preconfigured exactly the components enabled
// for the agent whose grant on the vMCP works: not one whose grant no longer works or was authorized for another URL,
// which is a reason instead, not one the vMCP holds no grant for, and not one that only another agent is granted.
func TestAgentReadinessPreconfiguredComponents(t *testing.T) {
	vmcp := readinessVMCP(
		[]types.VMCPComponent{
			staticOAuthComponent("working"),
			staticOAuthComponent("broken"),
			staticOAuthComponent("moved"),
			staticOAuthComponent("missing"),
			staticOAuthComponent("ungranted"),
			readinessComponent("plain"),
		},
		agentNamingProfileFor(readinessAgent7Profile, "7", "working", "broken"),
		agentNamingProfileFor("vp1agent7more", "7", "moved", "missing", "plain"),
		agentNamingProfileFor("vp1agent8", "8", "working", "ungranted"),
	)
	grants := newMemoryGrants()
	owner := principal.PreconfiguredOAuthOwner(vmcp.Name)
	for _, id := range []string{"working", "broken", "ungranted"} {
		grants.put(owner, PreconfiguredOAuthMCPID(vmcp.Name, id), "grant-"+id, "https://"+id+".example.com/mcp")
	}
	grants.put(owner, PreconfiguredOAuthMCPID(vmcp.Name, "moved"), "grant-moved", "https://elsewhere.example.com/mcp")
	grants.invalidate(owner, PreconfiguredOAuthMCPID(vmcp.Name, "broken"))
	store := NewPreconfiguredOAuthStore(grants)

	report, err := AgentReadiness(t.Context(), store, vmcp, nil, agentInfo(7, nil))
	require.NoError(t, err)
	require.Equal(t, []string{"working"}, report.PreconfiguredComponents())
	require.Equal(t, []ReadinessReason{
		{ComponentID: "broken", Code: types.AgentReadinessOAuthInvalid},
		{ComponentID: "moved", Code: types.AgentReadinessOAuthInvalid},
		{ComponentID: "missing", Code: types.AgentReadinessOAuthRequired},
	}, report.Readiness.Reasons)

	report, err = AgentReadiness(t.Context(), store, vmcp, nil, agentInfo(8, nil))
	require.NoError(t, err)
	require.True(t, report.Readiness.Ready())
	require.Equal(t, []string{"working", "ungranted"}, report.PreconfiguredComponents())

	// An agent that no profile names has nothing enabled, so nothing preconfigured either, and the list is empty
	// rather than nil.
	report, err = AgentReadiness(t.Context(), store, vmcp, nil, agentInfo(9, nil))
	require.NoError(t, err)
	require.Equal(t, []string{}, report.PreconfiguredComponents())
}

func TestAgentReadinessRefusesUsersAndPersonalVMCPs(t *testing.T) {
	vmcp := readinessVMCP([]types.VMCPComponent{readinessComponent("plain")}, everyUserProfile(readinessUsersProfile))
	if _, err := agentReadiness(t, vmcp, userInfo("7", nil)); err == nil {
		t.Fatal("readiness was computed for a user")
	}
	vmcp.Spec.UserID = "7"
	if _, err := agentReadiness(t, vmcp, agentInfo(7, nil)); err == nil {
		t.Fatal("readiness was computed for a personal vMCP")
	}
}

func TestReadinessErrorCodePrecedence(t *testing.T) {
	configuration := ReadinessReason{ComponentID: "one", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}
	oauth := ReadinessReason{ComponentID: "two", Code: types.AgentReadinessOAuthRequired}
	invalid := ReadinessReason{ComponentID: "two", Code: types.AgentReadinessOAuthInvalid}
	for _, tt := range []struct {
		name    string
		reasons []ReadinessReason
		want    string
	}{
		{
			name:    "configuration",
			reasons: []ReadinessReason{configuration},
			want:    AgentConfigurationRequiredError,
		},
		{
			name:    "configuration before OAuth",
			reasons: []ReadinessReason{oauth, configuration, invalid},
			want:    AgentConfigurationRequiredError,
		},
		{
			name:    "OAuth",
			reasons: []ReadinessReason{oauth, invalid},
			want:    AgentOAuthRequiredError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness := Readiness{Reasons: tt.reasons}
			if got := readiness.ErrorCode(); got != tt.want {
				t.Fatalf("ErrorCode() = %q, want %q", got, tt.want)
			}
			if failure := (&AgentNotReadyError{Readiness: readiness}).Failure(); failure.Error != tt.want {
				t.Fatalf("failure error = %q, want %q", failure.Error, tt.want)
			}
		})
	}
}
