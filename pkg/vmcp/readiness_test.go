package vmcp

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// readinessAgentProfile names agent 7, and readinessUsersProfile every user.
	readinessAgentProfile = "vp1explicit"
	readinessUsersProfile = "vp1users"
)

// failingGrants is a store of grants that cannot be read.
type failingGrants struct {
	*memoryGrants
}

func (failingGrants) MCPOAuthGrantMetadata(context.Context, []string, []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error) {
	return nil, errors.New("store unavailable")
}

// agentReadiness returns whether agent can use vmcp, which it has not connected to, on a vMCP whose profiles hold no
// grant.
func agentReadiness(t *testing.T, vmcp v1.VMCP, agent kuser.Info) (Readiness, error) {
	t.Helper()
	return readinessWith(t, NewProfileOAuthGrantStore(newMemoryGrants()), vmcp, agent)
}

// readinessWith returns whether agent can use vmcp, which it has not connected to, with the grants of store.
func readinessWith(t *testing.T, store *ProfileOAuthGrantStore, vmcp v1.VMCP, agent kuser.Info) (Readiness, error) {
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

func agentProfile(id, agentID string, components ...string) types.VMCPProfile {
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
		wantProfile string
		wantReasons []ReadinessReason
		wantError   string
	}{
		{
			name: "components that need nothing",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain")},
				agentProfile(readinessAgentProfile, "7", "plain"),
				everyUserProfile(readinessUsersProfile),
			),
			wantProfile: readinessAgentProfile,
		},
		{
			// The profile does not set the required value a user would enter, so it blocks the agent, fixed in the
			// agent's profile. Optional and fixed values do not.
			name: "required configuration",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{configuredComponent("configured")},
				agentProfile(readinessAgentProfile, "7", "configured"),
			),
			wantProfile: readinessAgentProfile,
			wantReasons: []ReadinessReason{{
				ComponentID: "configured",
				Code:        types.AgentReadinessConfigurationRequired,
				Field:       "REGION",
				ProfileID:   readinessAgentProfile,
			}},
			wantError: AgentConfigurationRequiredError,
		},
		{
			name: "static OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), readinessComponent("plain")},
				agentProfile(readinessAgentProfile, "7", "oauth", "plain"),
			),
			wantProfile: readinessAgentProfile,
			wantReasons: []ReadinessReason{{
				ComponentID: "oauth",
				Code:        types.AgentReadinessOAuthRequired,
				ProfileID:   readinessAgentProfile,
			}},
			wantError: AgentOAuthRequiredError,
		},
		{
			// One reason of each kind is reported as configuration, which an administrator fixes first.
			name: "configuration and OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured")},
				agentProfile(readinessAgentProfile, "7", "oauth", "configured"),
			),
			wantProfile: readinessAgentProfile,
			wantReasons: []ReadinessReason{
				{
					ComponentID: "oauth",
					Code:        types.AgentReadinessOAuthRequired,
					ProfileID:   readinessAgentProfile,
				},
				{
					ComponentID: "configured",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
					ProfileID:   readinessAgentProfile,
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// A component that needs both is reported for each.
			name: "configuration and OAuth for one component",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthConfiguredComponent("both")},
				agentProfile(readinessAgentProfile, "7", "both"),
			),
			wantProfile: readinessAgentProfile,
			wantReasons: []ReadinessReason{
				{
					ComponentID: "both",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
					ProfileID:   readinessAgentProfile,
				},
				{
					ComponentID: "both",
					Code:        types.AgentReadinessOAuthRequired,
					ProfileID:   readinessAgentProfile,
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// Components no profile enables for the agent need no setup.
			name: "disabled components",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain")},
				agentProfile(readinessAgentProfile, "7", "plain"),
				agentProfile("vp1otheragent", "8", "oauth", "configured"),
			),
			wantProfile: readinessAgentProfile,
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
			if readiness.ProfileID != tt.wantProfile {
				t.Errorf("profile = %q, want %q", readiness.ProfileID, tt.wantProfile)
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

// TestAgentNamedInNoProfileHasNothingEnabled checks that a profile for every user grants users what it always did, and
// an agent nothing, even when it is the only profile of the vMCP.
func TestAgentNamedInNoProfileHasNothingEnabled(t *testing.T) {
	vmcp := readinessVMCP(
		[]types.VMCPComponent{readinessComponent("one"), readinessComponent("two")},
		everyUserProfile(readinessUsersProfile),
		agentProfile(readinessAgentProfile, "7", "one"),
	)
	components := ComponentsForInstance(vmcp, v1.VMCPInstance{})
	// EnabledComponents removes components in place, so each caller gets its own copy.
	if got := EnabledComponents(userInfo("7", nil), vmcp, slices.Clone(components)); len(got) != 2 {
		t.Fatalf("user components = %v, want both", got)
	}
	if got := EnabledComponents(agentInfo(8, nil), vmcp, slices.Clone(components)); len(got) != 0 {
		t.Fatalf("components of an agent no profile names = %v, want none", got)
	}
	if got := EnabledComponents(agentInfo(7, nil), vmcp, slices.Clone(components)); len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("agent components = %v, want only the one its profile grants", got)
	}
	if _, err := agentReadiness(t, vmcp, userInfo("7", nil)); err == nil {
		t.Fatal("readiness was computed for a user")
	}
}

// TestAgentReadinessChecksOnlyEnabledComponents checks that readiness reports configuration and OAuth reasons only for
// the components enabled for the agent, selects their grants with one query, and fails when the grants cannot be read.
func TestAgentReadinessChecksOnlyEnabledComponents(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{configuredComponent("configured"), staticOAuthComponent("oauth"), staticOAuthConfiguredComponent("disabled")},
		agentProfile(readinessAgentProfile, "7", "configured", "oauth"),
	)
	instance := v1.VMCPInstance{Name: "vmcpi1agent", Spec: v1.VMCPInstanceSpec{UserID: "agent:7"}}

	grants := newMemoryGrants()
	report, err := AgentReadiness(t.Context(), NewProfileOAuthGrantStore(grants), vmcp, &instance, agent)
	if err != nil {
		t.Fatal(err)
	}
	want := []ReadinessReason{
		{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION", ProfileID: readinessAgentProfile},
		{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired, ProfileID: readinessAgentProfile},
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

	if _, err := AgentReadiness(t.Context(), NewProfileOAuthGrantStore(failingGrants{newMemoryGrants()}), vmcp, &instance, agent); err == nil {
		t.Fatal("a store that cannot be read did not fail readiness")
	}
	if _, err := AgentReadiness(t.Context(), nil, vmcp, &instance, agent); err == nil {
		t.Fatal("readiness was computed without a store of grants")
	}
}

func TestAgentReadinessRefusesPersonalVMCPs(t *testing.T) {
	vmcp := readinessVMCP([]types.VMCPComponent{readinessComponent("plain")})
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
