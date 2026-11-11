package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// countingCreates counts the creations of each object name, including those that fail because it exists.
type countingCreates struct {
	kclient.Client
	lock    sync.Mutex
	creates map[string]int
}

func (c *countingCreates) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	c.lock.Lock()
	c.creates[obj.GetName()]++
	c.lock.Unlock()
	return c.Client.Create(ctx, obj, opts...)
}

func createTestAgent(t *testing.T, c *Client, displayName string) *types.Agent {
	t.Helper()

	agent, err := c.CreateAgent(t.Context(), types.Agent{
		DisplayName:   displayName,
		Description:   displayName + " runs the nightly build",
		OwnerUserIDs:  []uint{1},
		CreatorUserID: 1,
	})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	return agent
}

func storedAgent(t *testing.T, c *Client, id uint) types.Agent {
	t.Helper()

	var agent types.Agent
	if err := c.db.WithContext(t.Context()).Unscoped().Where("id = ?", id).Take(&agent).Error; err != nil {
		t.Fatalf("failed to read agent %d: %v", id, err)
	}
	return agent
}

func agentLifecycleEvents(t *testing.T, c *Client, agentID uint) []types.AgentLifecycleEvent {
	t.Helper()

	var events []types.AgentLifecycleEvent
	if err := c.db.WithContext(t.Context()).Where("agent_id = ?", agentID).Order("id").Find(&events).Error; err != nil {
		t.Fatalf("failed to list agent lifecycle events: %v", err)
	}
	return events
}

func agentLifecycleEventTypes(events []types.AgentLifecycleEvent) []types.AgentLifecycleEventType {
	eventTypes := make([]types.AgentLifecycleEventType, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.Type)
	}
	return eventTypes
}

// kicked reports whether lifecycle delivery was kicked since the last call, and consumes the kick.
func kicked(c *Client) bool {
	select {
	case <-c.kickLifecycleDelivery:
		return true
	default:
		return false
	}
}

func TestCreateAgentWritesOnlyMetadata(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now()

	agent, err := c.CreateAgent(ctx, types.Agent{
		ID:             42,
		DisplayName:    "builder",
		Description:    "runs builds",
		OwnerUserIDs:   []uint{3, 4},
		CreatorUserID:  3,
		DisabledAt:     &now,
		DisabledReason: "stale copy",
		DeletedAt: gorm.DeletedAt{
			Time:  now,
			Valid: true,
		},
	})
	require.NoError(t, err)
	require.NotZero(t, agent.ID)
	require.NotEqual(t, uint(42), agent.ID)

	stored := storedAgent(t, c, agent.ID)
	require.Equal(t, "builder", stored.DisplayName)
	require.Equal(t, "runs builds", stored.Description)
	require.Equal(t, []uint{3, 4}, stored.OwnerUserIDs)
	require.Equal(t, uint(3), stored.CreatorUserID)
	require.Equal(t, apitypes.UserStatusActive, stored.Status())
	require.Empty(t, stored.DisabledReason)
	require.Empty(t, agentLifecycleEvents(t, c, agent.ID))

	// Display names need not be unique.
	again, err := c.CreateAgent(ctx, types.Agent{
		DisplayName: "builder",
	})
	require.NoError(t, err)
	require.NotEqual(t, agent.ID, again.ID)

	_, err = c.CreateAgent(ctx, types.Agent{
		DisplayName: "  ",
	})
	require.ErrorContains(t, err, "display name is required")
	require.ErrorIs(t, err, ErrInvalidAgentMetadata)
}

func TestUpdateAgentWritesOnlyMetadata(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	agent := createTestAgent(t, c, "builder")
	_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
	require.NoError(t, err)

	// A stale copy of the agent, from before it was disabled, with changed metadata, creator, and lifecycle.
	stale := *agent
	stale.DisplayName = "deployer"
	stale.Description = ""
	stale.OwnerUserIDs = []uint{}
	stale.CreatorUserID = 9
	updated, err := c.UpdateAgent(ctx, stale)
	require.NoError(t, err)
	require.Equal(t, "deployer", updated.DisplayName)
	require.Equal(t, apitypes.UserStatusDisabled, updated.Status())

	stored := storedAgent(t, c, agent.ID)
	require.Equal(t, "deployer", stored.DisplayName)
	require.Empty(t, stored.Description)
	require.Empty(t, stored.OwnerUserIDs)
	require.Equal(t, agent.CreatorUserID, stored.CreatorUserID)
	require.Equal(t, apitypes.UserStatusDisabled, stored.Status())
	require.Equal(t, "rotating keys", stored.DisabledReason)
	require.Equal(t, []types.AgentLifecycleEventType{types.AgentLifecycleEventDisabled}, agentLifecycleEventTypes(agentLifecycleEvents(t, c, agent.ID)))

	_, err = c.UpdateAgent(ctx, types.Agent{
		ID:          agent.ID,
		DisplayName: "",
	})
	require.ErrorContains(t, err, "display name is required")
	require.ErrorIs(t, err, ErrInvalidAgentMetadata)

	require.NoError(t, c.DeleteAgent(ctx, agent.ID))
	stale.DisplayName = "resurrected"
	_, err = c.UpdateAgent(ctx, stale)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Equal(t, "deployer", storedAgent(t, c, agent.ID).DisplayName)
	require.Equal(t, apitypes.UserStatusDeleted, storedAgent(t, c, agent.ID).Status())

	_, err = c.UpdateAgent(ctx, types.Agent{
		ID:          agent.ID + 1,
		DisplayName: "missing",
	})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestDeletedAgentsAreListedOnlyOnRequest(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	kept := createTestAgent(t, c, "kept")
	deleted := createTestAgent(t, c, "deleted")
	require.NoError(t, c.DeleteAgent(ctx, deleted.ID))

	agents, err := c.ListAgents(ctx, false)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	require.Equal(t, kept.ID, agents[0].ID)

	agents, err = c.ListAgents(ctx, true)
	require.NoError(t, err)
	require.Len(t, agents, 2)
	require.Equal(t, []uint{kept.ID, deleted.ID}, []uint{agents[0].ID, agents[1].ID})
	require.Equal(t, apitypes.UserStatusDeleted, agents[1].Status())

	_, err = c.GetAgent(ctx, deleted.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Audit displays can still show who a deleted agent was.
	stored := storedAgent(t, c, deleted.ID)
	require.Equal(t, "deleted", stored.DisplayName)
	require.Equal(t, apitypes.UserStatusDeleted, stored.Status())

	got, err := c.GetAgent(ctx, kept.ID)
	require.NoError(t, err)
	require.Equal(t, "kept", got.DisplayName)
}

func TestAgentLifecycleChangesRecordOneEventEach(t *testing.T) {
	c := newTestClient(t)
	c.kickLifecycleDelivery = make(chan struct{}, 1)
	ctx := t.Context()
	agent := createTestAgent(t, c, "builder")

	// The steps run in order on one agent.
	for _, step := range []struct {
		name       string
		change     func() error
		wantErr    error
		wantStatus apitypes.UserStatus
		wantReason string
		wantEvents []types.AgentLifecycleEventType
		wantKick   bool
	}{
		{
			name: "disable",
			change: func() error {
				_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
				return err
			},
			wantStatus: apitypes.UserStatusDisabled,
			wantReason: "rotating keys",
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
			},
			wantKick: true,
		},
		{
			name: "disable again",
			change: func() error {
				_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
				return err
			},
			wantStatus: apitypes.UserStatusDisabled,
			wantReason: "rotating keys",
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
			},
		},
		{
			name: "disable again with another reason",
			change: func() error {
				_, err := c.DisableAgent(ctx, agent.ID, "compromised")
				return err
			},
			wantStatus: apitypes.UserStatusDisabled,
			wantReason: "compromised",
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
			},
		},
		{
			name: "enable",
			change: func() error {
				_, err := c.EnableAgent(ctx, agent.ID)
				return err
			},
			wantStatus: apitypes.UserStatusActive,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
			},
			wantKick: true,
		},
		{
			name: "enable again",
			change: func() error {
				_, err := c.EnableAgent(ctx, agent.ID)
				return err
			},
			wantStatus: apitypes.UserStatusActive,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
			},
		},
		{
			name: "delete",
			change: func() error {
				return c.DeleteAgent(ctx, agent.ID)
			},
			wantStatus: apitypes.UserStatusDeleted,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
				types.AgentLifecycleEventDeleted,
			},
			wantKick: true,
		},
		{
			name: "delete again",
			change: func() error {
				return c.DeleteAgent(ctx, agent.ID)
			},
			wantErr:    gorm.ErrRecordNotFound,
			wantStatus: apitypes.UserStatusDeleted,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
				types.AgentLifecycleEventDeleted,
			},
		},
		{
			name: "disable after delete",
			change: func() error {
				_, err := c.DisableAgent(ctx, agent.ID, "too late")
				return err
			},
			wantErr:    gorm.ErrRecordNotFound,
			wantStatus: apitypes.UserStatusDeleted,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
				types.AgentLifecycleEventDeleted,
			},
		},
		{
			name: "enable after delete",
			change: func() error {
				_, err := c.EnableAgent(ctx, agent.ID)
				return err
			},
			wantErr:    gorm.ErrRecordNotFound,
			wantStatus: apitypes.UserStatusDeleted,
			wantEvents: []types.AgentLifecycleEventType{
				types.AgentLifecycleEventDisabled,
				types.AgentLifecycleEventEnabled,
				types.AgentLifecycleEventDeleted,
			},
		},
	} {
		err := step.change()
		if step.wantErr != nil {
			require.ErrorIs(t, err, step.wantErr, step.name)
		} else {
			require.NoError(t, err, step.name)
		}

		stored := storedAgent(t, c, agent.ID)
		require.Equal(t, step.wantStatus, stored.Status(), step.name)
		if step.wantStatus != apitypes.UserStatusDeleted {
			require.Equal(t, step.wantReason, stored.DisabledReason, step.name)
		}
		requireAgentAccess(t, c, agent.ID, step.wantStatus, step.name)
		require.Equal(t, step.wantEvents, agentLifecycleEventTypes(agentLifecycleEvents(t, c, agent.ID)), step.name)
		require.Equal(t, step.wantKick, kicked(c), step.name)
	}

	// An agent that never existed has no access.
	requireAgentAccess(t, c, agent.ID+1, apitypes.UserStatusDeleted, "missing agent")
}

// requireAgentAccess checks that ActiveAgent returns the agent with id when want is active, and otherwise denies it
// with want.
func requireAgentAccess(t *testing.T, c *Client, id uint, want apitypes.UserStatus, msg string) {
	t.Helper()
	agent, err := c.ActiveAgent(t.Context(), id)
	if want == apitypes.UserStatusActive {
		require.NoError(t, err, msg)
		require.Equal(t, id, agent.ID, msg)
		return
	}
	require.Nil(t, agent, msg)
	denied, ok := errors.AsType[*AgentAccessDeniedError](err)
	require.True(t, ok, "%s: error = %v, want a denial", msg, err)
	require.Equal(t, id, denied.AgentID, msg)
	require.Equal(t, want, denied.Status, msg)
}

func TestAgentLifecycleChangesRollBackWithTheirEvent(t *testing.T) {
	for _, tt := range []struct {
		name string
		// disabled disables the agent before the change.
		disabled bool
		change   func(tx *gorm.DB, id uint) error
	}{
		{
			name: "disable",
			change: func(tx *gorm.DB, id uint) error {
				_, _, err := disableAgentTx(tx, id, "rotating keys")
				return err
			},
		},
		{
			name:     "enable",
			disabled: true,
			change: func(tx *gorm.DB, id uint) error {
				_, _, err := enableAgentTx(tx, id)
				return err
			},
		},
		{
			name: "delete",
			change: func(tx *gorm.DB, id uint) error {
				_, err := deleteAgentTx(tx, id)
				return err
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			ctx := t.Context()
			agent := createTestAgent(t, c, "builder")
			if tt.disabled {
				_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
				require.NoError(t, err)
			}
			before := storedAgent(t, c, agent.ID)
			eventsBefore := agentLifecycleEvents(t, c, agent.ID)

			// A transaction that fails after the change keeps neither the change nor its event.
			rollback := errors.New("rollback")
			err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tt.change(tx, agent.ID); err != nil {
					return err
				}
				var pending int64
				if err := tx.Model(new(types.AgentLifecycleEvent)).Where("agent_id = ?", agent.ID).Count(&pending).Error; err != nil {
					return err
				}
				if pending != int64(len(eventsBefore))+1 {
					return fmt.Errorf("the change recorded %d events, want 1", pending-int64(len(eventsBefore)))
				}
				return rollback
			})
			require.ErrorIs(t, err, rollback)
			require.Equal(t, before.Status(), storedAgent(t, c, agent.ID).Status())
			require.Equal(t, eventsBefore, agentLifecycleEvents(t, c, agent.ID))

			// A change whose event cannot be recorded is not made.
			require.NoError(t, c.db.WithContext(ctx).Migrator().DropTable(&types.AgentLifecycleEvent{}))
			err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return tt.change(tx, agent.ID)
			})
			require.ErrorContains(t, err, "failed to record the")
			require.Equal(t, before.Status(), storedAgent(t, c, agent.ID).Status())
		})
	}
}

func TestDeliverAgentLifecycleEventsCreatesControllerObjects(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	agent := createTestAgent(t, c, "builder")

	_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
	require.NoError(t, err)
	_, err = c.EnableAgent(ctx, agent.ID)
	require.NoError(t, err)
	require.NoError(t, c.DeleteAgent(ctx, agent.ID))
	require.NoError(t, c.deliverAgentLifecycleEvents(ctx))

	events := agentLifecycleEvents(t, c, agent.ID)
	require.Len(t, events, 3)
	for _, event := range events {
		require.NotNil(t, event.DeliveredAt, "event %d", event.ID)
		require.Empty(t, event.LastError, "event %d", event.ID)
	}

	// The disable and the enable.
	for i := range 2 {
		var change v1.AgentIdentityChange
		require.NoError(t, c.storageClient.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      agentLifecycleObjectName(system.AgentIdentityChangePrefix, events[i].ID),
		}, &change))
		require.Equal(t, v1.AgentIdentityChangeSpec{
			AgentID: agent.ID,
		}, change.Spec)
	}
	var agentDelete v1.AgentIdentityDelete
	require.NoError(t, c.storageClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      agentLifecycleObjectName(system.AgentIdentityDeletePrefix, events[2].ID),
	}, &agentDelete))
	require.Equal(t, agent.ID, agentDelete.Spec.AgentID)

	// The user outbox is separate: no user event was recorded or delivered.
	var userEvents int64
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).Count(&userEvents).Error)
	require.Zero(t, userEvents)

	// A repeated delivery, from another replica or after a crash before the events were marked delivered, creates
	// nothing new.
	for _, event := range events {
		require.NoError(t, c.deliverAgentLifecycleEvent(ctx, event))
	}
	var changes v1.AgentIdentityChangeList
	require.NoError(t, c.storageClient.List(ctx, &changes))
	require.Len(t, changes.Items, 2)
	var deletes v1.AgentIdentityDeleteList
	require.NoError(t, c.storageClient.List(ctx, &deletes))
	require.Len(t, deletes.Items, 1)
}

func TestAgentLifecycleEventsOfOneAgentAndTypeAreDeliveredOnce(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	db := c.db.WithContext(ctx)

	// Repeated events of one type for agent 1, and one event for agent 2.
	for _, event := range []struct {
		agentID   uint
		eventType types.AgentLifecycleEventType
	}{
		{
			agentID:   1,
			eventType: types.AgentLifecycleEventDisabled,
		},
		{
			agentID:   2,
			eventType: types.AgentLifecycleEventDisabled,
		},
		{
			agentID:   1,
			eventType: types.AgentLifecycleEventDisabled,
		},
		{
			agentID:   1,
			eventType: types.AgentLifecycleEventDeleted,
		},
		{
			agentID:   1,
			eventType: types.AgentLifecycleEventDisabled,
		},
	} {
		require.NoError(t, recordAgentLifecycleEvent(db, event.agentID, event.eventType))
	}
	require.NoError(t, c.deliverAgentLifecycleEvents(ctx))

	var changes v1.AgentIdentityChangeList
	require.NoError(t, c.storageClient.List(ctx, &changes))
	perAgent := map[uint][]string{}
	for _, change := range changes.Items {
		perAgent[change.Spec.AgentID] = append(perAgent[change.Spec.AgentID], change.Name)
	}
	first := agentLifecycleEvents(t, c, 1)[0]
	// The delivery is named after the agent's first event, so a repeated delivery finds it.
	require.Equal(t, []string{agentLifecycleObjectName(system.AgentIdentityChangePrefix, first.ID)}, perAgent[1])
	require.Len(t, perAgent[2], 1)

	var deletes v1.AgentIdentityDeleteList
	require.NoError(t, c.storageClient.List(ctx, &deletes))
	require.Len(t, deletes.Items, 1)

	for _, agentID := range []uint{1, 2} {
		for _, event := range agentLifecycleEvents(t, c, agentID) {
			require.NotNil(t, event.DeliveredAt, "event %d of agent %d", event.ID, agentID)
		}
	}
}

// testReplicasDeliverEachAgentEventOnce runs two replicas' deliveries of the agent outbox of c's database at once,
// and checks that each event is delivered, and its object created, exactly once.
func testReplicasDeliverEachAgentEventOnce(t *testing.T, c *Client) {
	t.Helper()
	ctx := t.Context()

	// More than two batches of events for distinct agents, so that nothing coalesces and both replicas find
	// pending events to claim.
	const agents = 2*lifecycleDeliveryBatchSize + 50
	for agentID := uint(1); agentID <= agents; agentID++ {
		require.NoError(t, recordAgentLifecycleEvent(c.db.WithContext(ctx), agentID, types.AgentLifecycleEventDisabled))
	}

	storage := &countingCreates{
		Client:  c.storageClient,
		creates: map[string]int{},
	}
	replicas := []*Client{
		{
			db:            c.db,
			storageClient: storage,
		},
		{
			db:            c.db,
			storageClient: storage,
		},
	}

	var (
		start = make(chan struct{})
		wg    sync.WaitGroup
		errs  = make([]error, len(replicas))
	)
	for i, replica := range replicas {
		wg.Go(func() {
			<-start
			errs[i] = replica.deliverAgentLifecycleEvents(ctx)
		})
	}
	close(start)
	wg.Wait()
	require.NoError(t, errors.Join(errs...))

	var pending int64
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.AgentLifecycleEvent)).Where("delivered_at IS NULL").Count(&pending).Error)
	require.Zero(t, pending)

	var events []types.AgentLifecycleEvent
	require.NoError(t, c.db.WithContext(ctx).Order("id").Find(&events).Error)
	require.Len(t, events, agents)
	for _, event := range events {
		name := agentLifecycleObjectName(system.AgentIdentityChangePrefix, event.ID)
		require.Equal(t, 1, storage.creates[name], "creations of %s", name)
		require.Zero(t, event.Attempts, "event %d", event.ID)
	}
	require.Len(t, storage.creates, agents)
}

func TestReplicasDeliverEachAgentLifecycleEventOnce(t *testing.T) {
	testReplicasDeliverEachAgentEventOnce(t, newLifecycleTestClient(t))
}

func TestReplicasDeliverEachAgentLifecycleEventOnceOnPostgres(t *testing.T) {
	testReplicasDeliverEachAgentEventOnce(t, newPostgresLifecycleTestClient(t))
}

func TestLifecycleDeliveryLoopDeliversUserAndAgentEvents(t *testing.T) {
	c := newLifecycleTestClient(t)
	c.kickLifecycleDelivery = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		defer close(done)
		c.runLifecycleEventDelivery(ctx)
	}()

	user := createLifecycleTestUser(t, c, "rita", lifecycleTestProvider)
	require.NoError(t, recordUserReconcileEvent(c.db.WithContext(ctx), user.ID, false))
	agent := createTestAgent(t, c, "builder")
	_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
	require.NoError(t, err)

	// Long before the next tick, the kick of the agent change delivers both outboxes.
	require.Eventually(t, func() bool {
		agentEvents := agentLifecycleEvents(t, c, agent.ID)
		userEvents := lifecycleEvents(t, c, user.ID)
		return len(agentEvents) == 1 && agentEvents[0].DeliveredAt != nil &&
			len(userEvents) == 1 && userEvents[0].DeliveredAt != nil
	}, lifecycleDeliveryInterval/2, 20*time.Millisecond)
}
