package client

import (
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func TestConsumeMCPOAuthPendingStateOnceAcrossClients(t *testing.T) {
	client := newTestClient(t)
	replica := &Client{db: client.db}
	require.NoError(t, client.CreateMCPOAuthPendingState(t.Context(), "user", "component", "https://provider/mcp", "", "request", "state", "verifier", "resource", &oauth2.Config{ClientID: "registered"}, "challenge"))

	var workers sync.WaitGroup
	consumed := make(chan *types.MCPOAuthPendingState, 12)
	failures := make(chan error, 12)

	for i := range 12 {
		workers.Go(func() {
			reader := client
			if i%2 == 0 {
				reader = replica
			}

			pending, err := reader.ConsumeMCPOAuthPendingState(t.Context(), "state")
			if err != nil {
				failures <- err

				return
			}

			consumed <- pending
		})
	}

	workers.Wait()
	close(consumed)
	close(failures)

	require.Len(t, consumed, 1)
	require.Len(t, failures, 11)

	for err := range failures {
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}

	for pending := range consumed {
		require.Equal(t, "verifier", pending.Verifier)
		require.Equal(t, "challenge", pending.UpstreamChallenge)
	}
}

func TestFindMCPOAuthPendingStateMatchesLiveAttemptBindings(t *testing.T) {
	client := newTestClient(t)
	require.NoError(t, client.CreateMCPOAuthPendingState(t.Context(), "user", "component", "https://provider/mcp", "", "request", "state", "verifier", "resource", &oauth2.Config{ClientID: "registered"}, "challenge"))

	for _, test := range []struct {
		name      string
		user      string
		component string
		url       string
		request   string
		challenge string
	}{
		{
			name:      "other user",
			user:      "other",
			component: "component",
			url:       "https://provider/mcp",
			request:   "request",
			challenge: "challenge",
		},
		{
			name:      "other connection",
			user:      "user",
			component: "other",
			url:       "https://provider/mcp",
			request:   "request",
			challenge: "challenge",
		},
		{
			name:      "changed target",
			user:      "user",
			component: "component",
			url:       "https://other/mcp",
			request:   "request",
			challenge: "challenge",
		},
		{
			name:      "other transaction",
			user:      "user",
			component: "component",
			url:       "https://provider/mcp",
			request:   "other",
			challenge: "challenge",
		},
		{
			name:      "other requirement",
			user:      "user",
			component: "component",
			url:       "https://provider/mcp",
			request:   "request",
			challenge: "other",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.FindMCPOAuthPendingState(t.Context(), test.user, test.component, test.url, test.request, test.challenge)

			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}

	pending, err := client.FindMCPOAuthPendingState(t.Context(), "user", "component", "https://provider/mcp", "request", "challenge")
	require.NoError(t, err)
	require.Equal(t, "state", pending.State)
	require.Equal(t, "verifier", pending.Verifier)

	require.NoError(t, client.db.WithContext(t.Context()).Model(pending).Update("created_at", time.Now().Add(-MCPOAuthPendingStateTTL-time.Second)).Error)

	_, err = client.FindMCPOAuthPendingState(t.Context(), "user", "component", "https://provider/mcp", "request", "challenge")

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
