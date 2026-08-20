package mcphookcorrelation

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
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name      string
		createdAt time.Time
		deleted   bool
		delay     time.Duration
	}{
		{name: "retains recent correlation", createdAt: now.Add(-23 * time.Hour), delay: time.Hour},
		{name: "deletes expired correlation", createdAt: now.Add(-25 * time.Hour), deleted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			correlation := &v1.MCPHookCorrelation{
				Namespace: "default", Name: "correlation",
				Spec: v1.MCPHookCorrelationSpec{
					ExpiresAt: metav1.NewTime(tt.createdAt.Add(v1.MCPHookCorrelationTTL)),
				}}
			client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(correlation).Build()
			handler := &Handler{now: func() time.Time { return now }}
			req := router.Request{Client: client, Object: correlation, Ctx: t.Context(), Namespace: correlation.Namespace, Name: correlation.Name}
			response := &router.ResponseWrapper{}
			if err := handler.Cleanup(req, response); err != nil {
				t.Fatal(err)
			}
			if response.Delay != tt.delay {
				t.Fatalf("retry delay = %v, want %v", response.Delay, tt.delay)
			}
			err := client.Get(t.Context(), kclient.ObjectKeyFromObject(correlation), new(v1.MCPHookCorrelation))
			if got := apierrors.IsNotFound(err); got != tt.deleted {
				t.Fatalf("deleted=%v, want %v (err=%v)", got, tt.deleted, err)
			}
		})
	}
}
