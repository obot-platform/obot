package mcpclientsession

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanup(t *testing.T) {
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)

	t.Run("deletes a session idle for a week", func(t *testing.T) {
		session := testSession("stale", now.Add(-maxIdleTime))
		client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(session).Build()
		handler := &Handler{now: func() time.Time { return now }}

		if err := handler.Cleanup(testRequest(t, client, session), &router.ResponseWrapper{}); err != nil {
			t.Fatal(err)
		}

		err := client.Get(t.Context(), kclient.ObjectKeyFromObject(session), &v1.MCPClientSession{})
		if !apierrors.IsNotFound(err) {
			t.Fatalf("expected stale session to be deleted, got %v", err)
		}
	})

	t.Run("does not schedule cleanup before the retry window", func(t *testing.T) {
		session := testSession("active", now.Add(-24*time.Hour))
		client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(session).Build()
		handler := &Handler{now: func() time.Time { return now }}
		response := &router.ResponseWrapper{}

		if err := handler.Cleanup(testRequest(t, client, session), response); err != nil {
			t.Fatal(err)
		}
		if response.Delay != 0 {
			t.Fatalf("retry delay = %v, want %v", response.Delay, 0)
		}
	})

	t.Run("schedules cleanup within the retry window", func(t *testing.T) {
		session := testSession("active", now.Add(-6*24*time.Hour-15*time.Hour))
		client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(session).Build()
		handler := &Handler{now: func() time.Time { return now }}
		response := &router.ResponseWrapper{}

		if err := handler.Cleanup(testRequest(t, client, session), response); err != nil {
			t.Fatal(err)
		}
		if response.Delay != 9*time.Hour {
			t.Fatalf("retry delay = %v, want %v", response.Delay, 9*time.Hour)
		}
	})
}

func testSession(name string, lastUsed time.Time) *v1.MCPClientSession {
	return &v1.MCPClientSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       v1.MCPClientSessionSpec{},
		Status: v1.MCPClientSessionStatus{
			LastUsed: metav1.NewTime(lastUsed),
		},
	}
}

func testRequest(t *testing.T, client kclient.WithWatch, session *v1.MCPClientSession) router.Request {
	t.Helper()
	return router.Request{
		Client:    client,
		Object:    session,
		Ctx:       t.Context(),
		Namespace: session.Namespace,
		Name:      session.Name,
	}
}
