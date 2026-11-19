package cleanup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	nahfields "github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserverinstance"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	errInjected = errors.New("injected failure")
)

// failer fails the failAt-th mutation it is asked about, once. Zero never fails, and only counts.
type failer struct {
	calls  int
	failAt int
}

// agentCleanupFixture is a deleted agent, the user with the agent's numeric ID and another agent, each owning the
// same kinds of resources, and policies naming them.
type agentCleanupFixture struct {
	gateway    *gclient.Client
	db         *gorm.DB
	storage    kclient.WithWatch
	agentID    uint
	otherAgent uint
	event      *v1.AgentIdentityDelete
	// policies are the objects naming the agent, as they were before the cleanup.
	policies []kclient.Object
}

// unwrappingClient reads the objects that nah's untriggered package wraps, as the controller's client does, which
// the fake client cannot.
type unwrappingClient struct {
	kclient.WithWatch
}

func (f *failer) next() error {
	f.calls++
	if f.calls == f.failAt {
		return errInjected
	}
	return nil
}

func (c unwrappingClient) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	if holder, ok := obj.(*untriggered.Holder); ok {
		obj = holder.Object
	}
	return c.WithWatch.Get(ctx, key, obj, opts...)
}

// newAgentCleanupGateway returns a gateway client on an in-memory database, and that database for writing rows the
// client has no method for.
func newAgentCleanupGateway(t *testing.T) (*gclient.Client, *gorm.DB) {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())

	client := gclient.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close gateway client: %v", err)
		}
	})
	return client, storageServices.DB.DB
}

// withOwnerIndexes indexes spec.userID of the owned types, which the cleanups list by.
func withOwnerIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	for _, obj := range []kclient.Object{
		&v1.VMCP{}, &v1.VMCPInstance{}, &v1.MCPServer{}, &v1.MCPServerInstance{}, &v1.PowerUserWorkspace{}, &v1.AgentInstance{},
	} {
		builder.WithIndex(obj, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(nahfields.Fields).Get("spec.userID")}
		})
	}
	return builder
}

func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: name}
}

func agentSubject(id uint) types.Subject {
	return types.Subject{Type: types.SubjectTypeAgent, ID: strconv.FormatUint(uint64(id), 10)}
}

func userSubject(id uint) types.Subject {
	return types.Subject{Type: types.SubjectTypeUser, ID: strconv.FormatUint(uint64(id), 10)}
}

func storedObjectExists(t *testing.T, storage kclient.Client, obj kclient.Object) bool {
	t.Helper()
	err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj)
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

// newAgentCleanupFixture builds the fixture with a deleted agent. failures, if not nil, fails storage mutations.
func newAgentCleanupFixture(t *testing.T, failures *failer) *agentCleanupFixture {
	t.Helper()
	ctx := t.Context()

	gateway, db := newAgentCleanupGateway(t)
	agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "deploy bot", CreatorUserID: 3})
	require.NoError(t, err)
	other, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "triage bot", CreatorUserID: 3})
	require.NoError(t, err)
	f := &agentCleanupFixture{
		gateway:    gateway,
		db:         db,
		agentID:    agent.ID,
		otherAgent: other.ID,
	}

	for _, id := range []uint{agent.ID, other.ID} {
		_, err := gateway.CreateAgentAPIKey(ctx, id, gatewaytypes.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true})
		require.NoError(t, err)
	}
	// History of the agent, which the cleanup keeps.
	require.NoError(t, db.Create(&gatewaytypes.MCPAuditLog{PrincipalKind: string(principal.KindAgent), PrincipalID: strconv.FormatUint(uint64(agent.ID), 10)}).Error)
	require.NoError(t, db.Create(&gatewaytypes.LLMGatewayBudgetUsage{BudgetID: "budget", UserID: principal.Agent(agent.ID).String()}).Error)

	require.NoError(t, gateway.DeleteAgent(ctx, agent.ID))

	f.event = &v1.AgentIdentityDelete{
		ObjectMeta: objectMeta(system.AgentIdentityDeletePrefix + "agent-lifecycle-1"),
		Spec:       v1.AgentIdentityDeleteSpec{AgentID: agent.ID},
	}
	f.policies = []kclient.Object{
		&v1.VMCP{ObjectMeta: objectMeta("vmcp1shared"), Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{
			{ID: "mixed", Name: "mixed", Subjects: []types.Subject{agentSubject(agent.ID), userSubject(agent.ID), agentSubject(other.ID)}},
			{ID: "only", Name: "only", Subjects: []types.Subject{agentSubject(agent.ID)}},
		}}}},
		&v1.ModelAccessPolicy{ObjectMeta: objectMeta("map-agent"), Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Subjects: []types.Subject{agentSubject(agent.ID)}}}},
		&v1.SkillAccessRule{ObjectMeta: objectMeta("sar-agent"), Spec: v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{Subjects: []types.Subject{agentSubject(agent.ID)}}}},
		&v1.MCPWebhookValidation{ObjectMeta: objectMeta("filter-agent"), Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{Guardrail: &types.MCPGuardrailFilterConfig{
			Subjects: []types.MCPGuardrailFilterSubject{{Type: types.MCPGuardrailSubjectAgent, ID: strconv.FormatUint(uint64(agent.ID), 10)}},
		}}}},
	}
	objects := append([]kclient.Object{
		f.event,
		// Component resources of the agent's instance go through reference cleanup, and a multi-user server of the
		// default catalog is shared, as for users.
		&v1.MCPServer{ObjectMeta: objectMeta("ms1agent-component"), Spec: v1.MCPServerSpec{UserID: principal.Agent(agent.ID).String(), VMCPInstanceID: "vmcpi1agent", VMCPComponentID: "github"}},
		&v1.MCPServerInstance{ObjectMeta: objectMeta("msi1agent-component"), Spec: v1.MCPServerInstanceSpec{UserID: principal.Agent(agent.ID).String(), VMCPInstanceID: "vmcpi1agent", VMCPComponentID: "github"}},
		&v1.MCPServer{ObjectMeta: objectMeta("ms1agent-multiuser"), Spec: v1.MCPServerSpec{UserID: principal.Agent(agent.ID).String(), MCPCatalogID: system.DefaultCatalog}},
	}, f.policies...)
	for tag, owner := range f.owners() {
		objects = append(objects,
			&v1.VMCPInstance{ObjectMeta: objectMeta("vmcpi1" + tag), Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1shared"}}},
			&v1.MCPServer{ObjectMeta: objectMeta("ms1" + tag), Spec: v1.MCPServerSpec{UserID: owner}},
			&v1.MCPServerInstance{ObjectMeta: objectMeta("msi1" + tag), Spec: v1.MCPServerInstanceSpec{UserID: owner}},
		)
	}
	builder := withOwnerIndexes(fake.NewClientBuilder().WithScheme(scheme.Scheme)).WithObjects(objects...)
	if failures != nil {
		builder.WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.DeleteOption) error {
				if err := failures.next(); err != nil {
					return err
				}
				return c.Delete(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
				if err := failures.next(); err != nil {
					return err
				}
				return c.Update(ctx, obj, opts...)
			},
		})
	}
	f.storage = builder.Build()
	// Record the stored versions of the policies, which the cleanup must not change.
	for _, policy := range f.policies {
		require.True(t, storedObjectExists(t, f.storage, policy))
	}
	return f
}

// owners are the principals of the fixture's resources by tag: the deleted agent, the user with its numeric ID, and
// another agent.
func (f *agentCleanupFixture) owners() map[string]string {
	return map[string]string{
		"agent": principal.Agent(f.agentID).String(),
		"user":  principal.User(f.agentID).String(),
		"other": principal.Agent(f.otherAgent).String(),
	}
}

// request returns the request for the event as the controller hands it to the handler: read again for each attempt.
func (f *agentCleanupFixture) request(t *testing.T) router.Request {
	t.Helper()
	event := &v1.AgentIdentityDelete{ObjectMeta: objectMeta(f.event.Name)}
	if !storedObjectExists(t, f.storage, event) {
		// A stale copy, as a repeated delivery could hand over after the event is gone.
		event = f.event.DeepCopy()
	}
	return router.Request{
		Ctx:       t.Context(),
		Client:    f.storage,
		Object:    event,
		Namespace: event.Namespace,
		Name:      event.Name,
	}
}

func (f *agentCleanupFixture) activeKeys(t *testing.T, agentID uint) int {
	t.Helper()
	keys, err := f.gateway.ListAgentAPIKeys(t.Context(), agentID, gclient.APIKeyListOptions{})
	require.NoError(t, err)
	return len(keys)
}

// requireOwnedResources checks which of each owner's resources exist.
func (f *agentCleanupFixture) requireOwnedResources(t *testing.T, wantAgentResources bool) {
	t.Helper()
	for tag := range f.owners() {
		want := tag != "agent" || wantAgentResources
		for _, obj := range []kclient.Object{
			&v1.VMCPInstance{ObjectMeta: objectMeta("vmcpi1" + tag)},
			&v1.MCPServer{ObjectMeta: objectMeta("ms1" + tag)},
			&v1.MCPServerInstance{ObjectMeta: objectMeta("msi1" + tag)},
		} {
			require.Equal(t, want, storedObjectExists(t, f.storage, obj), "%T %s exists", obj, obj.GetName())
		}
	}
	for _, obj := range []kclient.Object{
		&v1.MCPServer{ObjectMeta: objectMeta("ms1agent-component")},
		&v1.MCPServerInstance{ObjectMeta: objectMeta("msi1agent-component")},
		&v1.MCPServer{ObjectMeta: objectMeta("ms1agent-multiuser")},
	} {
		require.True(t, storedObjectExists(t, f.storage, obj), "%T %s exists", obj, obj.GetName())
	}
}

// requirePoliciesUnchanged checks that the policies naming the agent were not written, so that its subjects stay
// where they were, as a deleted user's do.
func (f *agentCleanupFixture) requirePoliciesUnchanged(t *testing.T) {
	t.Helper()
	for _, policy := range f.policies {
		current := policy.DeepCopyObject().(kclient.Object)
		require.True(t, storedObjectExists(t, f.storage, current))
		require.Equal(t, policy.GetResourceVersion(), current.GetResourceVersion(), "%T %s is unchanged", policy, policy.GetName())
	}
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "vmcp1shared"}, &vmcp))
	require.Equal(t, []types.Subject{agentSubject(f.agentID)}, vmcp.Spec.Manifest.Profiles[1].Subjects)
}

// requireCleanedUp checks the state after a completed deletion cleanup.
func (f *agentCleanupFixture) requireCleanedUp(t *testing.T) {
	t.Helper()
	require.False(t, storedObjectExists(t, f.storage, &v1.AgentIdentityDelete{ObjectMeta: objectMeta(f.event.Name)}), "event is deleted")
	f.requireOwnedResources(t, false)
	f.requirePoliciesUnchanged(t)

	require.Zero(t, f.activeKeys(t, f.agentID))
	require.Equal(t, 1, f.activeKeys(t, f.otherAgent))

	// History is kept, and the agent stays resolvable for it.
	var audits, usage int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPAuditLog{}).Count(&audits).Error)
	require.NoError(t, f.db.Model(&gatewaytypes.LLMGatewayBudgetUsage{}).Count(&usage).Error)
	require.Equal(t, []int64{1, 1}, []int64{audits, usage})
	agent := new(gatewaytypes.Agent)
	require.NoError(t, f.db.Unscoped().Where("id = ?", f.agentID).First(agent).Error)
	require.Equal(t, "deploy bot", agent.DisplayName)
}

func TestAgentCleanupDeleteRemovesOnlyTheAgentsResources(t *testing.T) {
	f := newAgentCleanupFixture(t, nil)

	require.NoError(t, DeletedAgent(f.request(t), nil))
	f.requireCleanedUp(t)

	// Handling a stale copy of the event again finds nothing left and changes nothing.
	require.NoError(t, DeletedAgent(f.request(t), nil))
	f.requireCleanedUp(t)
}

func TestAgentCleanupDeleteResumesAfterAFailureAtEveryStep(t *testing.T) {
	// A clean run counts the mutations that a failure can interrupt: the three deletions of owned resources, and the
	// event's deletion.
	counter := &failer{}
	f := newAgentCleanupFixture(t, counter)
	require.NoError(t, DeletedAgent(f.request(t), nil))
	f.requireCleanedUp(t)
	require.Equal(t, 4, counter.calls)

	for failAt := 1; failAt <= counter.calls; failAt++ {
		t.Run(fmt.Sprintf("failure at mutation %d", failAt), func(t *testing.T) {
			failures := &failer{failAt: failAt}
			f := newAgentCleanupFixture(t, failures)

			require.ErrorIs(t, DeletedAgent(f.request(t), nil), errInjected)
			// The event survives a failed attempt, so the controller retries it.
			require.True(t, storedObjectExists(t, f.storage, &v1.AgentIdentityDelete{ObjectMeta: objectMeta(f.event.Name)}))

			require.NoError(t, DeletedAgent(f.request(t), nil))
			f.requireCleanedUp(t)
		})
	}
}

// TestAgentCleanupEnableNeedsNoRepair disables an agent and enables it again, handling each change as the controller
// does: everything it was assigned is still there, and its key works again.
func TestAgentCleanupEnableNeedsNoRepair(t *testing.T) {
	ctx := t.Context()
	gateway, _ := newAgentCleanupGateway(t)
	agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "deploy bot", CreatorUserID: 3})
	require.NoError(t, err)
	key, err := gateway.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "deploy", CanAccessLLMProxy: true})
	require.NoError(t, err)
	instance := &v1.VMCPInstance{ObjectMeta: objectMeta("vmcpi1agent"), Spec: v1.VMCPInstanceSpec{UserID: principal.Agent(agent.ID).String()}}
	policy := &v1.ModelAccessPolicy{ObjectMeta: objectMeta("map-agent"), Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Subjects: []types.Subject{agentSubject(agent.ID)}}}}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(instance, policy).Build()

	for i, step := range []struct {
		lifecycle  func() error
		wantStatus types.UserStatus
	}{
		{
			lifecycle: func() error {
				_, err := gateway.DisableAgent(ctx, agent.ID, "paused")
				return err
			},
			wantStatus: types.UserStatusDisabled,
		},
		{
			lifecycle: func() error {
				_, err := gateway.EnableAgent(ctx, agent.ID)
				return err
			},
			wantStatus: types.UserStatusActive,
		},
	} {
		require.NoError(t, step.lifecycle())
		change := &v1.AgentIdentityChange{
			ObjectMeta: objectMeta(fmt.Sprintf("%sagent-lifecycle-%d", system.AgentIdentityChangePrefix, i)),
			Spec: v1.AgentIdentityChangeSpec{
				AgentID: agent.ID,
			},
		}
		require.NoError(t, storage.Create(ctx, change))
		require.NoError(t, DeleteAgentIdentityEvent(router.Request{Ctx: ctx, Client: storage, Object: change, Namespace: change.Namespace, Name: change.Name}, nil))
		require.False(t, storedObjectExists(t, storage, change.DeepCopy()))

		// Authentication refuses the key only while the agent is disabled (spec 04).
		validated, err := gateway.ValidateAPIKey(ctx, key.Key)
		require.NoError(t, err)
		require.Equal(t, step.wantStatus, validated.OwnerStatus)
		require.True(t, storedObjectExists(t, storage, instance.DeepCopy()))
		current := policy.DeepCopy()
		require.True(t, storedObjectExists(t, storage, current))
		require.Equal(t, []types.Subject{agentSubject(agent.ID)}, current.Spec.Manifest.Subjects)
	}
}

// TestAgentCleanupDeleteCascadesToConnectionCredentials follows the deletion of an agent's VMCPInstance through
// reference cleanup and the finalizers, as for a user, and checks that the connection credential the configuration
// sync wrote in the "<owner>-<connection>" context, the instance's configuration credential and the connection's MCP
// OAuth tokens are deleted, while the user with the agent's numeric ID keeps theirs.
func TestAgentCleanupDeleteCascadesToConnectionCredentials(t *testing.T) {
	ctx := t.Context()
	gateway, db := newAgentCleanupGateway(t)
	agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "deploy bot", CreatorUserID: 3})
	require.NoError(t, err)
	require.NoError(t, gateway.DeleteAgent(ctx, agent.ID))

	component := types.VMCPComponent{
		ID:            "github",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "TOKEN", Usage: types.Header, Required: true}},
		}},
	}
	vmcp := &v1.VMCP{ObjectMeta: objectMeta("vmcp1shared"), Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}},
	}}
	server := &v1.MCPServer{ObjectMeta: objectMeta("ms1shared"), Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: component.ID,
	}}
	event := &v1.AgentIdentityDelete{
		ObjectMeta: objectMeta(system.AgentIdentityDeletePrefix + "agent-lifecycle-1"),
		Spec:       v1.AgentIdentityDeleteSpec{AgentID: agent.ID},
	}
	objects := []kclient.Object{vmcp, server, event}
	owners := map[string]string{
		"agent": principal.Agent(agent.ID).String(),
		"user":  principal.User(agent.ID).String(),
	}
	for tag, owner := range owners {
		connection := "msi1" + tag
		objects = append(objects,
			&v1.VMCPInstance{ObjectMeta: objectMeta("vmcpi1" + tag), Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}},
			&v1.MCPServerInstance{ObjectMeta: objectMeta(connection), Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  "vmcpi1" + tag,
				VMCPComponentID: component.ID,
				MCPServerName:   server.Name,
			}},
		)
		require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: vmcpconfig.InstanceConfigurationCredentialContext("vmcpi1" + tag),
			Name:    vmcpconfig.ConfigurationCredentialName(),
			Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): tag + "-token"},
		}))
		require.NoError(t, db.Create(&gatewaytypes.MCPOAuthToken{MCPID: connection, UserID: owner, AccessToken: "token"}).Error)
	}
	storage := withOwnerIndexes(fake.NewClientBuilder().WithScheme(scheme.Scheme)).
		WithStatusSubresource(&v1.MCPServerInstance{}).
		WithObjects(objects...).
		Build()
	request := func(obj kclient.Object) router.Request {
		require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(obj), obj))
		return router.Request{Ctx: ctx, Client: storage, Object: obj, Namespace: obj.GetNamespace(), Name: obj.GetName()}
	}

	// The configuration sync writes each connection's credential. An agent's carries nothing from its instance's
	// configuration credential, which is never read for an agent.
	sync := mcpserverinstance.New(gateway)
	connectionContext := map[string]string{}
	for tag, owner := range owners {
		connection := &v1.MCPServerInstance{ObjectMeta: objectMeta("msi1" + tag)}
		require.NoError(t, sync.SyncVMCPConfiguration(request(connection), nil))
		connectionContext[tag] = owner + "-" + connection.Name
		credential, err := gateway.RevealCredential(ctx, []string{connectionContext[tag]}, connection.Name)
		require.NoError(t, err)
		want := map[string]string{}
		if tag == "user" {
			want["TOKEN"] = "user-token"
		}
		require.Equal(t, want, credential.Secrets, tag)
	}

	require.NoError(t, DeletedAgent(request(event), nil))

	// Reference cleanup deletes the connection whose instance is gone, and the finalizers of the connection and the
	// instance delete their credentials and the connection's MCP OAuth tokens.
	credentials := NewCredentials(nil, gateway, "")
	for tag := range owners {
		connection := &v1.MCPServerInstance{ObjectMeta: objectMeta("msi1" + tag)}
		cleanupRequest := request(connection)
		cleanupRequest.Client = unwrappingClient{WithWatch: storage}
		require.NoError(t, Cleanup(cleanupRequest, nil))
		gone := !storedObjectExists(t, storage, connection)
		require.Equal(t, tag == "agent", gone, "connection of %s is deleted", tag)
		if gone {
			require.NoError(t, credentials.RemoveMCPInstanceCredentials(router.Request{Ctx: ctx, Client: storage, Object: connection, Namespace: connection.Namespace, Name: connection.Name}, nil))
			require.NoError(t, credentials.RemoveVMCPInstanceConfigurationCredentials(router.Request{Ctx: ctx, Client: storage, Object: &v1.VMCPInstance{}, Namespace: connection.Namespace, Name: "vmcpi1" + tag}, nil))
		}
	}
	for tag, owner := range owners {
		want := tag != "agent"
		require.Equal(t, want, credentialExists(t, gateway, connectionContext[tag], "msi1"+tag), "connection credential of %s", tag)
		require.Equal(t, want, credentialExists(t, gateway, vmcpconfig.InstanceConfigurationCredentialContext("vmcpi1"+tag), vmcpconfig.ConfigurationCredentialName()), "instance credential of %s", tag)
		var tokens int64
		require.NoError(t, db.Model(&gatewaytypes.MCPOAuthToken{}).Where("user_id = ?", owner).Count(&tokens).Error)
		require.Equal(t, want, tokens == 1, "MCP OAuth token of %s", tag)
	}
}
