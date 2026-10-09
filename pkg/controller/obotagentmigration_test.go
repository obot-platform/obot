package controller

import (
	"context"
	"testing"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeAPIKeys struct {
	keys    []gatewaytypes.APIKey
	revoked []uint
}

func (f *fakeAPIKeys) ListAllAPIKeys(context.Context, gclient.APIKeyListOptions) ([]gatewaytypes.APIKey, error) {
	return f.keys, nil
}

func (f *fakeAPIKeys) RevokeAPIKeyByID(_ context.Context, keyID uint) error {
	f.revoked = append(f.revoked, keyID)
	return nil
}

func TestDeleteObotAgentResources(t *testing.T) {
	agentServer := &v1.MCPServer{
		Name:      system.MCPServerPrefix + "nba1agent",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID:         "1",
			NanobotAgentID: "nba1agent",
		},
	}
	userServer := &v1.MCPServer{
		Name:      system.MCPServerPrefix + "user",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID: "1",
		},
	}
	searchServer := &v1.SystemMCPServer{
		Name:      obotAgentSearchServerName,
		Namespace: system.DefaultNamespace,
	}
	otherSystemServer := &v1.SystemMCPServer{
		Name:      system.SystemMCPServerPrefix + "other",
		Namespace: system.DefaultNamespace,
	}

	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(agentServer, userServer, searchServer, otherSystemServer).
		Build()
	apiKeys := &fakeAPIKeys{
		keys: []gatewaytypes.APIKey{
			{
				ID:   1,
				Name: obotAgentAPIKeyNamePrefix + agentServer.Name,
			},
			{
				ID:   2,
				Name: obotAgentAPIKeyNamePrefix + userServer.Name,
			},
			{
				ID:   3,
				Name: "my key",
			},
		},
	}

	require.NoError(t, deleteObotAgentResources(t.Context(), client, apiKeys))

	assert.Equal(t, []uint{1}, apiKeys.revoked)
	for _, deleted := range []kclient.Object{agentServer, searchServer} {
		err := client.Get(t.Context(), kclient.ObjectKeyFromObject(deleted), deleted)
		assert.True(t, apierrors.IsNotFound(err), "%s should be deleted, got %v", deleted.GetName(), err)
	}
	for _, kept := range []kclient.Object{userServer, otherSystemServer} {
		assert.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(kept), kept), "%s should be kept", kept.GetName())
	}

	// Running again finds nothing left to revoke or delete.
	apiKeys.revoked = nil
	require.NoError(t, deleteObotAgentResources(t.Context(), client, apiKeys))
	assert.Empty(t, apiKeys.revoked)
}
