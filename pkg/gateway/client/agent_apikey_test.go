package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// newAgentAPIKeyTestClient returns a client that caches validated API keys, with objects in its storage.
func newAgentAPIKeyTestClient(t *testing.T, objects ...kclient.Object) *Client {
	t.Helper()

	c := newLifecycleTestClient(t, objects...)
	c.apiKeyCache = make(map[[32]byte]apiKeyValidationCacheEntry)
	c.apiKeyCacheTTL = time.Minute
	return c
}

// newPostgresAgentAPIKeyTestClient is newAgentAPIKeyTestClient on PostgreSQL. The test is skipped without it.
func newPostgresAgentAPIKeyTestClient(t *testing.T) *Client {
	t.Helper()

	c := newPostgresLifecycleTestClient(t)
	c.apiKeyCache = make(map[[32]byte]apiKeyValidationCacheEntry)
	c.apiKeyCacheTTL = time.Minute
	return c
}

func testVMCP(name string) *v1.VMCP {
	return &v1.VMCP{
		Namespace: system.DefaultNamespace,
		Name:      name,
	}
}

// cachedValidation reports whether c has a cached validation of key.
func cachedValidation(c *Client, key string) bool {
	c.apiKeyCacheLock.RLock()
	defer c.apiKeyCacheLock.RUnlock()
	_, ok := c.apiKeyCache[apiKeyCacheFingerprint(key)]
	return ok
}

func createTestAgentAPIKey(t *testing.T, c *Client, agentID uint, name string) *types.APIKeyCreateResponse {
	t.Helper()

	created, err := c.CreateAgentAPIKey(t.Context(), agentID, types.AgentAPIKeyCreateRequest{
		Name:         name,
		MCPServerIDs: []string{"*"},
	})
	if err != nil {
		t.Fatalf("failed to create API key for agent %d: %v", agentID, err)
	}
	return created
}

func TestCreateAgentAPIKeyStoresTheRequestedScopes(t *testing.T) {
	c := newAgentAPIKeyTestClient(t, testVMCP("vmcp1-build"))
	ctx := t.Context()
	agent := createTestAgent(t, c, "builder")

	for _, tt := range []struct {
		name       string
		req        types.AgentAPIKeyCreateRequest
		wantScopes types.APIKeyScopes
	}{
		{
			// As with a user's key, an omitted scope is not granted.
			name: "LLM proxy only",
			req: types.AgentAPIKeyCreateRequest{
				Name:              "llm-only",
				CanAccessLLMProxy: true,
			},
			wantScopes: types.APIKeyScopes{
				CanAccessLLMProxy: true,
			},
		},
		{
			name: "one vMCP",
			req: types.AgentAPIKeyCreateRequest{
				Name:         "mcp-only",
				MCPServerIDs: []string{"vmcp1-build"},
			},
			wantScopes: types.APIKeyScopes{
				MCPServerIDs: []string{"vmcp1-build"},
			},
		},
		{
			name: "everything",
			req: types.AgentAPIKeyCreateRequest{
				Name:              "everything",
				MCPServerIDs:      []string{"*", "vmcp1-build"},
				CanAccessLLMProxy: true,
				CanAccessSkills:   true,
			},
			wantScopes: types.APIKeyScopes{
				CanAccessLLMProxy: true,
				CanAccessSkills:   true,
				MCPServerIDs:      []string{"*", "vmcp1-build"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			created, err := c.CreateAgentAPIKey(ctx, agent.ID, tt.req)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(created.Key, fmt.Sprintf("oak1-%d-%d-", agent.ID, created.ID)), "key %q has the wrong prefix", created.Key)
			require.Equal(t, principal.Agent(agent.ID), created.Owner())
			require.Nil(t, created.UserID)
			// Each read gives back the scopes that were requested.
			require.Equal(t, tt.wantScopes, created.APIKeyScopes)

			got, err := c.GetAgentAPIKey(ctx, agent.ID, created.ID)
			require.NoError(t, err)
			require.Equal(t, tt.wantScopes, got.APIKeyScopes)

			listed, err := c.ListAgentAPIKeys(ctx, agent.ID, APIKeyListOptions{})
			require.NoError(t, err)
			var found bool
			for _, key := range listed {
				if key.ID == created.ID {
					found = true
					require.Equal(t, tt.wantScopes, key.APIKeyScopes)
				}
			}
			require.True(t, found, "listed keys %+v lack key %d", listed, created.ID)

			// Validation reads the key from the database, and then from the cache.
			for range 2 {
				validated, err := c.ValidateAPIKey(ctx, created.Key)
				require.NoError(t, err)
				require.Equal(t, tt.wantScopes, validated.APIKeyScopes)
				require.Equal(t, principal.Agent(agent.ID), validated.Owner())
				require.Equal(t, apitypes.UserStatusActive, validated.OwnerStatus)
				require.NotNil(t, validated.LastUsedAt)
			}
			require.True(t, cachedValidation(c, created.Key))
		})
	}
}

func TestCreateAgentAPIKeyRefusesKeysItMustNotCreate(t *testing.T) {
	c := newAgentAPIKeyTestClient(t, testVMCP("vmcp1-build"))
	ctx := t.Context()
	active := createTestAgent(t, c, "active")
	deleted := createTestAgent(t, c, "deleted")
	require.NoError(t, c.DeleteAgent(ctx, deleted.ID))

	for _, tt := range []struct {
		name    string
		agentID uint
		req     types.AgentAPIKeyCreateRequest
		wantErr error
	}{
		{
			name:    "unknown vMCP",
			agentID: active.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "unknown",
				MCPServerIDs: []string{"vmcp1-build", "vmcp1-missing"},
			},
			wantErr: ErrInvalidAgentAPIKeyRequest,
		},
		{
			name:    "MCP server that is not a vMCP",
			agentID: active.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "server",
				MCPServerIDs: []string{"ms1-build"},
			},
			wantErr: ErrInvalidAgentAPIKeyRequest,
		},
		{
			name:    "vMCP instance",
			agentID: active.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "instance",
				MCPServerIDs: []string{"vmcpi1-build"},
			},
			wantErr: ErrInvalidAgentAPIKeyRequest,
		},
		{
			name:    "empty ID",
			agentID: active.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "empty",
				MCPServerIDs: []string{""},
			},
			wantErr: ErrInvalidAgentAPIKeyRequest,
		},
		{
			name:    "no scope at all",
			agentID: active.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name: "nothing",
			},
			wantErr: ErrInvalidAgentAPIKeyRequest,
		},
		{
			name:    "missing agent",
			agentID: deleted.ID + 100,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "missing",
				MCPServerIDs: []string{"*"},
			},
			wantErr: gorm.ErrRecordNotFound,
		},
		{
			name:    "deleted agent",
			agentID: deleted.ID,
			req: types.AgentAPIKeyCreateRequest{
				Name:         "deleted",
				MCPServerIDs: []string{"*"},
			},
			wantErr: gorm.ErrRecordNotFound,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.CreateAgentAPIKey(ctx, tt.agentID, tt.req)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	// None of the refused keys was stored.
	var count int64
	require.NoError(t, c.db.WithContext(ctx).Model(&types.APIKey{}).Count(&count).Error)
	require.Zero(t, count)

	// A wildcard needs no vMCP to exist, and a disabled agent gets a key, so that it is ready when it is enabled.
	_, err := c.CreateAgentAPIKey(ctx, active.ID, types.AgentAPIKeyCreateRequest{
		Name:         "wildcard",
		MCPServerIDs: []string{"*"},
	})
	require.NoError(t, err)
	_, err = c.DisableAgent(ctx, active.ID, "rotating keys")
	require.NoError(t, err)
	created, err := c.CreateAgentAPIKey(ctx, active.ID, types.AgentAPIKeyCreateRequest{
		Name:         "rotated",
		MCPServerIDs: []string{"*"},
	})
	require.NoError(t, err)
	validated, err := c.ValidateAPIKey(ctx, created.Key)
	require.NoError(t, err)
	require.Equal(t, apitypes.UserStatusDisabled, validated.OwnerStatus)
}

func TestAgentAndUserAPIKeysWithTheSameIDsAreNeverConfused(t *testing.T) {
	testAgentAndUserAPIKeysWithTheSameIDsAreNeverConfused(t, newAgentAPIKeyTestClient(t))
}

func TestAgentAndUserAPIKeysWithTheSameIDsAreNeverConfusedOnPostgres(t *testing.T) {
	testAgentAndUserAPIKeysWithTheSameIDsAreNeverConfused(t, newPostgresAgentAPIKeyTestClient(t))
}

func testAgentAndUserAPIKeysWithTheSameIDsAreNeverConfused(t *testing.T, c *Client) {
	t.Helper()

	ctx := t.Context()
	const ownerID = 5
	require.NoError(t, c.db.WithContext(ctx).Create(&types.User{ID: ownerID, Username: "five", HashedUsername: "five"}).Error)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.Agent{ID: ownerID, DisplayName: "five"}).Error)

	userKey, err := c.CreateAPIKey(ctx, ownerID, "user", "", nil, types.APIKeyScopes{
		CanAccessAPI: true,
		MCPServerIDs: []string{"*"},
	})
	require.NoError(t, err)
	agentKey := createTestAgentAPIKey(t, c, ownerID, "agent")
	_, _, _, userSecret, err := ParseAPIKey(userKey.Key)
	require.NoError(t, err)
	_, _, _, agentSecret, err := ParseAPIKey(agentKey.Key)
	require.NoError(t, err)

	// Each key validates as its own owner's, and is then cached.
	for key, want := range map[string]principal.Principal{
		userKey.Key:  principal.User(ownerID),
		agentKey.Key: principal.Agent(ownerID),
	} {
		got, err := c.ValidateAPIKey(ctx, key)
		require.NoError(t, err)
		require.Equal(t, want, got.Owner())
		require.True(t, cachedValidation(c, key))
	}

	// A key presented with the other owner's prefix is no key at all, even while the real keys are cached.
	for _, forged := range []string{
		fmt.Sprintf("oak1-%d-%d-%s", ownerID, userKey.ID, userSecret),
		fmt.Sprintf("ok1-%d-%d-%s", ownerID, agentKey.ID, agentSecret),
	} {
		for range 2 {
			_, err := c.ValidateAPIKey(ctx, forged)
			require.ErrorIs(t, err, ErrInvalidAPIKey, "validated %q", forged)
		}
		require.False(t, cachedValidation(c, forged))
	}

	// Reads by one owner never find the other's key.
	_, err = c.GetAPIKey(ctx, ownerID, agentKey.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = c.GetAgentAPIKey(ctx, ownerID, userKey.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	userKeys, err := c.ListAPIKeys(ctx, ownerID)
	require.NoError(t, err)
	require.Len(t, userKeys, 1)
	require.Equal(t, userKey.ID, userKeys[0].ID)
	agentKeys, err := c.ListAgentAPIKeys(ctx, ownerID, APIKeyListOptions{})
	require.NoError(t, err)
	require.Len(t, agentKeys, 1)
	require.Equal(t, agentKey.ID, agentKeys[0].ID)

	// Neither can revoke the other's key.
	require.NoError(t, c.RevokeAPIKey(ctx, ownerID, agentKey.ID))
	require.NoError(t, c.RevokeAgentAPIKey(ctx, ownerID, userKey.ID))
	for _, key := range []string{userKey.Key, agentKey.Key} {
		_, err := c.ValidateAPIKey(ctx, key)
		require.NoError(t, err)
	}

	// The admin list tells the owners apart, in Go and in JSON.
	all, err := c.ListAllAPIKeys(ctx, APIKeyListOptions{})
	require.NoError(t, err)
	owners := map[uint]principal.Principal{}
	for _, key := range all {
		owners[key.ID] = key.Owner()
	}
	require.Equal(t, map[uint]principal.Principal{
		userKey.ID:  principal.User(ownerID),
		agentKey.ID: principal.Agent(ownerID),
	}, owners)
	for _, key := range all {
		encoded, err := json.Marshal(key)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(encoded, &fields))
		if key.ID == userKey.ID {
			require.EqualValues(t, ownerID, fields["userId"])
			require.NotContains(t, fields, "agentId")
		} else {
			require.EqualValues(t, ownerID, fields["agentId"])
			require.NotContains(t, fields, "userId")
		}
	}
}

func TestValidateAgentAPIKeyReportsTheAgentsCurrentStatus(t *testing.T) {
	c := newAgentAPIKeyTestClient(t)
	ctx := t.Context()
	agent := createTestAgent(t, c, "status")
	created := createTestAgentAPIKey(t, c, agent.ID, "status")

	assertOwnerStatus := func(want apitypes.UserStatus) {
		t.Helper()
		key, err := c.ValidateAPIKey(ctx, created.Key)
		require.NoError(t, err)
		require.Equal(t, want, key.OwnerStatus)
		require.Equal(t, principal.Agent(agent.ID), key.Owner())
	}
	// assertOnCacheHitAndMiss checks the status when the key is cached, and again once it is not.
	assertOnCacheHitAndMiss := func(want apitypes.UserStatus) {
		t.Helper()
		require.True(t, cachedValidation(c, created.Key))
		assertOwnerStatus(want)
		c.invalidateValidatedAPIKeysByID(created.ID)
		assertOwnerStatus(want)
	}

	// The first validation misses the cache, and the next hits it.
	assertOwnerStatus(apitypes.UserStatusActive)
	assertOwnerStatus(apitypes.UserStatusActive)

	_, err := c.DisableAgent(ctx, agent.ID, "suspicious traffic")
	require.NoError(t, err)
	assertOnCacheHitAndMiss(apitypes.UserStatusDisabled)

	_, err = c.EnableAgent(ctx, agent.ID)
	require.NoError(t, err)
	assertOnCacheHitAndMiss(apitypes.UserStatusActive)

	// DeleteAgent revokes the agent's keys. Deleting the row alone shows that a deleted agent is read as deleted, not
	// as missing, so its key would be refused even unrevoked.
	require.NoError(t, c.db.WithContext(ctx).Delete(&types.Agent{}, agent.ID).Error)
	assertOnCacheHitAndMiss(apitypes.UserStatusDeleted)

	// An agent with no row at all is deleted too.
	require.NoError(t, c.db.WithContext(ctx).Unscoped().Delete(&types.Agent{}, agent.ID).Error)
	assertOnCacheHitAndMiss(apitypes.UserStatusDeleted)
}

func TestAgentAPIKeyRevocationAndExpiryTakeEffectOnTheNextValidation(t *testing.T) {
	c := newAgentAPIKeyTestClient(t)
	ctx := t.Context()
	// Another replica caches validations of its own.
	replica := &Client{
		db:             c.db,
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}
	agent := createTestAgent(t, c, "rotating")
	revoked := createTestAgentAPIKey(t, c, agent.ID, "revoked")
	expiring, err := c.CreateAgentAPIKey(ctx, agent.ID, types.AgentAPIKeyCreateRequest{
		Name:         "expiring",
		ExpiresAt:    new(time.Now().Add(time.Hour)),
		MCPServerIDs: []string{"*"},
	})
	require.NoError(t, err)
	for _, client := range []*Client{c, replica} {
		for _, key := range []string{revoked.Key, expiring.Key} {
			_, err := client.ValidateAPIKey(ctx, key)
			require.NoError(t, err)
		}
	}

	require.NoError(t, c.RevokeAgentAPIKey(ctx, agent.ID, revoked.ID))
	require.NoError(t, c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", expiring.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)

	// The replica still caches both keys, and refuses them anyway, as does the client that revoked one.
	require.True(t, cachedValidation(replica, revoked.Key))
	require.True(t, cachedValidation(replica, expiring.Key))
	for _, client := range []*Client{replica, c} {
		for _, key := range []string{revoked.Key, expiring.Key} {
			_, err := client.ValidateAPIKey(ctx, key)
			require.ErrorIs(t, err, ErrInvalidAPIKey)
			require.False(t, cachedValidation(client, key))
			_, err = client.ValidateAPIKey(ctx, key)
			require.ErrorIs(t, err, ErrInvalidAPIKey)
		}
	}

	// A revoked key keeps its metadata, and revoking it again keeps its revocation time.
	retained, err := c.GetAgentAPIKey(ctx, agent.ID, revoked.ID)
	require.NoError(t, err)
	require.NotNil(t, retained.RevokedAt)
	require.Equal(t, "revoked", retained.Name)
	require.NoError(t, c.RevokeAgentAPIKey(ctx, agent.ID, revoked.ID))
	again, err := c.GetAgentAPIKey(ctx, agent.ID, revoked.ID)
	require.NoError(t, err)
	require.True(t, again.RevokedAt.Equal(*retained.RevokedAt))

	// Only revoked keys are hidden from the operational list.
	active, err := c.ListAgentAPIKeys(ctx, agent.ID, APIKeyListOptions{})
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, expiring.ID, active[0].ID)
	withRevoked, err := c.ListAgentAPIKeys(ctx, agent.ID, APIKeyListOptions{ShowRevoked: true})
	require.NoError(t, err)
	require.Len(t, withRevoked, 2)
}

func TestDeleteAgentRevokesItsAPIKeys(t *testing.T) {
	c := newAgentAPIKeyTestClient(t)
	ctx := t.Context()
	agent := createTestAgent(t, c, "retiring")
	bystander := createTestAgent(t, c, "bystander")
	user := createLifecycleTestUser(t, c, "lena", lifecycleTestProvider)

	cached := createTestAgentAPIKey(t, c, agent.ID, "cached")
	uncached := createTestAgentAPIKey(t, c, agent.ID, "uncached")
	alreadyRevoked := createTestAgentAPIKey(t, c, agent.ID, "already revoked")
	require.NoError(t, c.RevokeAgentAPIKey(ctx, agent.ID, alreadyRevoked.ID))
	before, err := c.GetAgentAPIKey(ctx, agent.ID, alreadyRevoked.ID)
	require.NoError(t, err)
	bystanderKey := createTestAgentAPIKey(t, c, bystander.ID, "bystander")
	userKey, err := c.CreateAPIKey(ctx, user.ID, "user", "", nil, types.APIKeyScopes{
		CanAccessAPI: true,
	})
	require.NoError(t, err)
	_, err = c.ValidateAPIKey(ctx, cached.Key)
	require.NoError(t, err)

	// A deletion that rolls back revokes nothing.
	rollback := errors.New("rollback")
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := deleteAgentTx(tx, agent.ID); err != nil {
			return err
		}
		var unrevoked int64
		if err := tx.Model(&types.APIKey{}).Where("agent_id = ? AND revoked_at IS NULL", agent.ID).Count(&unrevoked).Error; err != nil {
			return err
		}
		if unrevoked != 0 {
			return fmt.Errorf("the deletion left %d keys unrevoked", unrevoked)
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	for _, key := range []string{cached.Key, uncached.Key} {
		_, err := c.ValidateAPIKey(ctx, key)
		require.NoError(t, err)
	}

	require.NoError(t, c.DeleteAgent(ctx, agent.ID))

	// The agent's keys are refused, cached or not.
	require.True(t, cachedValidation(c, cached.Key))
	for _, key := range []string{cached.Key, uncached.Key, alreadyRevoked.Key} {
		_, err := c.ValidateAPIKey(ctx, key)
		require.ErrorIs(t, err, ErrInvalidAPIKey)
	}
	keys, err := c.ListAgentAPIKeys(ctx, agent.ID, APIKeyListOptions{ShowRevoked: true})
	require.NoError(t, err)
	require.Len(t, keys, 3)
	for _, key := range keys {
		require.NotNil(t, key.RevokedAt, "key %d of the deleted agent is not revoked", key.ID)
		if key.ID == alreadyRevoked.ID {
			require.True(t, key.RevokedAt.Equal(*before.RevokedAt), "the deletion changed when key %d was revoked", key.ID)
		}
	}

	// Other owners' keys are untouched.
	for _, key := range []string{bystanderKey.Key, userKey.Key} {
		got, err := c.ValidateAPIKey(ctx, key)
		require.NoError(t, err)
		require.Nil(t, got.RevokedAt)
	}

	// The deleted agent gets no new key.
	_, err = c.CreateAgentAPIKey(ctx, agent.ID, types.AgentAPIKeyCreateRequest{
		Name:         "late",
		MCPServerIDs: []string{"*"},
	})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestConcurrentAgentAPIKeyCreationAndDeletionOnPostgres checks that a key created while its agent is deleted is
// either refused or revoked by the deletion: no working key outlives the agent.
func TestConcurrentAgentAPIKeyCreationAndDeletionOnPostgres(t *testing.T) {
	c := newPostgresAgentAPIKeyTestClient(t)
	ctx := t.Context()

	for i := range 20 {
		agent := createTestAgent(t, c, fmt.Sprintf("racer-%d", i))
		var (
			wg        sync.WaitGroup
			created   *types.APIKeyCreateResponse
			createErr error
			deleteErr error
		)
		wg.Go(func() {
			created, createErr = c.CreateAgentAPIKey(ctx, agent.ID, types.AgentAPIKeyCreateRequest{
				Name:         "racer",
				MCPServerIDs: []string{"*"},
			})
		})
		wg.Go(func() {
			deleteErr = c.DeleteAgent(ctx, agent.ID)
		})
		wg.Wait()

		require.NoError(t, deleteErr)
		if createErr != nil {
			require.ErrorIs(t, createErr, gorm.ErrRecordNotFound)
			continue
		}
		_, err := c.ValidateAPIKey(ctx, created.Key)
		require.ErrorIs(t, err, ErrInvalidAPIKey, "key %d outlived agent %d", created.ID, agent.ID)
	}
}
