package client

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestCredentialContexts(t *testing.T) {
	c := newTestClient(t)
	for _, credential := range []types.Credential{
		{Context: "vmcp-profile/vmcp1a/vp1one", Name: "configuration-1", Secrets: map[string]string{"k": "v"}},
		{Context: "vmcp-profile/vmcp1a/vp1one", Name: "configuration-2", Secrets: map[string]string{"k": "v"}},
		{Context: "vmcp-profile/vmcp1a/vp1two", Name: "configuration-1", Secrets: map[string]string{"k": "v"}},
		{Context: "vmcp-profile/vmcp1b/vp1one", Name: "configuration-1", Secrets: map[string]string{"k": "v"}},
	} {
		require.NoError(t, c.UpsertCredential(t.Context(), credential))
	}

	found, err := c.CredentialContextsWithCredentials(t.Context(), []string{"vmcp-profile/vmcp1a/vp1one", "vmcp-profile/vmcp1a/vp1two", "vmcp-profile/vmcp1a/vp1none"})
	require.NoError(t, err)
	slices.Sort(found)
	require.Equal(t, []string{"vmcp-profile/vmcp1a/vp1one", "vmcp-profile/vmcp1a/vp1two"}, found)
	found, err = c.CredentialContextsWithCredentials(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, found)

	// Deleting a context deletes every credential in it and nothing else, and can be repeated.
	for range 2 {
		require.NoError(t, c.DeleteCredentialsInContexts(t.Context(), []string{"vmcp-profile/vmcp1a/vp1one", "vmcp-profile/vmcp1a/vp1none"}))
	}
	require.NoError(t, c.DeleteCredentialsInContexts(t.Context(), nil))
	found, err = c.CredentialContextsWithCredentials(t.Context(), []string{"vmcp-profile/vmcp1a/vp1one", "vmcp-profile/vmcp1a/vp1two", "vmcp-profile/vmcp1b/vp1one"})
	require.NoError(t, err)
	slices.Sort(found)
	require.Equal(t, []string{"vmcp-profile/vmcp1a/vp1two", "vmcp-profile/vmcp1b/vp1one"}, found)
}
