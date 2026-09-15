package client

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrapLocalAuthUserLimit(t *testing.T) {
	c := newTestClient(t)

	first, err := c.CreateBootstrapLocalAuthUser(t.Context(), "owner@example.com", "password-hash", false)
	require.NoError(t, err)

	_, err = c.CreateBootstrapLocalAuthUser(t.Context(), "other@example.com", "password-hash", false)
	require.ErrorIs(t, err, ErrBootstrapLocalAuthUserLimit)

	_, err = c.CreateBootstrapLocalAuthUser(t.Context(), first.Email, "password-hash", false)
	require.ErrorIs(t, err, ErrBootstrapLocalAuthUserLimit)

	require.NoError(t, c.DeleteLocalAuthUser(t.Context(), first.ID))
	_, err = c.CreateBootstrapLocalAuthUser(t.Context(), "replacement@example.com", "password-hash", false)
	require.NoError(t, err)

	_, err = c.CreateLocalAuthUser(t.Context(), "staff@example.com", "password-hash", true)
	require.NoError(t, err)

	users, err := c.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 2)
}

func TestConcurrentBootstrapLocalAuthCreation(t *testing.T) {
	c := newTestClient(t)
	const attempts = 8
	results := make(chan error, attempts)
	start := make(chan struct{})
	var workers sync.WaitGroup

	for i := range attempts {
		workers.Go(func() {
			<-start
			_, err := c.CreateBootstrapLocalAuthUser(t.Context(), fmt.Sprintf("owner-%d@example.com", i), "password-hash", false)
			results <- err
		})
	}

	close(start)
	workers.Wait()
	close(results)

	var successes int
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrBootstrapLocalAuthUserLimit)
		}
	}

	require.Equal(t, 1, successes)
	users, err := c.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 1)
}
