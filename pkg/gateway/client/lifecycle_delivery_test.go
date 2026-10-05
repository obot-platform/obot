package client

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// countingOAuthTokenLists counts the lists of OAuth tokens.
type countingOAuthTokenLists struct {
	kclient.Client
	lists atomic.Int64
}

func (c *countingOAuthTokenLists) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if _, ok := list.(*v1.OAuthTokenList); ok {
		c.lists.Add(1)
	}
	return c.Client.List(ctx, list, opts...)
}

func TestMembershipReconcileEventsAreRecordedInBatches(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	counter := newStatementCounter(t, c)

	changes := make(map[uint]bool, 1200)
	for userID := uint(1); userID <= 1200; userID++ {
		changes[userID] = userID%2 == 0
	}
	var err error
	statements := counter.measure(func() {
		err = recordMembershipReconcileEventsTx(c.db.WithContext(ctx), changes)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 3 {
		t.Fatalf("recording 1200 events ran %d statements, want 3 batches of up to %d:\n%s", len(statements), scimMemberBatchSize, strings.Join(statements, "\n"))
	}

	var recorded, removed int64
	c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).Count(&recorded)
	c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).Where("groups_removed = ?", true).Count(&removed)
	if recorded != 1200 || removed != 600 {
		t.Fatalf("recorded %d events, %d of them for users who left a group; want 1200 and 600", recorded, removed)
	}
}

func TestReconcileEventsOfOneUserAreDeliveredOnce(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()

	// A user in three changed groups, who left one of them, and a user in one.
	for _, event := range []struct {
		userID        uint
		groupsRemoved bool
	}{
		{
			userID:        1,
			groupsRemoved: false,
		},
		{
			userID:        2,
			groupsRemoved: false,
		},
		{
			userID:        1,
			groupsRemoved: true,
		},
		{
			userID:        1,
			groupsRemoved: false,
		},
	} {
		if err := recordUserReconcileEvent(c.db.WithContext(ctx), event.userID, event.groupsRemoved); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatal(err)
	}

	countObjects := func(list kclient.ObjectList, userID uint) int {
		t.Helper()
		if err := c.storageClient.List(ctx, list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			t.Fatal(err)
		}
		n := 0
		switch l := list.(type) {
		case *v1.UserRoleChangeList:
			for _, item := range l.Items {
				if item.Spec.UserID == userID {
					n++
				}
			}
		case *v1.UserGroupChangeList:
			for _, item := range l.Items {
				if item.Spec.UserID == userID {
					n++
				}
			}
		}
		return n
	}
	if roles, groups := countObjects(new(v1.UserRoleChangeList), 1), countObjects(new(v1.UserGroupChangeList), 1); roles != 1 || groups != 1 {
		t.Fatalf("user 1 got %d role changes and %d group changes, want one of each", roles, groups)
	}
	if roles, groups := countObjects(new(v1.UserRoleChangeList), 2), countObjects(new(v1.UserGroupChangeList), 2); roles != 1 || groups != 0 {
		t.Fatalf("user 2 got %d role changes and %d group changes, want one role change", roles, groups)
	}

	// The delivery is named after the user's first event, so a repeated delivery finds it.
	first := lifecycleEvents(t, c, 1)[0]
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      userLifecycleObjectName(system.UserGroupChangePrefix, first.ID),
	}, new(v1.UserGroupChange)); err != nil {
		t.Fatalf("the group change is not named after the user's first event: %v", err)
	}
	for _, userID := range []uint{1, 2} {
		for _, event := range lifecycleEvents(t, c, userID) {
			if event.DeliveredAt == nil {
				t.Fatalf("event %d of user %d was not delivered", event.ID, userID)
			}
		}
	}
}

func TestDeliveryDeliversEveryBatchAtOnce(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()

	const users = 2*userLifecycleDeliveryBatchSize + 50
	changes := make(map[uint]bool, users)
	for userID := uint(1); userID <= users; userID++ {
		changes[userID] = false
	}
	if err := recordMembershipReconcileEventsTx(c.db.WithContext(ctx), changes); err != nil {
		t.Fatal(err)
	}

	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatal(err)
	}
	var pending int64
	c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).Where("delivered_at IS NULL").Count(&pending)
	if pending != 0 {
		t.Fatalf("%d events are still pending after one delivery", pending)
	}
}

func TestDisabledEventsOfABatchListOAuthTokensOnce(t *testing.T) {
	tokens := make([]kclient.Object, 0, 4)
	for userID := uint(1); userID <= 4; userID++ {
		tokens = append(tokens, &v1.OAuthToken{
			Namespace: system.DefaultNamespace,
			Name:      fmt.Sprintf("token-%d", userID),
			Spec: v1.OAuthTokenSpec{
				ClientID: "client",
				UserID:   userID,
			},
		})
	}
	c := newLifecycleTestClient(t, tokens...)
	ctx := t.Context()
	counting := &countingOAuthTokenLists{
		Client: c.storageClient,
	}
	c.storageClient = counting

	for _, name := range []string{"ann", "bob", "cid"} {
		user := createLifecycleTestUser(t, c, name, lifecycleTestProvider)
		if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatal(err)
	}

	for userID := uint(1); userID <= 4; userID++ {
		err := c.storageClient.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      fmt.Sprintf("token-%d", userID),
		}, new(v1.OAuthToken))
		if deleted := apierrors.IsNotFound(err); deleted != (userID <= 3) {
			t.Fatalf("token of user %d: deleted = %v, error = %v", userID, deleted, err)
		}
	}
	if lists := counting.lists.Load(); lists != 1 {
		t.Fatalf("delivering 3 disabled events listed the OAuth tokens %d times, want once", lists)
	}
}
