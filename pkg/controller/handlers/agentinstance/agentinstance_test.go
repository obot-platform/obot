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
	resume   func() error
	calls    int
	deadline time.Time
}

type suspendControl struct {
	pb.ControlClient
	calls    int
	deadline time.Time
}

func (c *resumeControl) ResumeActor(ctx context.Context, _ *pb.ResumeActorRequest, _ ...grpc.CallOption) (*pb.ResumeActorResponse, error) {
	c.calls++
	c.deadline, _ = ctx.Deadline()
	if c.resume == nil {
		return &pb.ResumeActorResponse{}, nil
	}
	return &pb.ResumeActorResponse{}, c.resume()
}

func (c *suspendControl) SuspendActor(ctx context.Context, _ *pb.SuspendActorRequest, _ ...grpc.CallOption) (*pb.SuspendActorResponse, error) {
	c.calls++
	c.deadline, _ = ctx.Deadline()
	return &pb.SuspendActorResponse{}, nil
}

func TestSuspendClaudeResumesInterruptedSuspends(t *testing.T) {
	for _, tc := range []struct {
		state   pb.ActorState
		suspend bool
	}{
		{state: pb.ActorState_ACTOR_STATE_RUNNING, suspend: true},
		// A suspend canceled partway leaves the actor SUSPENDING; Substrate resumes it.
		{state: pb.ActorState_ACTOR_STATE_SUSPENDING, suspend: true},
		{state: pb.ActorState_ACTOR_STATE_SUSPENDED},
		{state: pb.ActorState_ACTOR_STATE_RESUMING},
	} {
		t.Run(tc.state.String(), func(t *testing.T) {
			control := &suspendControl{}
			h := Handler{Substrate: &substrate.Client{Control: control}}
			// The reconcile's own deadline is too short for a checkpoint.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			suspending, err := h.suspendClaude(ctx, &pb.ObjectRef{Name: "attempt"}, tc.state)
			require.NoError(t, err)
			require.Equal(t, tc.suspend, suspending)
			if !tc.suspend {
				require.Zero(t, control.calls)
				return
			}
			require.Equal(t, 1, control.calls)
			require.WithinDuration(t, time.Now().Add(5*time.Minute), control.deadline, 5*time.Second, "the suspend gets its own deadline")
		})
	}
}

func TestRecoverResumeRetriesInterruptedResumes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   pb.ActorState
		started bool
		done    bool
		recover bool
	}{
		// A resume canceled partway leaves the actor RESUMING; Substrate resumes it again.
		{name: "interrupted", state: pb.ActorState_ACTOR_STATE_RESUMING, started: true, recover: true},
		{name: "not started", state: pb.ActorState_ACTOR_STATE_RESUMING},
		{name: "finished", state: pb.ActorState_ACTOR_STATE_RESUMING, started: true, done: true},
		{name: "running", state: pb.ActorState_ACTOR_STATE_RUNNING, started: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := &resumeControl{}
			h := Handler{Substrate: &substrate.Client{Control: control}}
			instance := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{ClaudeDone: tc.done}, Status: v1.AgentInstanceStatus{ClaudeStarted: tc.started}}
			// The reconcile's own deadline is too short for a restore.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			recovering, err := h.recoverResume(ctx, instance, &pb.ObjectRef{Name: "attempt"}, tc.state)
			require.NoError(t, err)
			require.Equal(t, tc.recover, recovering)
			if !tc.recover {
				require.Zero(t, control.calls)
				return
			}
			require.Equal(t, 1, control.calls)
			require.WithinDuration(t, time.Now().Add(5*time.Minute), control.deadline, 5*time.Second, "the resume gets its own deadline")
		})
	}
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
			require.WithinDuration(t, time.Now().Add(5*time.Minute), control.deadline, 5*time.Second, "the resume gets its own deadline")
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
