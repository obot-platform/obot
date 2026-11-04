package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestFindIdentitiesForUserAndProvider(t *testing.T) {
	c := newTestClient(t)
	identities := []types.Identity{
		{
			AuthProviderName:      "okta",
			AuthProviderNamespace: "default",
			HashedProviderUserID:  "alice",
			UserID:                7,
		},
		{
			AuthProviderName:      "okta",
			AuthProviderNamespace: "other",
			HashedProviderUserID:  "alice",
			UserID:                7,
		},
		{
			AuthProviderName:      "github",
			AuthProviderNamespace: "default",
			HashedProviderUserID:  "alice",
			UserID:                7,
		},
		{
			AuthProviderName:      "okta",
			AuthProviderNamespace: "default",
			HashedProviderUserID:  "bob",
			UserID:                8,
		},
	}
	require.NoError(t, c.db.WithContext(t.Context()).Create(&identities).Error)
	for _, tt := range []struct {
		name      string
		provider  string
		namespace string
		want      []types.Identity
	}{
		{
			name: "API key without provider",
			want: identities[:3],
		},
		{
			name:      "provider and namespace",
			provider:  "okta",
			namespace: "default",
			want:      identities[:1],
		},
		{
			name:     "provider only",
			provider: "okta",
			want:     identities[:2],
		},
		{
			name:      "namespace only",
			namespace: "default",
			want:      []types.Identity{identities[0], identities[2]},
		},
		{
			name:     "unknown provider",
			provider: "missing",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.FindIdentitiesForUserAndProvider(t.Context(), 7, tt.provider, tt.namespace)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.want, got)
		})
	}

	// API keys lack provider fields, so their account lookup needs its own index.
	var plan []struct {
		Detail string
	}
	require.NoError(t, c.db.WithContext(t.Context()).Raw("EXPLAIN QUERY PLAN SELECT * FROM identities WHERE user_id = ?", 7).Scan(&plan).Error)
	require.Len(t, plan, 1)
	require.Contains(t, plan[0].Detail, "USING INDEX idx_identities_user_id")
}
