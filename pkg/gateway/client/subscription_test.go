package client

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	subscriptionKey = kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
	}
)

func newTestSubscription() *v1.Subscription {
	return &v1.Subscription{
		Namespace: subscriptionKey.Namespace,
		Name:      subscriptionKey.Name,
	}
}

func TestDeleteRequestsUsageRecount(t *testing.T) {
	tests := []struct {
		name   string
		delete func(t *testing.T, c *Client)
	}{
		{
			name: "user",
			delete: func(t *testing.T, c *Client) {
				t.Helper()

				user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)
				require.NoError(t, c.DeleteUser(t.Context(), fmt.Sprint(user.ID)))
			},
		},
		{
			name: "SCIM user",
			delete: func(t *testing.T, c *Client) {
				t.Helper()

				conn := createLookupTestConnection(t, c)
				provisioned, err := provisionLookupTestUser(t, c, conn, "oid-1")
				require.NoError(t, err)
				_, err = c.DeleteSCIMUser(t.Context(), conn, provisioned.ID)
				require.NoError(t, err)
			},
		},
		{
			name: "identity and user",
			delete: func(t *testing.T, c *Client) {
				t.Helper()

				user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)
				_, err := c.RemoveIdentityAndUser(t.Context(), &types.Identity{UserID: user.ID})
				require.NoError(t, err)
			},
		},
		{
			name: "device",
			delete: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, enrollDeviceLimitTestDevice(t.Context(), c, "device-1", DeviceLimit{Unlimited: true}))
				require.NoError(t, c.DeleteDevice(t.Context(), "device-1", false))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newLookupTestClient(t)
			storage := c.storageClient.(kclient.WithWatch)
			require.NoError(t, storage.Create(t.Context(), newTestSubscription()))
			var requests int
			c.storageClient = interceptor.NewClient(storage, interceptor.Funcs{
				Patch: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, patch kclient.Patch, opts ...kclient.PatchOption) error {
					if _, ok := obj.(*v1.Subscription); !ok {
						return client.Patch(ctx, obj, patch, opts...)
					}
					requests++
					// Check that the delete committed before the request. The count runs on another connection, so it
					// misses an uncommitted delete, or with SQLite, waits for it until the timeout.
					countCtx, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					users, err := c.UserCount(countCtx)
					require.NoError(t, err)
					devices, err := c.DeviceCount(countCtx)
					require.NoError(t, err)
					require.Zero(t, users+devices)
					return client.Patch(ctx, obj, patch, opts...)
				},
			})

			tt.delete(t, c)

			// The request changes nothing, so it can't ask for a sync with billing.
			require.Equal(t, 1, requests)
			var subscription v1.Subscription
			require.NoError(t, c.storageClient.Get(t.Context(), subscriptionKey, &subscription))
			require.Empty(t, subscription.Annotations)
		})
	}
}

func TestRemovingNoUserRequestsNoUsageRecount(t *testing.T) {
	c := newLifecycleTestClient(t, newTestSubscription())
	var requests int
	c.storageClient = interceptor.NewClient(c.storageClient.(kclient.WithWatch), interceptor.Funcs{
		Patch: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, patch kclient.Patch, opts ...kclient.PatchOption) error {
			requests++
			return client.Patch(ctx, obj, patch, opts...)
		},
	})

	// Startup removes the "nobody" user this way, usually finding none.
	userID, err := c.RemoveIdentityAndUser(t.Context(), &types.Identity{
		ProviderUsername:     "nobody",
		ProviderUserID:       "nobody",
		HashedProviderUserID: hash.String("nobody"),
	})
	require.NoError(t, err)
	require.Zero(t, userID)
	require.Zero(t, requests)
}

func TestDeleteSucceedsWhenTheUsageRecountRequestFails(t *testing.T) {
	c := newLifecycleTestClient(t, newTestSubscription())
	c.storageClient = interceptor.NewClient(c.storageClient.(kclient.WithWatch), interceptor.Funcs{
		Patch: func(context.Context, kclient.WithWatch, kclient.Object, kclient.Patch, ...kclient.PatchOption) error {
			return errors.New("storage unavailable")
		},
	})

	require.NoError(t, enrollDeviceLimitTestDevice(t.Context(), c, "device-1", DeviceLimit{Unlimited: true}))
	require.NoError(t, c.DeleteDevice(t.Context(), "device-1", false))

	_, err := c.GetDeviceByDeviceID(t.Context(), "device-1")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
