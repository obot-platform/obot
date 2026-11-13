package vmcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	readinessExplicitProfile = "vp1explicit"
	readinessWildcardProfile = "vp1wildcard"
	readinessOtherWildcard   = "vp1otherwildcard"
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

// legacyWildcardProfile returns a profile that names everyone and allows every component, as a vMCP stored before the
// wildcard rule may have several of.
func legacyWildcardProfile(id string) types.VMCPProfile {
	return types.VMCPProfile{
		ID:          id,
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
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
		name         string
		vmcp         v1.VMCP
		wantExplicit string
		wantWildcard string
		wantReasons  []ReadinessReason
		wantError    string
	}{
		{
			name: "components that need nothing",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain")},
				agentProfile(readinessExplicitProfile, "7", "plain"),
				agentProfile(readinessWildcardProfile, types.SubjectWildcardID, "plain"),
			),
			wantExplicit: readinessExplicitProfile,
			wantWildcard: readinessWildcardProfile,
		},
		{
			// No profile sets the required value a user would enter, so it blocks the agent, fixed by the explicit
			// profile. Optional and fixed values do not.
			name: "required configuration with an explicit profile",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{configuredComponent("configured")},
				agentProfile(readinessExplicitProfile, "7", "configured"),
				agentProfile(readinessWildcardProfile, types.SubjectWildcardID),
			),
			wantExplicit: readinessExplicitProfile,
			wantWildcard: readinessWildcardProfile,
			wantReasons: []ReadinessReason{{
				ComponentID: "configured",
				Code:        types.AgentReadinessConfigurationRequired,
				Field:       "REGION",
				ProfileID:   readinessExplicitProfile,
			}},
			wantError: AgentConfigurationRequiredError,
		},
		{
			name: "required configuration fixed in the wildcard profile",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{configuredComponent("configured")},
				agentProfile(readinessWildcardProfile, types.SubjectWildcardID, "configured"),
			),
			wantWildcard: readinessWildcardProfile,
			wantReasons: []ReadinessReason{{
				ComponentID: "configured",
				Code:        types.AgentReadinessConfigurationRequired,
				Field:       "REGION",
				ProfileID:   readinessWildcardProfile,
			}},
			wantError: AgentConfigurationRequiredError,
		},
		{
			name: "static OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), readinessComponent("plain")},
				agentProfile(readinessExplicitProfile, "7", "oauth", "plain"),
			),
			wantExplicit: readinessExplicitProfile,
			wantReasons: []ReadinessReason{{
				ComponentID: "oauth",
				Code:        types.AgentReadinessOAuthRequired,
				ProfileID:   readinessExplicitProfile,
			}},
			wantError: AgentOAuthRequiredError,
		},
		{
			// One reason of each kind is reported as configuration, which an administrator fixes first.
			name: "configuration and OAuth",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured")},
				agentProfile(readinessExplicitProfile, "7", "oauth", "configured"),
			),
			wantExplicit: readinessExplicitProfile,
			wantReasons: []ReadinessReason{
				{
					ComponentID: "oauth",
					Code:        types.AgentReadinessOAuthRequired,
					ProfileID:   readinessExplicitProfile,
				},
				{
					ComponentID: "configured",
					Code:        types.AgentReadinessConfigurationRequired,
					Field:       "REGION",
					ProfileID:   readinessExplicitProfile,
				},
			},
			wantError: AgentConfigurationRequiredError,
		},
		{
			// Components no profile enables for the agent need no setup.
			name: "disabled components",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{staticOAuthComponent("oauth"), configuredComponent("configured"), readinessComponent("plain")},
				agentProfile(readinessExplicitProfile, "7", "plain"),
				agentProfile("vp1otheragent", "8", "oauth", "configured"),
			),
			wantExplicit: readinessExplicitProfile,
		},
		{
			// Two profiles name every agent, so the profile an agent without an explicit one falls back to is unknown,
			// and nothing else is checked, even for components that need nothing.
			name: "legacy wildcard profiles without an explicit profile",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain")},
				legacyWildcardProfile(readinessWildcardProfile),
				legacyWildcardProfile(readinessOtherWildcard),
			),
			wantReasons: []ReadinessReason{{Code: types.AgentReadinessSelectionAmbiguous}},
			wantError:   AgentSelectionAmbiguousError,
		},
		{
			// An agent with an explicit profile needs no fallback for components that need nothing.
			name: "legacy wildcard profiles with an explicit profile and components that need nothing",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain"), readinessComponent("other")},
				agentProfile(readinessExplicitProfile, "7", "plain"),
				legacyWildcardProfile(readinessWildcardProfile),
				legacyWildcardProfile(readinessOtherWildcard),
			),
			wantExplicit: readinessExplicitProfile,
		},
		{
			// The explicit profile sets no configuration, and cannot hold grants yet, so components that need them
			// would need the ambiguous fallback, and are reported as ambiguous, once each, without a profile to fix. A
			// component that needs nothing is not reported.
			name: "legacy wildcard profiles with an explicit profile and components that need setup",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{configuredComponent("configured"), readinessComponent("plain"), staticOAuthConfiguredComponent("both")},
				agentProfile(readinessExplicitProfile, "7", "configured", "plain", "both"),
				legacyWildcardProfile(readinessWildcardProfile),
				legacyWildcardProfile(readinessOtherWildcard),
			),
			wantExplicit: readinessExplicitProfile,
			wantReasons: []ReadinessReason{
				{ComponentID: "configured", Code: types.AgentReadinessSelectionAmbiguous},
				{ComponentID: "both", Code: types.AgentReadinessSelectionAmbiguous},
			},
			wantError: AgentSelectionAmbiguousError,
		},
		{
			// Components enabled only by the wildcard profiles are still the agent's, and still need the fallback.
			name: "legacy wildcard profiles grant an explicit agent more components",
			vmcp: readinessVMCP(
				[]types.VMCPComponent{readinessComponent("plain"), staticOAuthComponent("oauth")},
				agentProfile(readinessExplicitProfile, "7", "plain"),
				legacyWildcardProfile(readinessWildcardProfile),
				legacyWildcardProfile(readinessOtherWildcard),
			),
			wantExplicit: readinessExplicitProfile,
			wantReasons:  []ReadinessReason{{ComponentID: "oauth", Code: types.AgentReadinessSelectionAmbiguous}},
			wantError:    AgentSelectionAmbiguousError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness, err := agentReadiness(t, tt.vmcp, agent)
			if err != nil {
				t.Fatal(err)
			}
			if readiness.ExplicitProfileID != tt.wantExplicit || readiness.WildcardProfileID != tt.wantWildcard {
				t.Errorf("profiles = explicit %q, wildcard %q; want %q, %q", readiness.ExplicitProfileID, readiness.WildcardProfileID, tt.wantExplicit, tt.wantWildcard)
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

// TestUserMatchesLegacyWildcardProfiles checks that the legacy wildcard profiles that make agent selection ambiguous
// still grant users what they did.
func TestUserMatchesLegacyWildcardProfiles(t *testing.T) {
	vmcp := readinessVMCP(
		[]types.VMCPComponent{readinessComponent("one"), readinessComponent("two")},
		types.VMCPProfile{
			ID:          readinessWildcardProfile,
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"one": {}}},
		},
		types.VMCPProfile{
			ID:          readinessOtherWildcard,
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"two": {}}},
		},
	)
	if got := EnabledComponents(userInfo("7", nil), vmcp, ComponentsForInstance(vmcp, v1.VMCPInstance{})); len(got) != 2 {
		t.Fatalf("user components = %v, want both", got)
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
		agentProfile(readinessExplicitProfile, "7", "configured", "oauth"),
	)
	instance := v1.VMCPInstance{Name: "vmcpi1agent", Spec: v1.VMCPInstanceSpec{UserID: "agent:7"}}

	grants := newMemoryGrants()
	report, err := AgentReadiness(t.Context(), NewProfileOAuthGrantStore(grants), vmcp, &instance, agent)
	if err != nil {
		t.Fatal(err)
	}
	want := []ReadinessReason{
		{ComponentID: "configured", Code: types.AgentReadinessConfigurationRequired, Field: "REGION", ProfileID: readinessExplicitProfile},
		{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired, ProfileID: readinessExplicitProfile},
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
	ambiguous := ReadinessReason{Code: types.AgentReadinessSelectionAmbiguous}
	ambiguousComponent := AmbiguousFallbackReason("one")
	configuration := ReadinessReason{ComponentID: "one", Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}
	oauth := ReadinessReason{ComponentID: "two", Code: types.AgentReadinessOAuthRequired}
	invalid := ReadinessReason{ComponentID: "two", Code: types.AgentReadinessOAuthInvalid}
	for _, tt := range []struct {
		name    string
		reasons []ReadinessReason
		want    string
	}{
		{
			name:    "ambiguous vMCP",
			reasons: []ReadinessReason{ambiguous},
			want:    AgentSelectionAmbiguousError,
		},
		{
			name:    "ambiguous component",
			reasons: []ReadinessReason{ambiguousComponent},
			want:    AgentSelectionAmbiguousError,
		},
		{
			name:    "ambiguity before configuration",
			reasons: []ReadinessReason{configuration, ambiguousComponent},
			want:    AgentSelectionAmbiguousError,
		},
		{
			name:    "ambiguity before OAuth",
			reasons: []ReadinessReason{oauth, ambiguousComponent},
			want:    AgentSelectionAmbiguousError,
		},
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

// TestAgentReadinessWithAnAmbiguousFallback checks readiness on a vMCP whose fallback is ambiguous: an agent with an
// explicit profile is checked against that profile alone, and is ready when it supplies everything, and an agent
// without one is refused for the whole vMCP before any component is checked.
func TestAgentReadinessWithAnAmbiguousFallback(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{configuredComponent("configured")},
		agentProfile(readinessExplicitProfile, "7", "configured"),
		legacyWildcardProfile(readinessWildcardProfile),
		legacyWildcardProfile(readinessOtherWildcard),
	)
	vmcp.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{
		readinessExplicitProfile: {Name: "revision1", Keys: []string{ConfigurationKey("configured", "REGION")}},
	}

	report, err := AgentReadiness(t.Context(), NewProfileOAuthGrantStore(newMemoryGrants()), vmcp, nil, agent)
	if err != nil {
		t.Fatal(err)
	}
	if readiness := report.Readiness; !readiness.Ready() || readiness.ExplicitProfileID != readinessExplicitProfile || readiness.WildcardProfileID != "" {
		t.Fatalf("readiness = %#v, want ready with only the explicit profile", readiness)
	}
	if len(report.Grants) != 1 {
		t.Fatalf("grants = %#v, want the explicit agent's component checked", report.Grants)
	}

	// A wildcard-only agent is refused before any component is checked.
	grants := newMemoryGrants()
	report, err = AgentReadiness(t.Context(), NewProfileOAuthGrantStore(grants), vmcp, nil, agentInfo(8, nil))
	if err != nil {
		t.Fatal(err)
	}
	if report.Grants != nil || grants.queries != 0 {
		t.Fatal("components were checked for an agent without an explicit profile on an ambiguous vMCP")
	}
	if !reflect.DeepEqual(report.Readiness.Reasons, []ReadinessReason{{Code: types.AgentReadinessSelectionAmbiguous}}) {
		t.Fatalf("reasons = %#v, want the vMCP-wide ambiguity", report.Readiness.Reasons)
	}
}
