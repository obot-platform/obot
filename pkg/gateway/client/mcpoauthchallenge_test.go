package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMCPOAuthChallengeClaimBindingsAndLifecycle(t *testing.T) {
	c := newTestClient(t)
	batch := &types.MCPOAuthChallenge{
		UserID:    "user",
		Namespace: "namespace",
		MCPID:     "vmcp",
		ClientID:  "client",
		Resource:  "https://obot/mcp-connect/vmcp",
		Requirements: []v1.UpstreamOAuthRequirement{{
			Component:  "component",
			ConnectID:  "connection",
			Scope:      "read write",
			StatusCode: 403,
		}},
	}
	require.NoError(t, c.CreateMCPOAuthChallenge(t.Context(), batch))

	for _, binding := range []struct {
		name      string
		user      string
		namespace string
		mcp       string
		client    string
		resource  string
	}{
		{
			name:      "user",
			user:      "other",
			namespace: batch.Namespace,
			mcp:       batch.MCPID,
			client:    batch.ClientID,
			resource:  batch.Resource,
		},
		{
			name:      "namespace",
			user:      batch.UserID,
			namespace: "other",
			mcp:       batch.MCPID,
			client:    batch.ClientID,
			resource:  batch.Resource,
		},
		{
			name:      "connection",
			user:      batch.UserID,
			namespace: batch.Namespace,
			mcp:       "other",
			client:    batch.ClientID,
			resource:  batch.Resource,
		},
		{
			name:      "client",
			user:      batch.UserID,
			namespace: batch.Namespace,
			mcp:       batch.MCPID,
			client:    "other",
			resource:  batch.Resource,
		},
		{
			name:      "resource",
			user:      batch.UserID,
			namespace: batch.Namespace,
			mcp:       batch.MCPID,
			client:    batch.ClientID,
			resource:  "other",
		},
	} {
		t.Run(binding.name, func(t *testing.T) {
			_, err := c.ClaimMCPOAuthChallenge(t.Context(), batch.ID, binding.user, binding.namespace, binding.mcp, binding.client, binding.resource, "attacker")

			require.Error(t, err)
		})
	}

	claim := func(c *Client, request string) (*types.MCPOAuthChallenge, error) {
		return c.ClaimMCPOAuthChallenge(t.Context(), batch.ID, batch.UserID, batch.Namespace, batch.MCPID, batch.ClientID, batch.Resource, request)
	}
	replica := &Client{db: c.db}
	claimed, err := claim(replica, "request")
	require.NoError(t, err)
	require.Equal(t, batch.Requirements, claimed.Requirements)

	// A failed transaction save keeps the source recoverable by the same request.
	_, err = claim(c, "request")
	require.NoError(t, err)

	_, err = claim(c, "other-request")

	require.Error(t, err)
	require.NoError(t, c.DeleteClaimedMCPOAuthChallenge(t.Context(), batch.ID, "other-request"))

	_, err = claim(c, "request")
	require.NoError(t, err)
	require.NoError(t, c.DeleteClaimedMCPOAuthChallenge(t.Context(), batch.ID, "request"))

	_, err = claim(c, "request")

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	_, err = c.ClaimMCPOAuthChallenge(t.Context(), "unknown", "user", "namespace", "vmcp", "client", batch.Resource, "request")

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	expired := *batch
	expired.ID = ""
	require.NoError(t, c.CreateMCPOAuthChallenge(t.Context(), &expired))
	require.NoError(t, c.db.WithContext(t.Context()).Model(&expired).Update("expires_at", time.Now().Add(-time.Minute)).Error)

	_, err = c.ClaimMCPOAuthChallenge(t.Context(), expired.ID, expired.UserID, expired.Namespace, expired.MCPID, expired.ClientID, expired.Resource, "request")

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, c.CleanupExpiredMCPOAuthPendingStates(t.Context(), time.Hour))

	var count int64
	require.NoError(t, c.db.WithContext(t.Context()).Model(&types.MCPOAuthChallenge{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMCPOAuthChallengeConcurrentClaims(t *testing.T) {
	c := newTestClient(t)
	batch := &types.MCPOAuthChallenge{
		UserID: "user",
		MCPID:  "vmcp",
	}
	require.NoError(t, c.CreateMCPOAuthChallenge(t.Context(), batch))

	var wg sync.WaitGroup
	outcomes := make(chan string, 10)

	for i := range 10 {
		wg.Go(func() {
			request := fmt.Sprint(i)
			_, err := c.ClaimMCPOAuthChallenge(t.Context(), batch.ID, "user", "", "vmcp", "", "", request)
			if err == nil {
				outcomes <- request
			}
		})
	}

	wg.Wait()
	close(outcomes)

	require.Len(t, outcomes, 1)
}

func TestMCPOAuthAttemptBudgetExpiresAndIsShared(t *testing.T) {
	c := newTestClient(t)
	replica := &Client{db: c.db}

	for range 3 {
		require.NoError(t, replica.ReserveMCPOAuthAttempt(t.Context(), "stable-component"))
	}

	require.ErrorContains(t, c.ReserveMCPOAuthAttempt(t.Context(), "stable-component"), "limit reached")
	require.NoError(t, c.ReserveMCPOAuthAttempt(t.Context(), "other-component"))
	require.NoError(t, c.db.WithContext(t.Context()).Model(&types.MCPOAuthAttemptBudget{}).Where("id = ?", "stable-component").Update("expires_at", time.Now().Add(-time.Second)).Error)
	require.NoError(t, c.ReserveMCPOAuthAttempt(t.Context(), "stable-component"))
}

func TestMCPOAuthChallengePersistenceFailure(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := c.CreateMCPOAuthChallenge(ctx, &types.MCPOAuthChallenge{UserID: "user"})

	require.True(t, errors.Is(err, context.Canceled))
}
