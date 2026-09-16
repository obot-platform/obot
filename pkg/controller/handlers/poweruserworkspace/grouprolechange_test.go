package poweruserworkspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestHandleGroupRoleChange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		empty      bool
		failCreate bool
	}{
		{
			name: "notifies only group members",
		},
		{
			name:  "empty group completes",
			empty: true,
		},
		{
			name:       "partial failure preserves event for retry",
			failCreate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
			require.NoError(t, err)
			db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate())
			gateway := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
			t.Cleanup(func() { require.NoError(t, gateway.Close()) })
			var members []uint
			for _, username := range []string{"alice", "bob", "outsider"} {
				u := &gatewaytypes.User{
					Username:       username,
					HashedUsername: username,
				}
				require.NoError(t, db.WithContext(t.Context()).Create(u).Error)
				group := "other"
				if !tc.empty && username != "outsider" {
					group = "team"
					members = append(members, u.ID)
				}
				require.NoError(t, db.WithContext(t.Context()).Create(&gatewaytypes.GroupMemberships{UserID: u.ID, GroupID: group}).Error)
			}
			event := &v1.GroupRoleChange{
				Name:      "role-change",
				Namespace: system.DefaultNamespace,
				Spec:      v1.GroupRoleChangeSpec{GroupName: "team"},
			}
			attempts := 0
			createErr := errors.New("create failed")
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(event).
				WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					require.IsType(t, &v1.UserRoleChange{}, obj)
					attempts++
					if tc.failCreate && attempts == 2 {
						return createErr
					}
					return client.Create(ctx, obj, opts...)
				}}).Build()
			handler := NewHandler(gateway)
			req := router.Request{Ctx: t.Context(), Client: client, Object: event, Namespace: event.Namespace}
			err = handler.HandleGroupRoleChange(req, nil)
			if tc.failCreate {
				require.ErrorIs(t, err, createErr)
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(event), &v1.GroupRoleChange{}))
				var partial v1.UserRoleChangeList
				require.NoError(t, client.List(t.Context(), &partial))
				require.Len(t, partial.Items, 1)
				err = handler.HandleGroupRoleChange(req, nil)
			}
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(event), &v1.GroupRoleChange{})))
			var notifications v1.UserRoleChangeList
			require.NoError(t, client.List(t.Context(), &notifications))
			seen := map[uint]bool{}
			for _, notification := range notifications.Items {
				require.Equal(t, event.Namespace, notification.Namespace)
				require.Equal(t, system.UserRoleChangePrefix, notification.GenerateName)
				require.Contains(t, members, notification.Spec.UserID)
				seen[notification.Spec.UserID] = true
			}
			require.Len(t, seen, len(members))
			if !tc.failCreate {
				require.Len(t, notifications.Items, len(members))
			}
		})
	}
}
