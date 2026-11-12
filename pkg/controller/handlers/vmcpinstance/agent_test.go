package vmcpinstance

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// agentOwnerInfo returns the principal of agent 7 as IdentityInfo loads it.
func agentOwnerInfo(status types.UserStatus) kuser.Info {
	agent := &gatewaytypes.Agent{ID: 7, DisplayName: "nightly build"}
	if status == types.UserStatusDisabled {
		disabledAt := time.Now()
		agent.DisabledAt = &disabledAt
	}
	return gateway.AgentUserInfo(agent, nil, nil)
}

// withAgentChangeWatches requires that the handlers watch agent 7's lifecycle events, and never the user events of
// the user with its numeric ID.
func withAgentChangeWatches(t *testing.T, builder *fake.ClientBuilder) *fake.ClientBuilder {
	t.Helper()
	return builder.
		WithIndex(&v1.AgentIdentityChange{}, "spec.agentID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.AgentIdentityChange).Get("spec.agentID")}
		}).
		WithIndex(&v1.AgentIdentityDelete{}, "spec.agentID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.AgentIdentityDelete).Get("spec.agentID")}
		}).
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			switch list.(type) {
			case *v1.AgentIdentityChangeList, *v1.AgentIdentityDeleteList:
				options := (&kclient.ListOptions{}).ApplyOptions(opts)
				require.Equal(t, "default", options.Namespace)
				require.NotNil(t, options.FieldSelector)
				require.Equal(t, "spec.agentID=7", options.FieldSelector.String())
			case *v1.UserGroupChangeList, *v1.UserRoleChangeList:
				t.Errorf("an agent's instance watched %T", list)
			}
			return client.List(ctx, list, opts...)
		}})
}

func TestDeleteUnauthorizedAgentInstances(t *testing.T) {
	notFound := fmt.Errorf("agent 7: %w", gorm.ErrRecordNotFound)
	for _, tc := range []struct {
		name     string
		subjects []types.Subject
		status   types.UserStatus
		lookup   error
		wantErr  bool
		deleted  bool
	}{
		{
			name:     "explicit profile",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
		},
		{
			name:     "every agent",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}},
		},
		{
			name:     "everyone",
			subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		},
		{
			// Disabling an agent keeps its instance; authentication refuses it until it is enabled again.
			name:     "disabled agent keeps its access",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
			status:   types.UserStatusDisabled,
		},
		{
			// The user with the agent's numeric ID grants the agent nothing.
			name:     "user with the agent's ID",
			subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
			deleted:  true,
		},
		{
			name:     "every user",
			subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}},
			deleted:  true,
		},
		{
			name:     "another agent",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}},
			deleted:  true,
		},
		{
			name:    "profile removed",
			deleted: true,
		},
		{
			name:     "deleted agent",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
			lookup:   notFound,
			deleted:  true,
		},
		{
			name:     "lookup error preserves instance",
			subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
			lookup:   errors.New("lookup failed"),
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{Name: "vmcp1agents", Namespace: "default"}
			if tc.subjects != nil {
				vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{{ID: "vp1agents", Subjects: tc.subjects}}
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1agent",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID:   "agent:7",
					Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				},
			}
			client := withAgentChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance)).Build()
			handler := &Handler{userInfo: func(_ context.Context, owner principal.Principal) (kuser.Info, error) {
				require.Equal(t, principal.Agent(7), owner)
				if tc.lookup != nil {
					return nil, tc.lookup
				}
				return agentOwnerInfo(tc.status), nil
			}}
			for range 2 {
				err := handler.DeleteUnauthorized(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				err = client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.VMCPInstance{})
				if tc.deleted {
					require.True(t, apierrors.IsNotFound(err), "expected deletion, got %v", err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

// TestAgentInstanceResourcesAreBoundToComponents checks the invariant the access control cleanup controllers rely on:
// every MCPServer and MCPServerInstance created for an agent's vMCP instance is a vMCP component server or
// connection, which they skip, so they never look an agent up as a user. It also checks that an agent and the user
// with its numeric ID get separate servers, connections and connection credentials on one vMCP.
func TestAgentInstanceResourcesAreBoundToComponents(t *testing.T) {
	shared := dedicatedTestComponent("shared")
	shared.ForceSingleUser = false
	vmcp := &v1.VMCP{
		Name:      "vmcp1mixed",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{dedicatedTestComponent("dedicated"), shared},
				Profiles:   allowAllProfiles(),
			},
		},
	}
	agentInstance := &v1.VMCPInstance{
		Name:      "vmcpi1agent",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			UserID:   "agent:7",
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	userInstance := agentInstance.DeepCopy()
	userInstance.Name = "vmcpi1user"
	userInstance.Spec.UserID = "7"
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, agentInstance, userInstance).
		WithIndex(&v1.AgentIdentityChange{}, "spec.agentID", func(o kclient.Object) []string { return []string{o.(*v1.AgentIdentityChange).Get("spec.agentID")} }).
		WithIndex(&v1.AgentIdentityDelete{}, "spec.agentID", func(o kclient.Object) []string { return []string{o.(*v1.AgentIdentityDelete).Get("spec.agentID")} }).
		WithIndex(&v1.UserGroupChange{}, "spec.userID", func(o kclient.Object) []string { return []string{o.(*v1.UserGroupChange).Get("spec.userID")} }).
		WithIndex(&v1.UserRoleChange{}, "spec.userID", func(o kclient.Object) []string { return []string{o.(*v1.UserRoleChange).Get("spec.userID")} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).
		Build()
	handler := &Handler{userInfo: func(_ context.Context, owner principal.Principal) (kuser.Info, error) {
		if owner.Kind == principal.KindAgent {
			require.Equal(t, principal.Agent(7), owner)
			return agentOwnerInfo(types.UserStatusActive), nil
		}
		require.Equal(t, principal.User(7), owner)
		return principaltest.AsUser(&kuser.DefaultInfo{UID: "7"}), nil
	}}
	require.NoError(t, vmcphandler.EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil))
	for _, instance := range []*v1.VMCPInstance{agentInstance, userInstance} {
		req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
		require.NoError(t, handler.EnsureMCPServers(req, nil))
		require.NoError(t, handler.EnsureMCPServerInstances(req, nil))
	}

	var servers v1.MCPServerList
	require.NoError(t, client.List(t.Context(), &servers))
	dedicated := map[string]string{}
	for _, server := range servers.Items {
		if server.Spec.VMCPInstanceID == "" {
			// The shared server of the vMCP belongs to no caller.
			require.Empty(t, server.Spec.UserID)
			continue
		}
		require.NotEmpty(t, server.Spec.VMCPComponentID, "server %s of an instance is not a vMCP component", server.Name)
		dedicated[server.Spec.UserID] = server.Name
	}
	require.Len(t, dedicated, 2)
	require.NotEqual(t, dedicated["agent:7"], dedicated["7"])

	var connections v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &connections))
	contexts := map[string]string{}
	for _, connection := range connections.Items {
		require.NotEmpty(t, connection.Spec.VMCPComponentID, "connection %s is not a vMCP component connection", connection.Name)
		require.NotEmpty(t, connection.Spec.VMCPInstanceID)
		// The credential context of a connection, as mcpserverinstance.SyncVMCPConfiguration writes it and the gateway
		// reads it: "<owner>-<connection>", which ':' in an agent's owner keeps apart from a user's.
		contexts[connection.Spec.UserID] = connection.Spec.UserID + "-" + connection.Name
	}
	require.Len(t, contexts, 2)
	require.Equal(t, "agent:7-msi1vmcpi1agent-shared", contexts["agent:7"])
	require.Equal(t, "7-msi1vmcpi1user-shared", contexts["7"])
}
