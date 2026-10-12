package setup

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// failingUpdates fails every update while fail is set.
type failingUpdates struct {
	kclient.Client
	fail bool
}

func (f *failingUpdates) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	if f.fail {
		return errors.New("update refused")
	}
	return f.Client.Update(ctx, obj, opts...)
}

func TestCleanUpGroupSubjects(t *testing.T) {
	ctx := t.Context()
	both := policy("both", "okta/deleted")
	both.Spec.Manifest.Subjects = append(both.Spec.Manifest.Subjects, clienttypes.Subject{
		Type: clienttypes.SubjectTypeGroup,
		ID:   "okta/kept",
	}, clienttypes.Subject{
		Type: clienttypes.SubjectTypeGroup,
		ID:   "okta/also-deleted",
	})
	storage := &failingUpdates{
		Client: fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(both, policy("other", "okta/kept")).
			Build(),
		fail: true,
	}
	gateway, db := newTestGatewayWithDB(t, storage)
	for _, groupID := range []string{"okta/deleted", "okta/also-deleted"} {
		if err := db.Create(&types.SCIMGroupSubjectCleanup{
			GroupID:   groupID,
			Namespace: system.DefaultNamespace,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	subjects := func(name string) []string {
		t.Helper()
		var p v1.ModelAccessPolicy
		if err := storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &p); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(p.Spec.Manifest.Subjects))
		for _, subject := range p.Spec.Manifest.Subjects {
			ids = append(ids, subject.ID)
		}
		return ids
	}
	cleanups := func() []types.SCIMGroupSubjectCleanup {
		t.Helper()
		var cleanups []types.SCIMGroupSubjectCleanup
		if err := db.Order("group_id").Find(&cleanups).Error; err != nil {
			t.Fatal(err)
		}
		return cleanups
	}

	// Failed cleanups are kept, and retried once their claims expire.
	if err := CleanUpGroupSubjects(ctx, gateway, storage); err == nil {
		t.Fatal("a cleanup whose updates were refused succeeded")
	}
	if failed := cleanups(); len(failed) != 2 || failed[0].Attempts != 1 || failed[1].Attempts != 1 {
		t.Fatalf("the failed cleanups = %+v", failed)
	}
	storage.fail = false
	if err := CleanUpGroupSubjects(ctx, gateway, storage); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(subjects("both"), []string{"okta/deleted", "okta/kept", "okta/also-deleted"}) {
		t.Fatal("a cleanup ran before its claim expired")
	}
	if err := db.Model(new(types.SCIMGroupSubjectCleanup)).Where("1 = 1").
		UpdateColumn("claimed_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}

	if err := CleanUpGroupSubjects(ctx, gateway, storage); err != nil {
		t.Fatal(err)
	}
	if got := subjects("both"); !slices.Equal(got, []string{"okta/kept"}) {
		t.Fatalf("subjects after the cleanup = %v", got)
	}
	if got := subjects("other"); !slices.Equal(got, []string{"okta/kept"}) {
		t.Fatalf("an unrelated policy's subjects = %v", got)
	}
	if remaining := cleanups(); len(remaining) != 0 {
		t.Fatalf("the completed cleanups remain: %+v", remaining)
	}
}

func TestRunGroupSubjectCleanupsRunsAtOnceAndStopsWithItsContext(t *testing.T) {
	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(policy("deleted", "okta/deleted")).
		Build()
	gateway, db := newTestGatewayWithDB(t, storage)
	if err := db.Create(&types.SCIMGroupSubjectCleanup{
		GroupID:   "okta/deleted",
		Namespace: system.DefaultNamespace,
	}).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		RunGroupSubjectCleanups(ctx, gateway, storage)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var p v1.ModelAccessPolicy
		if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "deleted"}, &p); err != nil {
			t.Fatal(err)
		}
		if len(p.Spec.Manifest.Subjects) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the cleanup did not run once the runner started")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the runner did not stop once its context was done")
	}
}
