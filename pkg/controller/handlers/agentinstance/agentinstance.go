package agentinstance

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	gt "github.com/obot-platform/obot/pkg/gateway/types"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	Substrate *substrate.Client
	Gateway   *client.Client
}

func (h *Handler) Reconcile(req router.Request, resp router.Response) error {
	instance := req.Object.(*v1.AgentInstance)

	// Legacy SDK instances are no longer managed; retain their stored workspaces.
	if instance.Spec.ClaudeOrder == "" || instance.Spec.ClaudeEnvironment == "" {
		return nil
	}

	// A disconnected account, deleted environment, or revoked profile must not
	// keep a runner alive or allow a queued work order to start.
	environment, accessErr := clauderunner.AuthorizedEnvironment(req.Ctx, h.Gateway, instance.Spec.ClaudeEnvironment, instance.Spec.ClaudeAccount, instance.Spec.UserID)
	if clauderunner.AccessDenied(accessErr) {
		return req.Client.Delete(req.Ctx, instance)
	}
	if accessErr != nil {
		return accessErr
	}

	age := time.Since(instance.CreationTimestamp.Time)
	unlinked := strings.HasPrefix(instance.Spec.UserID, "claude-unlinked-")
	if unlinked || (!instance.Spec.ClaudeDone && age > 3*time.Hour) || (!instance.Spec.ClaudeDone && instance.Status.State != "RUNNING" && age > 15*time.Minute) {
		return req.Client.Delete(req.Ctx, instance)
	}

	days := sessionRetention(instance)
	if days > 0 && instance.Spec.ClaudeDone && sessionIdleAge(instance).Hours()/24 >= float64(days) {
		var deleted bool
		err := h.Gateway.WithClaudeProvisionLock(req.Ctx, func() error {
			var err error
			deleted, err = h.expireSession(req, instance)
			return err
		})
		if err != nil || deleted {
			return err
		}
	}

	previous := instance.Status
	ctx, cancel := context.WithTimeout(req.Ctx, 30*time.Second)
	defer cancel()

	state, err := h.reconcileClaude(ctx, instance, environment.Runtime.EgressHosts, environment.Runtime.AllowAllEgress, environment.EgressSecrets)
	if removed, err := removeCrashed(req, instance, state, err); removed {
		return err
	}
	resumeAttempted := state == "ReadyToStart" && err == nil
	if resumeAttempted {
		state, err = h.resumeClaude(ctx, req, instance)
	}
	instance.Status = v1.AgentInstanceStatus{State: state, ObservedGeneration: instance.Generation, ClaudeStarted: instance.Status.ClaudeStarted}
	if instance.Spec.ClaudeDone && !instance.Spec.ClaudeCheckpoint {
		instance.Status.Error = "Claude runner ended without a workspace checkpoint. The next attempt starts from a fresh checkout."
	}
	if err != nil {
		// Do not copy RPC errors into user-visible state: upstream validation may contain environment values.
		slog.ErrorContext(ctx, "Agent reconciliation failed", "instance", instance.Name, "code", status.Code(err))
		instance.Status.Error = "Session reconciliation failed. Obot will retry automatically. Check the Substrate control plane and worker status."
		if status.Code(err) == codes.ResourceExhausted {
			instance.Status.Error = "Waiting for an available Substrate worker; startup will retry automatically."
		}
	}
	if resumeAttempted || previous != instance.Status {
		if err := req.Client.Status().Update(req.Ctx, instance); err != nil {
			return err
		}
	}
	if instance.Spec.ClaudeDone && state == "SUSPENDED" && err == nil {
		if err := h.Gateway.WithClaudeProvisionLock(req.Ctx, func() error { return h.pruneExecutions(req, instance) }); err != nil {
			return err
		}
		// Parked history needs retention checks, not continuous runner polling.
		resp.RetryAfter(time.Hour)
		return nil
	}
	resp.RetryAfter(5 * time.Second)

	return nil
}

// The caller holds the provisioning lock so a new execution cannot race deletion.
func (h *Handler) expireSession(req router.Request, instance *v1.AgentInstance) (bool, error) {
	days := sessionRetention(instance)
	if days <= 0 {
		return false, nil
	}

	var all v1.AgentInstanceList
	if err := req.Client.List(req.Ctx, &all, kclient.InNamespace(instance.Namespace)); err != nil {
		return false, err
	}
	var attempts []*v1.AgentInstance
	for i := range all.Items {
		attempt := &all.Items[i]
		if attempt.Spec.ClaudeOrder == "" || attempt.Spec.ClaudeSession != instance.Spec.ClaudeSession ||
			attempt.Spec.ClaudeEnvironment != instance.Spec.ClaudeEnvironment || attempt.Spec.ClaudeAccount != instance.Spec.ClaudeAccount ||
			attempt.Spec.UserID != instance.Spec.UserID {
			continue
		}
		// Keep the whole session while any execution is recent or not safely parked.
		if !attempt.Spec.ClaudeDone || attempt.Status.State != "SUSPENDED" || sessionIdleAge(attempt).Hours()/24 < float64(days) {
			return false, nil
		}
		attempts = append(attempts, attempt)
	}

	for _, attempt := range attempts {
		if err := req.Client.Delete(req.Ctx, attempt); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
	}
	return true, nil
}

func (h *Handler) resumeClaude(ctx context.Context, req router.Request, instance *v1.AgentInstance) (string, error) {
	// Persist before the RPC. Ambiguous failures must not reuse a single-use order.
	previous := instance.Status
	instance.Status.ClaudeStarted = true
	instance.Status.State = "Resuming"
	if err := req.Client.Status().Update(req.Ctx, instance); err != nil {
		instance.Status = previous
		return previous.State, err
	}

	err := h.resumeActor(ctx, substrate.Ref(instance))
	if status.Code(err) == codes.ResourceExhausted {
		// Capacity rejection happens before the runner starts, so the order is unused.
		instance.Status.ClaudeStarted = false
		return "Pending", err
	}

	return "Resuming", err
}

func (h *Handler) reconcileClaude(ctx context.Context, instance *v1.AgentInstance, egressHosts []string, allowAllEgress bool, egressSecrets []gt.ClaudeEgressSecret) (string, error) {
	credential, err := h.Gateway.RevealCredential(ctx, []string{"claude-runner"}, instance.Name)
	if err != nil {
		return "Pending", err
	}
	actor, err := h.Substrate.EnsureClaude(ctx, instance, credential.Secrets, egressHosts, allowAllEgress, egressSecrets)
	if err != nil {
		return "Pending", err
	}
	state := actor.GetStatus().GetState()
	ref := substrate.Ref(instance)
	if recovering, err := h.recoverResume(ctx, instance, ref, state); recovering {
		return "Resuming", err
	}
	if instance.Spec.ClaudeDone {
		if suspending, err := h.suspendClaude(ctx, ref, state); suspending {
			return "Suspending", err
		}
	} else if state == pb.ActorState_ACTOR_STATE_PAUSED || state == pb.ActorState_ACTOR_STATE_SUSPENDED {
		// A work order is single-use: never restart an execution that was already launched.
		if !instance.Status.ClaudeStarted {
			return "ReadyToStart", nil
		}
	}
	return strings.TrimPrefix(state.String(), "ACTOR_STATE_"), nil
}

// removeCrashed deletes an execution whose actor crashed. A crashed execution cannot
// restart its single-use work order, and the next attempt waits for it to checkpoint,
// so keeping it blocks the session until the stale-execution timeout. Removing it lets
// the session resume from its last checkpoint. It reports false when the actor did not
// crash.
func removeCrashed(req router.Request, instance *v1.AgentInstance, state string, err error) (bool, error) {
	if state != "CRASHED" || err != nil {
		return false, nil
	}
	return true, req.Client.Delete(req.Ctx, instance)
}

// resumeActor resumes an execution's actor. Substrate restores the actor during
// ResumeActor, which can outlast the reconcile's deadline, and a canceled resume leaves the
// actor RESUMING without a running workload. So the call gets its own deadline.
func (h *Handler) resumeActor(ctx context.Context, ref *pb.ObjectRef) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	_, err := h.Substrate.Control.ResumeActor(ctx, &pb.ResumeActorRequest{Actor: ref})
	return err
}

// recoverResume resumes a started execution's actor again when an earlier resume was
// interrupted, which leaves the actor RESUMING. Substrate's resume workflow is idempotent and
// picks up where the interrupted one stopped. It reports false when there is nothing to
// recover.
func (h *Handler) recoverResume(ctx context.Context, instance *v1.AgentInstance, ref *pb.ObjectRef, state pb.ActorState) (bool, error) {
	if state != pb.ActorState_ACTOR_STATE_RESUMING || !instance.Status.ClaudeStarted || instance.Spec.ClaudeDone {
		return false, nil
	}
	return true, h.resumeActor(ctx, ref)
}

// suspendClaude suspends a finished execution's actor. It reports false when the actor
// needs no suspend. Substrate checkpoints the actor during SuspendActor, which can outlast
// the reconcile's deadline, and a canceled suspend leaves the actor SUSPENDING. So the call
// gets its own deadline, and SUSPENDING actors are suspended again: Substrate resumes the
// interrupted suspend.
func (h *Handler) suspendClaude(ctx context.Context, ref *pb.ObjectRef, state pb.ActorState) (bool, error) {
	if state != pb.ActorState_ACTOR_STATE_RUNNING && state != pb.ActorState_ACTOR_STATE_SUSPENDING {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	_, err := h.Substrate.Control.SuspendActor(ctx, &pb.SuspendActorRequest{Actor: ref})
	return true, err
}

func (h *Handler) Cleanup(req router.Request, resp router.Response) error {
	instance := req.Object.(*v1.AgentInstance)
	ctx, cancel := context.WithTimeout(req.Ctx, 30*time.Second)
	defer cancel()

	revoke := func() error {
		return h.Gateway.RevokeClaudeCredentials(ctx, instance.Name)
	}
	var err error
	if instance.Spec.ClaudeOrder != "" {
		// Credential issuance checks deletion under the same lock. A request that
		// started before deletion must finish before its key can be revoked here.
		err = h.Gateway.WithClaudeProvisionLock(ctx, revoke)
	} else {
		err = revoke()
	}
	if err != nil {
		return err
	}

	if h.Substrate != nil {
		if err := h.Substrate.DeleteExecution(ctx, substrate.Ref(instance)); err != nil {
			resp.RetryAfter(5 * time.Second)
			return err
		}
	}
	_, err = h.Gateway.DeleteCredential(ctx, "substrate-agent", instance.Name)
	if err == nil {
		_, err = h.Gateway.DeleteCredential(ctx, client.ClaudeLLMCredentialContext, instance.Name)
	}
	if err == nil && instance.Spec.ClaudeOrder != "" {
		_, err = h.Gateway.DeleteCredential(ctx, "claude-runner", instance.Name)
	}

	return err
}

func sessionIdleAge(instance *v1.AgentInstance) time.Duration {
	if !instance.Spec.ClaudeFinishedAt.IsZero() {
		return time.Since(instance.Spec.ClaudeFinishedAt.Time)
	}

	return time.Since(instance.CreationTimestamp.Time)
}

func sessionRetention(instance *v1.AgentInstance) int {
	var runtime gt.ClaudeRuntime
	if json.Unmarshal([]byte(instance.Spec.ClaudeRuntime), &runtime) != nil {
		return -1
	}
	return runtime.Resources.SessionRetentionDays
}

// Retain the newest execution for reconnects, removing older attempts only after
// all successors are parked and own their checkpoints. Cleanup also checks tags.
func (h *Handler) pruneExecutions(req router.Request, instance *v1.AgentInstance) error {
	var all v1.AgentInstanceList
	if err := req.Client.List(req.Ctx, &all, kclient.InNamespace(instance.Namespace)); err != nil {
		return err
	}
	var previous []*v1.AgentInstance
	for i := range all.Items {
		other := &all.Items[i]
		if other.Name == instance.Name || other.Spec.ClaudeEnvironment != instance.Spec.ClaudeEnvironment || other.Spec.ClaudeSession != instance.Spec.ClaudeSession || other.Spec.ClaudeAccount != instance.Spec.ClaudeAccount || other.Spec.UserID != instance.Spec.UserID {
			continue
		}
		if !other.CreationTimestamp.Before(&instance.CreationTimestamp) || !other.Spec.ClaudeDone || other.Status.State != "SUSPENDED" {
			return nil
		}
		previous = append(previous, other)
	}
	if instance.Spec.ClaudePrevious != "" {
		instance.Spec.ClaudePrevious = ""
		if err := req.Client.Update(req.Ctx, instance); err != nil {
			return err
		}
	}
	for _, old := range previous {
		if err := req.Client.Delete(req.Ctx, old); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
