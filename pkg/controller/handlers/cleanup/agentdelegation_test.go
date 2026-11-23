package cleanup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// recordingDelegationCleanup records the scopes it is asked to clean up, and fails with err if it is set.
type recordingDelegationCleanup struct {
	scopes []agentdelegation.Scope
	err    error
}

// delegationCleanupFixture is two agents, each of which two users approved, with an approval request of each agent
// that no one approved yet.
type delegationCleanupFixture struct {
	gateway        *gclient.Client
	db             *gorm.DB
	agents         [2]uint
	users          [2]uint
	delegations    map[[2]uint]string
	pendingByAgent map[uint]string
}

func (r *recordingDelegationCleanup) CleanupDelegations(_ context.Context, scope agentdelegation.Scope) error {
	r.scopes = append(r.scopes, scope)
	return r.err
}

func newDelegationCleanupFixture(t *testing.T) *delegationCleanupFixture {
	t.Helper()
	ctx := t.Context()
	gateway, db := newAgentCleanupGateway(t)
	f := &delegationCleanupFixture{
		gateway:        gateway,
		db:             db,
		delegations:    map[[2]uint]string{},
		pendingByAgent: map[uint]string{},
	}

	secret := strings.Repeat("s", 43)
	digest := sha256.Sum256([]byte(secret))
	secretHash := base64.RawURLEncoding.EncodeToString(digest[:])
	for i, name := range []string{"alice", "bob"} {
		u := &gatewaytypes.User{Username: name, HashedUsername: hash.String(name), Role: types.RoleBasic}
		require.NoError(t, db.Create(u).Error)
		f.users[i] = u.ID
	}
	for i, name := range []string{"deploy bot", "triage bot"} {
		agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{
			DisplayName:        name,
			Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}}},
			AllowedReturnHosts: []string{"agent.example.com"},
		})
		require.NoError(t, err)
		f.agents[i] = agent.ID
		request := func() string {
			created, err := gateway.CreateAgentDelegationRequest(ctx, gclient.AgentDelegationRequestInput{
				AgentID: agent.ID, APIKeyID: 1, ReturnURL: "https://agent.example.com/callback", SecretHash: secretHash,
			})
			require.NoError(t, err)
			return created.ID
		}
		for _, userID := range f.users {
			approval, _, err := gateway.ApproveAgentDelegationRequest(ctx, request(), subjects.Caller{Principal: principal.User(userID)})
			require.NoError(t, err)
			f.delegations[[2]uint{agent.ID, userID}] = approval.Delegation.ID
		}
		f.pendingByAgent[agent.ID] = request()
	}
	return f
}

// remaining returns the delegations that still exist, by agent and user.
func (f *delegationCleanupFixture) remaining(t *testing.T) map[[2]uint]bool {
	t.Helper()
	remaining := map[[2]uint]bool{}
	for key, id := range f.delegations {
		if _, err := f.gateway.GetAgentDelegation(t.Context(), id); err == nil {
			remaining[key] = true
		} else {
			require.ErrorIs(t, err, gclient.ErrAgentDelegationNotFound)
		}
	}
	return remaining
}

func (f *delegationCleanupFixture) requests(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, f.db.Model(&gatewaytypes.AgentDelegationRequest{}).Where(query, args...).Count(&count).Error)
	return count
}

// TestAgentCleanupDeletesDelegations checks that deleting an agent removes what users set up through it, then its
// delegations and approval requests, and only its, and that a failure to remove the setup leaves the delegations for
// the retry to find.
func TestAgentCleanupDeletesDelegations(t *testing.T) {
	f := newDelegationCleanupFixture(t)
	deleted, kept := f.agents[0], f.agents[1]
	require.NoError(t, f.gateway.DeleteAgent(t.Context(), deleted))
	event := &v1.AgentIdentityDelete{ObjectMeta: objectMeta(system.AgentIdentityDeletePrefix + "agent-lifecycle-1"), Spec: v1.AgentIdentityDeleteSpec{AgentID: deleted}}
	storage := withOwnerIndexes(fake.NewClientBuilder().WithScheme(scheme.Scheme)).WithObjects(event).Build()
	request := router.Request{Ctx: t.Context(), Client: storage, Object: event, Namespace: event.Namespace, Name: event.Name}

	hook := &recordingDelegationCleanup{err: errors.New("storage is down")}
	require.ErrorIs(t, NewAgentCleanup(f.gateway, hook).DeletedAgent(request, nil), hook.err)
	require.Len(t, f.remaining(t), 4, "nothing is deleted before the setup is removed")
	require.True(t, storedObjectExists(t, storage, &v1.AgentIdentityDelete{ObjectMeta: objectMeta(event.Name)}))

	hook.err = nil
	require.NoError(t, NewAgentCleanup(f.gateway, hook).DeletedAgent(request, nil))
	require.Equal(t, []agentdelegation.Scope{{AgentID: deleted}, {AgentID: deleted}}, hook.scopes)
	require.Equal(t, map[[2]uint]bool{{kept, f.users[0]}: true, {kept, f.users[1]}: true}, f.remaining(t))
	require.Zero(t, f.requests(t, "agent_id = ?", deleted))
	require.NotZero(t, f.requests(t, "id = ?", f.pendingByAgent[kept]))
	require.False(t, storedObjectExists(t, storage, &v1.AgentIdentityDelete{ObjectMeta: objectMeta(event.Name)}))
}

// TestUserCleanupDeletesDelegations checks that deleting a user removes what they set up through agents, then their
// delegations and the requests they approved, and only theirs.
func TestUserCleanupDeletesDelegations(t *testing.T) {
	f := newDelegationCleanupFixture(t)
	deleted, kept := f.users[0], f.users[1]
	// Deleted as DeleteUser deletes a user, without the storage it notifies.
	require.NoError(t, f.db.Model(&gatewaytypes.User{}).Where("id = ?", deleted).Update("deleted_at", time.Now()).Error)

	userDelete := &v1.UserDelete{ObjectMeta: objectMeta("ud1alice"), Spec: v1.UserDeleteSpec{UserID: deleted}}
	storage := withOwnerIndexes(fake.NewClientBuilder().WithScheme(scheme.Scheme)).WithObjects(userDelete).Build()
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(any) ([]string, error) { return nil, nil },
	})
	request := router.Request{Ctx: t.Context(), Client: storage, Object: userDelete, Namespace: system.DefaultNamespace, Name: userDelete.Name}
	hook := &recordingDelegationCleanup{err: errors.New("storage is down")}
	cleanup := NewUserCleanup(f.gateway, accesscontrolrule.NewAccessControlRuleHelper(indexer, storage), hook)

	require.ErrorIs(t, cleanup.Cleanup(request, nil), hook.err)
	require.Len(t, f.remaining(t), 4, "nothing is deleted before the setup is removed")

	hook.err = nil
	require.NoError(t, cleanup.Cleanup(request, nil))
	require.Equal(t, []agentdelegation.Scope{{UserID: deleted}, {UserID: deleted}}, hook.scopes)
	require.Equal(t, map[[2]uint]bool{{f.agents[0], kept}: true, {f.agents[1], kept}: true}, f.remaining(t))
	require.Zero(t, f.requests(t, "approved_user_id = ?", deleted))
	require.NotZero(t, f.requests(t, "approved_user_id = ?", kept))
	for _, id := range f.pendingByAgent {
		require.NotZero(t, f.requests(t, "id = ?", id), "a request no one approved is no user's")
	}
	require.False(t, storedObjectExists(t, storage, &v1.UserDelete{ObjectMeta: objectMeta(userDelete.Name)}))
}
