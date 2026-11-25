package vmcp

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// onBehalfOf returns the principal of agent 7 acting on behalf of user 9, as the authenticator of delegated tokens
// builds it.
func onBehalfOf(t *testing.T) *kuser.DefaultInfo {
	t.Helper()

	info := agentInfo(7, nil)
	principal.RecordOnBehalfOf(info.Extra, 9)
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(9)},
	}))
	return info
}

// TestProfilesOnBehalfOfAUser checks that an agent acting on behalf of a user is granted, on a vMCP, every profile that
// names the agent: its components and tools are their union, as for the agent acting for itself, and a profile of the
// user's never applies.
func TestProfilesOnBehalfOfAUser(t *testing.T) {
	components := []types.VMCPComponent{{ID: "mail"}, {ID: "files"}, {ID: "calendar"}}
	read := types.VMCPProfile{
		ID:       "vp1read",
		Name:     "read",
		Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
			"files": {AllowedTools: []string{"read"}},
		}},
	}
	write := types.VMCPProfile{
		ID:       "vp1write",
		Name:     "write",
		Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
			"files": {AllowedTools: []string{"write"}},
			"mail":  {AllowedTools: []string{"send"}},
		}},
	}
	users := types.VMCPProfile{
		ID:          "vp1users",
		Name:        "users",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}, {Type: types.SubjectTypeUser, ID: "9"}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
	vmcp := sharedVMCP(read, write, users)
	vmcp.Name = "vmcp1files"
	vmcp.Spec.Manifest.Components = components
	tool := func(componentID, name string) types.VMCPToolReference {
		return types.VMCPToolReference{ComponentID: componentID, Name: name}
	}

	caller := onBehalfOf(t)
	var profiles []string
	for _, profile := range CallerProfiles(caller, vmcp) {
		profiles = append(profiles, profile.ID)
	}
	require.Equal(t, []string{"vp1read", "vp1write"}, profiles)

	var enabled []string
	for _, component := range EnabledComponents(caller, vmcp, slices.Clone(components)) {
		enabled = append(enabled, component.ID)
	}
	require.Equal(t, []string{"mail", "files"}, enabled)
	tools := []types.VMCPToolReference{tool("files", "read"), tool("files", "write"), tool("mail", "send")}
	require.Equal(t, tools, AllowedTools(caller, vmcp, nil))
	require.Equal(t, tools, InstanceGrant(caller, vmcp, v1.VMCPInstance{}))
	// The agent acting for itself is granted the same.
	require.Equal(t, tools, AllowedTools(agentInfo(7, nil), vmcp, nil))

	// Profiles are found only for the vMCP they belong to.
	require.Empty(t, MatchingProfiles(caller, vmcp.Spec.Manifest.Profiles))

	// A personal vMCP grants an agent acting on behalf of a user nothing.
	personal := vmcp
	personal.Spec.UserID = "9"
	require.Empty(t, CallerProfiles(onBehalfOf(t), personal))
}

// TestReadinessFailureForUser checks how a vMCP that an agent acting on behalf of a user cannot use is reported to it:
// what the user can set up needs their approval, and a grant of the vMCP that no longer works is an administrator's.
func TestReadinessFailureForUser(t *testing.T) {
	configuration := ReadinessReason{ComponentID: "files", Code: types.AgentReadinessConfigurationRequired, Field: "TOKEN"}
	oauth := ReadinessReason{ComponentID: "mail", Code: types.AgentReadinessOAuthRequired}
	invalid := ReadinessReason{ComponentID: "calendar", Code: types.AgentReadinessOAuthInvalid}

	for _, tt := range []struct {
		name    string
		failure AgentReadinessFailure
		want    AgentReadinessFailure
	}{
		{
			name:    "configuration the user enters",
			failure: AgentReadinessFailure{Error: AgentConfigurationRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{configuration}},
			want:    AgentReadinessFailure{Error: AgentUserApprovalRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{configuration}},
		},
		{
			name:    "OAuth that no grant covers",
			failure: AgentReadinessFailure{Error: AgentOAuthRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{oauth, configuration}},
			want:    AgentReadinessFailure{Error: AgentUserApprovalRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{oauth, configuration}},
		},
		{
			name:    "a grant of the vMCP that no longer works",
			failure: AgentReadinessFailure{Error: AgentConfigurationRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{configuration, invalid}},
			want:    AgentReadinessFailure{Error: AgentOAuthRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{invalid}},
		},
		{
			name:    "no reasons",
			failure: AgentReadinessFailure{Error: AgentOAuthRequiredError},
			want:    AgentReadinessFailure{Error: AgentOAuthRequiredError},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.failure.ForUser())
		})
	}

	notReady := &AgentNotReadyError{Readiness: Readiness{Reasons: []ReadinessReason{configuration}}, VMCPID: "vmcp1x", OnBehalfOfUser: true}
	require.Equal(t, AgentReadinessFailure{Error: AgentUserApprovalRequiredError, VMCPID: "vmcp1x", Reasons: []ReadinessReason{configuration}}, notReady.Failure())
	require.Contains(t, notReady.Error(), AgentUserApprovalRequiredError)
	notReady.OnBehalfOfUser = false
	require.Equal(t, AgentConfigurationRequiredError, notReady.Failure().Error)
}
