package vmcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	readinessExplicitProfile = "vp1explicit"
	readinessWildcardProfile = "vp1wildcard"
	readinessOtherWildcard   = "vp1otherwildcard"
)

// recordingCheck is a readiness check that reports one reason with its code for each component, or none without a
// code, and records what it was asked.
type recordingCheck struct {
	code  string
	asked []string
	in    *AgentReadinessInput
}

type failingCheck struct{}

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
				Code:        ReadinessConfigurationRequired,
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
				Code:        ReadinessConfigurationRequired,
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
				Code:        ReadinessOAuthRequired,
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
					Code:        ReadinessOAuthRequired,
					ProfileID:   readinessExplicitProfile,
				},
				{
					ComponentID: "configured",
					Code:        ReadinessConfigurationRequired,
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
			wantReasons: []ReadinessReason{{Code: ReadinessSelectionAmbiguous}},
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
				{ComponentID: "configured", Code: ReadinessSelectionAmbiguous},
				{ComponentID: "both", Code: ReadinessSelectionAmbiguous},
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
			wantReasons:  []ReadinessReason{{ComponentID: "oauth", Code: ReadinessSelectionAmbiguous}},
			wantError:    AgentSelectionAmbiguousError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness, err := AgentReadiness(t.Context(), tt.vmcp, v1.VMCPInstance{}, agent)
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
	if _, err := AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, userInfo("7", nil)); err == nil {
		t.Fatal("readiness was computed for a user")
	}
}

func (c *recordingCheck) ComponentReadiness(_ context.Context, in AgentReadinessInput, component types.VMCPComponent) ([]ReadinessReason, error) {
	c.asked = append(c.asked, component.ID)
	c.in = &in
	if c.code == "" {
		// The component is fully supplied by the explicit profile.
		return nil, nil
	}
	return []ReadinessReason{{ComponentID: component.ID, Code: c.code}}, nil
}

func (failingCheck) ComponentReadiness(context.Context, AgentReadinessInput, types.VMCPComponent) ([]ReadinessReason, error) {
	return nil, errors.New("store unavailable")
}

// TestAgentReadinessChecksAreReplaceable checks that each source of reasons can be replaced on its own, as specs 12
// and 13 do, and that the checks only see components enabled for the agent.
func TestAgentReadinessChecksAreReplaceable(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{configuredComponent("configured"), staticOAuthComponent("oauth"), readinessComponent("disabled")},
		agentProfile(readinessExplicitProfile, "7", "configured", "oauth"),
	)
	instance := v1.VMCPInstance{Name: "vmcpi1agent", Spec: v1.VMCPInstanceSpec{UserID: "agent:7"}}

	configuration := &recordingCheck{code: ReadinessOAuthInvalid}
	readiness, err := AgentReadinessChecks{Configuration: configuration}.AgentReadiness(t.Context(), vmcp, instance, agent)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(configuration.asked, []string{"configured", "oauth"}) {
		t.Fatalf("configuration check asked about %v, want only the enabled components", configuration.asked)
	}
	if configuration.in.Instance.Name != instance.Name || configuration.in.Selection.Explicit == nil || configuration.in.Selection.Explicit.ID != readinessExplicitProfile {
		t.Fatalf("configuration check input = %#v", configuration.in)
	}
	// The replaced configuration check reports for both components, and the default OAuth check still reports the
	// static OAuth component.
	want := []ReadinessReason{
		{ComponentID: "configured", Code: ReadinessOAuthInvalid},
		{ComponentID: "oauth", Code: ReadinessOAuthInvalid},
		{ComponentID: "oauth", Code: ReadinessOAuthRequired, ProfileID: readinessExplicitProfile},
	}
	if !reflect.DeepEqual(readiness.Reasons, want) {
		t.Fatalf("reasons = %#v, want %#v", readiness.Reasons, want)
	}
	if readiness.ErrorCode() != AgentOAuthRequiredError {
		t.Fatalf("error = %q, want %q", readiness.ErrorCode(), AgentOAuthRequiredError)
	}

	if _, err := (AgentReadinessChecks{OAuth: failingCheck{}}).AgentReadiness(t.Context(), vmcp, instance, agent); err == nil {
		t.Fatal("a failing check did not fail readiness")
	}
}

func TestAgentReadinessRefusesPersonalVMCPs(t *testing.T) {
	vmcp := readinessVMCP([]types.VMCPComponent{readinessComponent("plain")})
	vmcp.Spec.UserID = "7"
	if _, err := AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, agentInfo(7, nil)); err == nil {
		t.Fatal("readiness was computed for a personal vMCP")
	}
}

func TestReadinessErrorCodePrecedence(t *testing.T) {
	ambiguous := ReadinessReason{Code: ReadinessSelectionAmbiguous}
	ambiguousComponent := AmbiguousFallbackReason("one")
	configuration := ReadinessReason{ComponentID: "one", Code: ReadinessConfigurationRequired, Field: "REGION"}
	oauth := ReadinessReason{ComponentID: "two", Code: ReadinessOAuthRequired}
	invalid := ReadinessReason{ComponentID: "two", Code: ReadinessOAuthInvalid}
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

// TestAgentReadinessChecksSeeAnAmbiguousFallback checks the contract of a replacement check on a vMCP whose fallback is
// ambiguous: it is called for an agent with an explicit profile, which it is given, with no wildcard and the ambiguity
// marked, and a component it resolves from the explicit profile alone is ready.
func TestAgentReadinessChecksSeeAnAmbiguousFallback(t *testing.T) {
	agent := agentInfo(7, nil)
	vmcp := readinessVMCP(
		[]types.VMCPComponent{configuredComponent("configured")},
		agentProfile(readinessExplicitProfile, "7", "configured"),
		legacyWildcardProfile(readinessWildcardProfile),
		legacyWildcardProfile(readinessOtherWildcard),
	)

	explicitOnly := &recordingCheck{}
	readiness, err := AgentReadinessChecks{Configuration: explicitOnly, OAuth: explicitOnly}.AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, agent)
	if err != nil {
		t.Fatal(err)
	}
	if explicitOnly.in == nil {
		t.Fatal("the check was not called for the explicit agent")
	}
	selection := explicitOnly.in.Selection
	if !selection.Ambiguous || selection.Wildcard != nil || selection.Explicit == nil || selection.Explicit.ID != readinessExplicitProfile {
		t.Fatalf("selection = %#v, want the explicit profile, no wildcard, and the ambiguity marked", selection)
	}
	if !readiness.Ready() || readiness.ExplicitProfileID != readinessExplicitProfile || readiness.WildcardProfileID != "" {
		t.Fatalf("readiness = %#v, want ready with only the explicit profile", readiness)
	}

	// A wildcard-only agent is refused before any check runs.
	unchecked := &recordingCheck{}
	readiness, err = AgentReadinessChecks{Configuration: unchecked, OAuth: unchecked}.AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, agentInfo(8, nil))
	if err != nil {
		t.Fatal(err)
	}
	if unchecked.in != nil {
		t.Fatal("a check ran for an agent without an explicit profile on an ambiguous vMCP")
	}
	if !reflect.DeepEqual(readiness.Reasons, []ReadinessReason{{Code: ReadinessSelectionAmbiguous}}) {
		t.Fatalf("reasons = %#v, want the vMCP-wide ambiguity", readiness.Reasons)
	}
}
