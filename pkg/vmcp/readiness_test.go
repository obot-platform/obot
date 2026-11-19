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
	return readinessWith(t, NewAgentOAuthGrantStore(newMemoryGrants()), vmcp, agent)
}

// readinessWith returns whether agent can use vmcp, which it has not connected to, with the grants of store.
func readinessWith(t *testing.T, store *AgentOAuthGrantStore, vmcp v1.VMCP, agent kuser.Info) (Readiness, error) {
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
	report, err := AgentReadiness(t.Context(), NewAgentOAuthGrantStore(grants), vmcp, &instance, agent)
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

	if _, err := AgentReadiness(t.Context(), NewAgentOAuthGrantStore(failingGrants{newMemoryGrants()}), vmcp, &instance, agent); err == nil {
		t.Fatal("a store that cannot be read did not fail readiness")
	}
	if _, err := AgentReadiness(t.Context(), nil, vmcp, &instance, agent); err == nil {
		t.Fatal("readiness was computed without a store of grants")
	}
}

// TestAgentReadinessFollowsTheProfilesThatNameIt checks that an agent's readiness covers the union of the components
// its profiles grant, and changes with them: naming the agent in a second profile adds that profile's components and
// what they need, the vMCP's grant for a component serves the agent whichever profile grants it the component, and
// removing the agent from that profile takes away only what that profile granted.
func TestAgentReadinessFollowsTheProfilesThatNameIt(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain")},
		agentNamingProfileFor(readinessAgent7Profile, "7", "plain"),
		agentNamingProfileFor("vp1second", "8", "oauth", "configured"),
	)
	grants := newMemoryGrants()
	store := NewAgentOAuthGrantStore(grants)
	readiness := func() []ReadinessReason {
		t.Helper()
		readiness, err := readinessWith(t, store, vmcp, agent)
		if err != nil {
			t.Fatal(err)
		}
		return readiness.Reasons
	}
	if got := readiness(); got != nil {
		t.Fatalf("reasons before the second profile names the agent = %v, want none", got)
	}

	// Named by the second profile too, the agent is granted its components, and needs what they need.
	vmcp.Spec.Manifest.Profiles[1].Subjects = append(vmcp.Spec.Manifest.Profiles[1].Subjects, types.Subject{Type: types.SubjectTypeAgent, ID: "7"})
	want := []ReadinessReason{
		{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired},
		{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"},
	}
	if got := readiness(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reasons in two profiles = %#v, want %#v", got, want)
	}

	// The vMCP's grant for the component serves the agent, as it serves agent 8.
	grants.put(principal.AgentOAuthGrantOwner(vmcp.Name), AgentOAuthGrantMCPID(vmcp.Name, "oauth"), "grant", "https://oauth.example.com/mcp")
	want = want[1:]
	if got := readiness(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reasons with the vMCP's grant = %#v, want %#v", got, want)
	}

	// Removed from the second profile, the agent keeps only what the first grants it.
	vmcp.Spec.Manifest.Profiles[1].Subjects = vmcp.Spec.Manifest.Profiles[1].Subjects[:1]
	if got := readiness(); got != nil {
		t.Fatalf("reasons after leaving the second profile = %v, want none", got)
	}
}

// TestAgentSetup checks the vMCP's agent setup: it lists each component that a profile naming an agent grants, once and
// in manifest order, whichever agents' profiles grant it, and no component that only a profile for every user grants,
// with the reasons agent readiness reports for it, so that every agent's readiness is the setup's reasons for the
// components the agent is granted. A vMCP whose profiles name no agent, and a personal vMCP, have none.
func TestAgentSetup(t *testing.T) {
	vmcp := readinessVMCP(
		[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain"), configuredComponent("users-only")},
		agentNamingProfileFor(readinessAgent7Profile, "7", "plain", "configured"),
		agentNamingProfileFor("vp1agent7oauth", "7", "oauth"),
		agentNamingProfileFor("vp1agent8", "8", "oauth", "plain"),
		everyUserProfile(readinessUsersProfile),
	)
	store := NewAgentOAuthGrantStore(newMemoryGrants())
	setup, err := AgentSetup(t.Context(), store, vmcp)
	if err != nil {
		t.Fatal(err)
	}
	want := types.VMCPAgentSetup{
		VMCPID: vmcp.Name,
		Components: []types.VMCPAgentSetupComponent{
			{
				ComponentID: "oauth",
				Reasons:     []types.AgentReadinessReason{{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired}},
				OAuth:       &types.VMCPAgentOAuthStatus{Required: true, State: types.VMCPAgentOAuthMissing},
			},
			{
				ComponentID: "configured",
				Reasons:     []types.AgentReadinessReason{{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}},
			},
			{
				ComponentID: "plain",
				Ready:       true,
				Reasons:     []types.AgentReadinessReason{},
			},
		},
	}
	if !reflect.DeepEqual(setup, want) {
		t.Fatalf("setup = %#v, want %#v", setup, want)
	}

	// Each agent's readiness is the setup's reasons for the components it is granted.
	for _, agent := range []uint{7, 8} {
		info := agentInfo(agent, nil)
		var granted []ReadinessReason
		for _, component := range setup.Components {
			if len(EnabledComponents(info, vmcp, []types.VMCPComponent{{ID: component.ComponentID}})) == 1 {
				granted = append(granted, component.Reasons...)
			}
		}
		readiness, err := readinessWith(t, store, vmcp, info)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(readiness.Reasons, granted) {
			t.Fatalf("agent %d readiness = %#v, want the setup's reasons %#v", agent, readiness.Reasons, granted)
		}
	}

	usersOnly := readinessVMCP(vmcp.Spec.Manifest.Components, everyUserProfile(readinessUsersProfile))
	personal := vmcp
	personal.Spec.UserID = "1"
	for name, other := range map[string]v1.VMCP{"no profile names an agent": usersOnly, "personal": personal} {
		setup, err := AgentSetup(t.Context(), store, other)
		if err != nil {
			t.Fatal(err)
		}
		if len(setup.Components) != 0 {
			t.Fatalf("%s: setup lists %v, want nothing", name, setup.Components)
		}
	}

	if _, err := AgentSetup(t.Context(), NewAgentOAuthGrantStore(failingGrants{newMemoryGrants()}), vmcp); err == nil {
		t.Fatal("a store that cannot be read did not fail the setup")
	}
	if _, err := AgentSetup(t.Context(), nil, vmcp); err == nil {
		t.Fatal("the setup was computed without a store of grants")
	}
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
