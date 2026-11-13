package vmcpinstance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserver"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserverinstance"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// profileDelivery is a vMCP whose profiles give agents configuration, the controllers that deliver it, and the
// instances of agent 7 (named by its own profile), agent 9 (named only by the profile for every agent) and user 7 (the
// user with agent 7's numeric ID, who enters their own values).
type profileDelivery struct {
	t           *testing.T
	client      kclient.WithWatch
	gateway     *gateway.Client
	store       *vmcpconfig.ProfileConfigurationStore
	instances   *Handler
	servers     *mcpserver.Handler
	connections *mcpserverinstance.Handler
	vmcp        *v1.VMCP
	seven       *v1.VMCPInstance
	nine        *v1.VMCPInstance
	user        *v1.VMCPInstance
}

func newDeliveryTestGateway(t *testing.T) *gateway.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	client := gateway.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newProfileDelivery(t *testing.T) *profileDelivery {
	t.Helper()
	// ForceSingleUser gives each instance its own server for this component, even though its values are headers.
	dedicated := dedicatedTestComponent("dedicated")
	dedicated.Name = "dedicated"
	dedicated.CatalogEntry.Manifest.Config = []types.MCPConfig{
		{Key: "REGION", Required: true, Usage: types.Header},
		{Key: "VERBOSE", Usage: types.Header},
		{Key: "ADMIN_TOKEN", Required: true, Usage: types.Header},
	}
	dedicated.Configuration = []types.VMCPConfigurationPolicy{
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
	}
	shared := dedicatedTestComponent("shared")
	shared.Name = "shared"
	shared.ForceSingleUser = false
	shared.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Required: true, Usage: types.Header}}
	shared.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: "vmcp1delivery", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{
			DisplayName: "Delivery",
			Components:  []types.VMCPComponent{dedicated, shared},
			Profiles: []types.VMCPProfile{
				profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
				profile("vp1agents", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
				profile("vp1users", types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
			},
		},
		StaticConfigurationHash: "static",
	}}
	instance := func(name, owner string) *v1.VMCPInstance {
		return &v1.VMCPInstance{Name: name, Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	}
	d := &profileDelivery{
		t:     t,
		vmcp:  vmcp,
		seven: instance("vmcpi1seven", "agent:7"),
		nine:  instance("vmcpi1nine", "agent:9"),
		user:  instance("vmcpi1user", "7"),
	}
	d.client = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.VMCP{}, &v1.VMCPInstance{}, &v1.MCPServer{}, &v1.MCPServerInstance{}).
		WithObjects(vmcp, d.seven, d.nine, d.user).
		WithIndex(&v1.AgentIdentityChange{}, "spec.agentID", func(o kclient.Object) []string { return []string{o.(*v1.AgentIdentityChange).Get("spec.agentID")} }).
		WithIndex(&v1.AgentIdentityDelete{}, "spec.agentID", func(o kclient.Object) []string { return []string{o.(*v1.AgentIdentityDelete).Get("spec.agentID")} }).
		WithIndex(&v1.UserGroupChange{}, "spec.userID", func(o kclient.Object) []string { return []string{o.(*v1.UserGroupChange).Get("spec.userID")} }).
		WithIndex(&v1.UserRoleChange{}, "spec.userID", func(o kclient.Object) []string { return []string{o.(*v1.UserRoleChange).Get("spec.userID")} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).
		Build()
	d.gateway = newDeliveryTestGateway(t)
	d.store = vmcpconfig.NewProfileConfigurationStore(d.gateway)
	d.instances = New(d.gateway)
	d.instances.userInfo = func(_ context.Context, owner principal.Principal) (kuser.Info, error) {
		if id, ok := owner.AgentID(); ok {
			return gateway.AgentUserInfo(&gatewaytypes.Agent{ID: id}, nil, nil), nil
		}
		return principaltest.AsUser(&kuser.DefaultInfo{UID: owner.String()}), nil
	}
	d.servers = mcpserver.New(d.gateway, nil, nil, false, false, 0, 0, "", "", nil)
	d.connections = mcpserverinstance.New(d.gateway)

	// The vMCP fixes the administrator token for everyone.
	require.NoError(t, d.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("dedicated", "ADMIN_TOKEN"): "fixed-admin-token"},
	}))
	// The user entered their own values.
	userValues := map[string]string{
		vmcpconfig.ConfigurationKey("dedicated", "REGION"): "user-region",
		vmcpconfig.ConfigurationKey("shared", "X-Token"):   "user-token",
	}
	require.NoError(t, d.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(d.user.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: userValues,
	}))
	d.user.Annotations = map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: utils.Digest(userValues)}
	require.NoError(t, d.client.Update(t.Context(), d.user))
	// The vMCP controller has recorded the profiles.
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
	vmcp.Status.ProfileIDs = vmcpconfig.ProfileIDs(vmcp.Spec.Manifest)
	require.NoError(t, d.client.Status().Update(t.Context(), vmcp))
	return d
}

func (d *profileDelivery) configure(profileID string, update map[string]map[string]*string) {
	d.t.Helper()
	_, err := d.store.Update(d.t.Context(), d.client, kclient.ObjectKeyFromObject(d.vmcp), profileID, update)
	require.NoError(d.t, err)
}

// syncInstance runs the vMCP instance controller's handlers for an instance, in their registered order, and returns
// the instance as stored.
func (d *profileDelivery) syncInstance(instance *v1.VMCPInstance) *v1.VMCPInstance {
	d.t.Helper()
	current := &v1.VMCPInstance{}
	require.NoError(d.t, d.client.Get(d.t.Context(), kclient.ObjectKeyFromObject(instance), current))
	req := router.Request{Ctx: d.t.Context(), Client: d.client, Object: current}
	require.NoError(d.t, d.instances.EnsureMCPServers(req, nil))
	require.NoError(d.t, d.instances.EnsureMCPServerInstances(req, nil))
	require.NoError(d.t, d.instances.SyncAgentProfileConfiguration(req, nil))
	require.NoError(d.t, d.instances.SyncUserConfigurationHash(req, nil))
	require.NoError(d.t, d.client.Get(d.t.Context(), kclient.ObjectKeyFromObject(instance), current))
	return current
}

// sync runs every controller involved in delivering configuration, for every instance.
func (d *profileDelivery) sync() {
	d.t.Helper()
	vmcp := &v1.VMCP{}
	require.NoError(d.t, d.client.Get(d.t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	require.NoError(d.t, vmcphandler.EnsureMCPServers(router.Request{Ctx: d.t.Context(), Client: d.client, Object: vmcp}, nil))
	for _, instance := range []*v1.VMCPInstance{d.seven, d.nine, d.user} {
		d.syncInstance(instance)
	}
	var servers v1.MCPServerList
	require.NoError(d.t, d.client.List(d.t.Context(), &servers))
	for i := range servers.Items {
		require.NoError(d.t, d.servers.SyncVMCPConfiguration(router.Request{Ctx: d.t.Context(), Client: d.client, Object: &servers.Items[i]}, nil))
	}
	var connections v1.MCPServerInstanceList
	require.NoError(d.t, d.client.List(d.t.Context(), &connections))
	for i := range connections.Items {
		require.NoError(d.t, d.connections.SyncVMCPConfiguration(router.Request{Ctx: d.t.Context(), Namespace: "default", Client: d.client, Object: &connections.Items[i]}, nil))
	}
}

// instanceValues returns the configuration credential of an instance.
func (d *profileDelivery) instanceValues(instance *v1.VMCPInstance) map[string]string {
	d.t.Helper()
	credential, err := d.gateway.RevealCredential(d.t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	if err != nil {
		require.ErrorAs(d.t, err, &gateway.CredentialNotFoundError{})
		return nil
	}
	return credential.Secrets
}

// dedicatedServerValues returns the configuration credential of an instance's dedicated component server.
func (d *profileDelivery) dedicatedServerValues(instance *v1.VMCPInstance) map[string]string {
	d.t.Helper()
	var servers v1.MCPServerList
	require.NoError(d.t, d.client.List(d.t.Context(), &servers, kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}))
	require.Len(d.t, servers.Items, 1)
	server := servers.Items[0]
	credential, err := d.gateway.RevealCredential(d.t.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	require.NoError(d.t, err)
	return credential.Secrets
}

// connectionValues returns the credential of an instance's connection to the shared component server, as the gateway
// reads it to set the connection's headers.
func (d *profileDelivery) connectionValues(instance *v1.VMCPInstance) map[string]string {
	d.t.Helper()
	var connections v1.MCPServerInstanceList
	require.NoError(d.t, d.client.List(d.t.Context(), &connections, kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}))
	require.Len(d.t, connections.Items, 1)
	connection := connections.Items[0]
	credential, err := d.gateway.RevealCredential(d.t.Context(), []string{fmt.Sprintf("%s-%s", connection.Spec.UserID, connection.Name)}, connection.Name)
	require.NoError(d.t, err)
	return credential.Secrets
}

// TestProfileConfigurationDelivery checks that agents' instances are given the configuration of the profiles that name
// them, through the credentials their component servers and shared connections are configured from, and nothing of
// any other profile's or of the user's, while the vMCP's fixed values reach agents unchanged.
func TestProfileConfigurationDelivery(t *testing.T) {
	d := newProfileDelivery(t)
	d.configure("vp1seven", map[string]map[string]*string{
		"dedicated": {"REGION": new("seven-region")},
		"shared":    {"X-Token": new("seven-token")},
	})
	d.configure("vp1agents", map[string]map[string]*string{
		"dedicated": {"REGION": new("agents-region"), "VERBOSE": new("agents-verbose")},
		"shared":    {"X-Token": new("agents-token")},
	})
	d.sync()

	// Agent 7 takes everything from its own profile. Its unset optional value does not fall back.
	require.Equal(t, map[string]string{
		vmcpconfig.ConfigurationKey("dedicated", "REGION"): "seven-region",
		vmcpconfig.ConfigurationKey("shared", "X-Token"):   "seven-token",
	}, d.instanceValues(d.seven))
	require.Equal(t, map[string]string{"REGION": "seven-region", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.seven))
	require.Equal(t, map[string]string{"X-Token": "seven-token"}, d.connectionValues(d.seven))
	// Agent 9 has no profile of its own, so it is given the wildcard's configuration as it is.
	require.Equal(t, map[string]string{
		vmcpconfig.ConfigurationKey("dedicated", "REGION"):  "agents-region",
		vmcpconfig.ConfigurationKey("dedicated", "VERBOSE"): "agents-verbose",
		vmcpconfig.ConfigurationKey("shared", "X-Token"):    "agents-token",
	}, d.instanceValues(d.nine))
	require.Equal(t, map[string]string{"REGION": "agents-region", "VERBOSE": "agents-verbose", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.nine))
	require.Equal(t, map[string]string{"X-Token": "agents-token"}, d.connectionValues(d.nine))
	// The user with agent 7's numeric ID keeps their own values.
	require.Equal(t, map[string]string{"REGION": "user-region", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.user))
	require.Equal(t, map[string]string{"X-Token": "user-token"}, d.connectionValues(d.user))

	// Agent 7's profile values reach nobody else.
	for _, instance := range []*v1.VMCPInstance{d.nine, d.user} {
		for _, values := range []map[string]string{d.instanceValues(instance), d.dedicatedServerValues(instance), d.connectionValues(instance)} {
			for _, value := range values {
				require.NotContains(t, value, "seven", "instance %s was given agent 7's profile configuration", instance.Name)
			}
		}
	}

	// The instances record the revisions they were resolved from.
	seven := d.syncInstance(d.seven)
	vmcp := &v1.VMCP{}
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	require.Equal(t, map[string]string{"vp1seven": vmcp.Status.ProfileConfigurationRevisions["vp1seven"].Name}, seven.Status.AgentConfigurationRevisions)
	require.Equal(t, seven.Status.AgentConfigurationRevision, seven.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
	require.True(t, seven.Status.Configured, "missing %v", seven.Status.MissingRequiredConfiguration)
	user := d.syncInstance(d.user)
	require.Empty(t, user.Status.AgentConfigurationRevision)
	require.Equal(t, utils.Digest(map[string]string{
		vmcpconfig.ConfigurationKey("dedicated", "REGION"): "user-region",
		vmcpconfig.ConfigurationKey("shared", "X-Token"):   "user-token",
	}), user.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
}

// TestProfileConfigurationChangeReachesAgentInstances checks that changing a profile's value updates the running
// agent instances it reaches, and leaves other agents' and users' instances alone; that removing an agent's own
// profile makes it fall back rather than lose its configuration; and that the superseded revisions are deleted once no
// instance uses them.
func TestProfileConfigurationChangeReachesAgentInstances(t *testing.T) {
	d := newProfileDelivery(t)
	d.configure("vp1seven", map[string]map[string]*string{"dedicated": {"REGION": new("seven-region")}, "shared": {"X-Token": new("seven-token")}})
	d.configure("vp1agents", map[string]map[string]*string{"dedicated": {"REGION": new("agents-region")}, "shared": {"X-Token": new("agents-token")}})
	d.sync()
	before := map[string]v1.VMCPInstance{}
	for _, instance := range []*v1.VMCPInstance{d.seven, d.nine, d.user} {
		before[instance.Name] = *d.syncInstance(instance)
	}

	d.configure("vp1seven", map[string]map[string]*string{"dedicated": {"REGION": new("seven-region-2")}})
	d.sync()
	require.Equal(t, map[string]string{"REGION": "seven-region-2", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.seven))
	require.Equal(t, map[string]string{"X-Token": "seven-token"}, d.connectionValues(d.seven))
	seven := d.syncInstance(d.seven)
	require.NotEqual(t, before[d.seven.Name].Status.AgentConfigurationRevision, seven.Status.AgentConfigurationRevision)
	require.NotEqual(t, before[d.seven.Name].Status.UserConfigurationHash, seven.Status.UserConfigurationHash)
	// Agent 9 does not use agent 7's profile, and the user uses no profile's configuration: neither is touched.
	for _, instance := range []*v1.VMCPInstance{d.nine, d.user} {
		current := d.syncInstance(instance)
		require.Equal(t, before[instance.Name].ResourceVersion, current.ResourceVersion, "instance %s changed", instance.Name)
	}
	require.Equal(t, map[string]string{"REGION": "user-region", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.user))

	// The revision agent 7 was first resolved from is superseded, and deleted once collected.
	vmcp := &v1.VMCP{}
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	superseded := vmcp.Status.ProfileConfigurationRevisions["vp1seven"].Superseded
	require.Len(t, superseded, 1)
	collector := vmcphandler.New(d.gateway, nil, vmcpconfig.ProfileDataStores{d.store}, d.store)
	require.NoError(t, collector.CollectProfileConfiguration(router.Request{Ctx: t.Context(), Client: d.client, Object: vmcp}, nil))
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	require.Empty(t, vmcp.Status.ProfileConfigurationRevisions["vp1seven"].Superseded)
	_, err := d.store.RevisionValues(t.Context(), vmcp.Name, "vp1seven", superseded[0])
	require.ErrorAs(t, err, &gateway.CredentialNotFoundError{})

	// Agent 7 leaves its own profile: it falls back to the wildcard's configuration for every value.
	vmcp.Spec.Manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}
	require.NoError(t, d.client.Update(t.Context(), vmcp))
	d.sync()
	require.Equal(t, map[string]string{"REGION": "agents-region", "ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.seven))
	require.Equal(t, map[string]string{"X-Token": "agents-token"}, d.connectionValues(d.seven))
	require.Equal(t, map[string]string{"vp1agents": vmcp.Status.ProfileConfigurationRevisions["vp1agents"].Name}, d.syncInstance(d.seven).Status.AgentConfigurationRevisions)

	// Clearing the wildcard's values leaves agent 7 with nothing, which its instance reports, rather than any value
	// from before.
	_, _, err = d.store.Reset(t.Context(), d.client, kclient.ObjectKeyFromObject(d.vmcp), "vp1agents")
	require.NoError(t, err)
	d.sync()
	require.Empty(t, d.instanceValues(d.seven))
	seven = d.syncInstance(d.seven)
	require.False(t, seven.Status.Configured)
	require.Equal(t, []string{vmcpconfig.ConfigurationKey("dedicated", "REGION"), vmcpconfig.ConfigurationKey("shared", "X-Token")}, seven.Status.MissingRequiredConfiguration)
	require.Equal(t, map[string]string{"ADMIN_TOKEN": "fixed-admin-token"}, d.dedicatedServerValues(d.seven))

	// The instances, synced again, no longer use the reset profile's revision, so collection deletes it and the profile
	// has no data left, which is what a reset waits for.
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	require.NotEmpty(t, vmcp.Status.ProfileConfigurationRevisions["vp1agents"].Superseded)
	require.NoError(t, collector.CollectProfileConfiguration(router.Request{Ctx: t.Context(), Client: d.client, Object: vmcp}, nil))
	require.NoError(t, d.client.Get(t.Context(), kclient.ObjectKeyFromObject(d.vmcp), vmcp))
	require.NotContains(t, vmcp.Status.ProfileConfigurationRevisions, "vp1agents")
	withData, err := d.store.ProfilesWithData(t.Context(), vmcp.Name, []string{"vp1agents"})
	require.NoError(t, err)
	require.Empty(t, withData)
}

// TestProfileConfigurationReplacesWhatWasEnteredOnAnAgentInstance checks that an agent's instance holds only what its
// profiles give it, even when an administrator entered values on it before profile configuration existed.
func TestProfileConfigurationReplacesWhatWasEnteredOnAnAgentInstance(t *testing.T) {
	d := newProfileDelivery(t)
	require.NoError(t, d.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(d.seven.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("dedicated", "REGION"): "entered-on-the-instance"},
	}))
	d.sync()
	require.Empty(t, d.instanceValues(d.seven))

	d.configure("vp1agents", map[string]map[string]*string{"dedicated": {"VERBOSE": new("agents-verbose")}})
	d.sync()
	require.Empty(t, d.instanceValues(d.seven), "an optional value fell back to agent 7, which has its own profile")
	require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("dedicated", "VERBOSE"): "agents-verbose"}, d.instanceValues(d.nine))
}
