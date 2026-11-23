package mcp

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// asAgentOnBehalfOf returns the principal of the agent with id acting on behalf of the user with userID, as the
// authenticator of delegated tokens builds it.
func asAgentOnBehalfOf(t *testing.T, id, keyID, userID uint) *kuser.DefaultInfo {
	t.Helper()

	info := asAgent(id, keyID)
	principal.RecordOnBehalfOf(info.Extra, userID)
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(userID)},
	}))
	return info
}

// TestAgentConnectOnBehalfOfAUser checks that an agent acting on behalf of a user connects through the agent's own
// instance, with the tools of every vMCP profile that names the agent, and never of one that names the user, and that the
// call is attributed to the agent's profiles.
func TestAgentConnectOnBehalfOfAUser(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1onbehalfof",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "On behalf of",
			Components:  []types.VMCPComponent{{ID: "one", Name: "one"}, {ID: "two", Name: "two"}},
			Profiles: []types.VMCPProfile{
				{
					ID:       "vp1read",
					Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
					Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
						"one": {AllowedTools: []string{"read"}},
					}},
				},
				{
					ID:       "vp1write",
					Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
					Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
						"one": {AllowedTools: []string{"write"}},
						"two": {AllowedTools: []string{"deploy"}},
					}},
				},
				{
					ID:          "vp1users",
					Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}, {Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
			},
		}},
	}
	sharedOne := vmcpComponentServer("ms1shared-one", "", "", "one", "one", "https://one.example.com/mcp")
	sharedOne.Spec.VMCPID = vmcp.Name
	sharedTwo := vmcpComponentServer("ms1shared-two", "", "", "two", "two", "https://two.example.com/mcp")
	sharedTwo.Spec.VMCPID = vmcp.Name
	storageClient := newVMCPTestStorage(vmcp, sharedOne, sharedTwo)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	manager := newAgentTestManager(storageClient)

	toolsByComponent := func(cfg ServerConfig) map[string][]string {
		tools := map[string][]string{}
		for _, component := range cfg.Components {
			for _, tool := range component.Tools {
				tools[component.ComponentID] = append(tools[component.ComponentID], tool.Name)
			}
			slices.Sort(tools[component.ComponentID])
		}
		return tools
	}

	own := connectVMCP(t, manager, vmcp.Name, asAgent(5, 11))
	forSeven := connectVMCP(t, manager, vmcp.Name, asAgentOnBehalfOf(t, 5, 11, 7))
	require.Equal(t, own.MCPServerName, forSeven.MCPServerName, "the agent's own instance")
	require.Equal(t, principal.Agent(5).String(), forSeven.UserID)
	if got, want := toolsByComponent(forSeven), map[string][]string{"one": {"read", "write"}, "two": {"deploy"}}; !equalToolMaps(got, want) {
		t.Fatalf("tools on behalf of a user = %v, want %v", got, want)
	}
	if got, want := toolsByComponent(forSeven), toolsByComponent(own); !equalToolMaps(got, want) {
		t.Fatalf("tools on behalf of a user = %v, want the agent's own %v", got, want)
	}
	require.Equal(t, "vp1read,vp1write", forSeven.AuditLogMetadata[AuditLogProfiles])
}

// TestAgentConnectOnBehalfOfAUserNeedsApproval checks that a component the agent is granted, which needs something the
// user would set up, refuses the agent acting on behalf of the user with agent_user_approval_required, naming the vMCP
// and the component.
func TestAgentConnectOnBehalfOfAUserNeedsApproval(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1approval",
		types.VMCPProfile{
			ID:          "vp1needs",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
	vmcp.Spec.Manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp)
	manager := newAgentTestManager(storageClient)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgentOnBehalfOf(t, 5, 11, 7))
	notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
	require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  vmcp.Name,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessConfigurationRequired, Field: "X-Token"}},
	}, notReady.Failure())

	// The agent acting for itself is told an administrator must fix it, as before.
	_, err = manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 11))
	notReady, ok = errors.AsType[*vmcpaccess.AgentNotReadyError](err)
	require.True(t, ok, "connect error = %v, want an AgentNotReadyError", err)
	require.Equal(t, vmcpaccess.AgentConfigurationRequiredError, notReady.Failure().Error)
	require.Empty(t, notReady.Failure().VMCPID)
}
