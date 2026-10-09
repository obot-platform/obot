package agentinstance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type resumeControl struct {
	pb.ControlClient
	resume func() error
}

func (c *resumeControl) ResumeActor(context.Context, *pb.ResumeActorRequest, ...grpc.CallOption) (*pb.ResumeActorResponse, error) {
	return &pb.ResumeActorResponse{}, c.resume()
}

func TestResumeClaudeCapacityRetry(t *testing.T) {
	for _, code := range []codes.Code{codes.ResourceExhausted, codes.Unavailable, codes.DeadlineExceeded, codes.OK} {
		t.Run(code.String(), func(t *testing.T) {
			ctx := context.Background()
			instance := &v1.AgentInstance{}
			instance.Name = "attempt"
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).
				WithStatusSubresource(instance).WithObjects(instance).Build()
			req := router.Request{Ctx: ctx, Client: storage}
			control := &resumeControl{
				resume: func() error {
					var stored v1.AgentInstance
					require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(instance), &stored))
					require.True(t, stored.Status.ClaudeStarted, "record intent before sending the RPC")
					return status.Error(code, "resume result")
				},
			}
			h := Handler{Substrate: &substrate.Client{Control: control}}

			state, err := h.resumeClaude(ctx, req, instance)
			require.Equal(t, code, status.Code(err))
			if code != codes.ResourceExhausted {
				require.True(t, instance.Status.ClaudeStarted, "never retry an ambiguous start")
				require.Equal(t, "Resuming", state)
				return
			}

			require.False(t, instance.Status.ClaudeStarted)
			require.Equal(t, "Pending", state)
			instance.Status.State = state
			require.NoError(t, storage.Status().Update(ctx, instance))
			code = codes.OK

			state, err = h.resumeClaude(ctx, req, instance)
			require.NoError(t, err)
			require.True(t, instance.Status.ClaudeStarted)
			require.Equal(t, "Resuming", state)
		})
	}
}

func TestSessionRetention(t *testing.T) {
	for _, tc := range []struct {
		name            string
		days            int
		latestAgeDays   int
		finishedAgeDays int
		recordedFinish  bool
		unfinished      bool
		running         bool
		otherOwner      bool
		deleteOld       bool
	}{
		{name: "long-running session just finished", days: 30, latestAgeDays: 31, recordedFinish: true},
		{name: "recent finish protects history", days: 30, latestAgeDays: 40, finishedAgeDays: 29, recordedFinish: true},
		{name: "expired since finish", days: 30, latestAgeDays: 40, finishedAgeDays: 31, recordedFinish: true, deleteOld: true},
		{name: "expired history", days: 30, latestAgeDays: 31, deleteOld: true},
		{name: "recent execution protects history", days: 30, latestAgeDays: 29},
		{name: "unfinished execution protects history", days: 30, latestAgeDays: 31, unfinished: true},
		{name: "waiting for suspension", days: 30, latestAgeDays: 31, running: true},
		{name: "disabled", days: -1, latestAgeDays: 31},
		{name: "custom retention", days: 60, latestAgeDays: 31},
		{name: "other owner is independent", days: 30, latestAgeDays: 1, otherOwner: true, deleteOld: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			old := &v1.AgentInstance{
				Spec: v1.AgentInstanceSpec{
					UserID:            "owner",
					ClaudeRuntime:     fmt.Sprintf(`{"resources":{"sessionRetentionDays":%d}}`, tc.days),
					ClaudeEnvironment: "environment",
					ClaudeAccount:     "account",
					ClaudeSession:     "session",
					ClaudeOrder:       "order-old",
					ClaudeDone:        true,
					ClaudeCheckpoint:  true,
				},
				Status: v1.AgentInstanceStatus{State: "SUSPENDED"},
			}
			old.Name = "old"
			old.Namespace = "default"
			old.CreationTimestamp = metav1.NewTime(time.Now().Add(-40 * 24 * time.Hour))
			old.Finalizers = []string{substrate.Finalizer}

			latest := old.DeepCopy()
			latest.Name = "latest"
			latest.Spec.ClaudeOrder = "order-latest"
			latest.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Duration(tc.latestAgeDays) * 24 * time.Hour))
			latest.Spec.ClaudeDone = !tc.unfinished
			if tc.recordedFinish {
				latest.Spec.ClaudeFinishedAt = metav1.NewTime(time.Now().Add(-time.Duration(tc.finishedAgeDays) * 24 * time.Hour))
			}
			if tc.running {
				latest.Status.State = "RUNNING"
			}
			if tc.otherOwner {
				latest.Spec.UserID = "other"
			}
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(old, latest).Build()
			h := Handler{}

			deleted, err := h.expireSession(router.Request{Ctx: ctx, Client: storage}, old)
			require.NoError(t, err)
			require.Equal(t, tc.deleteOld, deleted)
			require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(old), old))
			require.Equal(t, tc.deleteOld, old.DeletionTimestamp != nil, "retain finalizers for resource cleanup")
			require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(latest), latest))
			require.Equal(t, tc.deleteOld && !tc.otherOwner, latest.DeletionTimestamp != nil)
		})
	}
}

func TestPruneCompletedExecutions(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			old := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: "owner", ClaudeEnvironment: "env", ClaudeSession: "session", ClaudeAccount: "account", ClaudeDone: !active}, Status: v1.AgentInstanceStatus{State: "SUSPENDED"}}
			old.Name, old.Namespace = "old", "default"
			old.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
			old.Finalizers = []string{substrate.Finalizer}
			latest := old.DeepCopy()
			latest.Name = "latest"
			latest.CreationTimestamp = metav1.NewTime(time.Now())
			latest.Spec.ClaudeDone = true
			latest.Spec.ClaudePrevious = "old"
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(old, latest).Build()
			h := Handler{}
			require.NoError(t, h.pruneExecutions(router.Request{Ctx: t.Context(), Client: storage}, latest))
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(old), old))
			require.Equal(t, !active, old.DeletionTimestamp != nil)
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(latest), latest))
			require.Equal(t, active, latest.Spec.ClaudePrevious != "")
			require.Nil(t, latest.DeletionTimestamp)
		})
	}
}
