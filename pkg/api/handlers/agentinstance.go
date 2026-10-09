package handlers

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ClaudeSessionHandler struct{}

func agentSessionID(instance *v1.AgentInstance) string {
	sum := sha256.Sum256([]byte(instance.Spec.ClaudeEnvironment + "\x00" + instance.Spec.UserID + "\x00" + instance.Spec.ClaudeAccount + "\x00" + instance.Spec.ClaudeSession))
	return fmt.Sprintf("acs-%x", sum[:16])
}

// Public agents represent Claude sessions; work orders remain internal attempts.
func agentSessions(attempts []v1.AgentInstance) []v1.AgentInstance {
	sessions := make([]v1.AgentInstance, 0)
	indices := map[string]int{}
	latest := map[string]int{}
	for i := range attempts {
		attempt := &attempts[i]
		if attempt.Spec.ClaudeSession == "" || attempt.Spec.ClaudeOrder == "" {
			continue
		}

		id := agentSessionID(attempt)
		if index, ok := indices[id]; ok {
			current := &sessions[index]
			first := current.CreationTimestamp
			if attempt.CreationTimestamp.Before(&first) {
				first = attempt.CreationTimestamp
			}
			// Status comes from the newest execution, independently of session age.
			previous := &attempts[latest[id]]
			if attempt.CreationTimestamp.After(previous.CreationTimestamp.Time) || (attempt.CreationTimestamp.Equal(&previous.CreationTimestamp) && attempt.Name > previous.Name) {
				*current = *attempt.DeepCopy()
				latest[id] = i
			}
			current.CreationTimestamp = first
			continue
		}

		indices[id] = len(sessions)
		latest[id] = i
		sessions = append(sessions, *attempt.DeepCopy())
	}
	for i := range sessions {
		sessions[i].Name = agentSessionID(&sessions[i])
		sessions[i].UID = ktypes.UID(sessions[i].Name)
	}
	return sessions
}

func (h *ClaudeSessionHandler) List(req api.Context) error {
	all := req.UserIsAdmin() && req.URL.Query().Get("all") == "true"

	var options []kclient.ListOption
	if !all {
		options = append(options, kclient.MatchingFields{"spec.userID": req.User.GetUID()})
	}

	var instances v1.AgentInstanceList
	if err := req.List(&instances, options...); err != nil {
		return err
	}
	if all {
		instances.Items = agentSessions(instances.Items)
		return req.Write(instances)
	}

	visible, err := visibleAgentInstances(req, instances.Items)
	if err != nil {
		return err
	}
	instances.Items = agentSessions(visible)
	return req.Write(instances)
}

func (h *ClaudeSessionHandler) Get(req api.Context) error {
	var instances v1.AgentInstanceList
	if err := req.List(&instances, kclient.MatchingFields{"spec.userID": req.User.GetUID()}); err != nil {
		return err
	}
	visible, err := visibleAgentInstances(req, instances.Items)
	if err != nil {
		return err
	}
	for _, session := range agentSessions(visible) {
		if session.Name == req.PathValue("session_id") {
			return req.Write(session)
		}
	}
	return types.NewErrHTTP(http.StatusNotFound, "Claude session not found")
}

// Suspend asks the runner to release its session and checkpoint before Substrate stops it.
func (h *ClaudeSessionHandler) Suspend(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrHTTP(http.StatusForbidden, "Administrator access required")
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error { return h.suspend(req) })
}

func (h *ClaudeSessionHandler) suspend(req api.Context) error {
	var instances v1.AgentInstanceList
	if err := req.List(&instances); err != nil {
		return err
	}

	var latest *v1.AgentInstance
	for i := range instances.Items {
		instance := &instances.Items[i]
		if instance.Spec.ClaudeOrder == "" || instance.Spec.ClaudeSession == "" || agentSessionID(instance) != req.PathValue("session_id") {
			continue
		}
		if latest == nil || instance.CreationTimestamp.After(latest.CreationTimestamp.Time) || (instance.CreationTimestamp.Equal(&latest.CreationTimestamp) && instance.Name > latest.Name) {
			latest = instance
		}
	}
	if latest == nil {
		return types.NewErrHTTP(http.StatusNotFound, "Claude session not found")
	}
	if latest.DeletionTimestamp != nil {
		return types.NewErrHTTP(http.StatusConflict, "Session is being deleted")
	}
	if !latest.Spec.Suspended {
		if latest.Status.State != "RUNNING" {
			return types.NewErrHTTP(http.StatusConflict, "Session must be running to suspend")
		}
		latest.Spec.Suspended = true
		if err := req.Update(latest); err != nil {
			return err
		}
	}
	req.WriteHeader(http.StatusAccepted)
	return nil
}

func (h *ClaudeSessionHandler) Delete(req api.Context) error {
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		var options []kclient.ListOption
		if !req.UserIsAdmin() {
			options = append(options, kclient.MatchingFields{"spec.userID": req.User.GetUID()})
		}

		var instances v1.AgentInstanceList
		if err := req.List(&instances, options...); err != nil {
			return err
		}

		found := false
		for i := range instances.Items {
			instance := &instances.Items[i]
			if instance.Spec.ClaudeOrder == "" || instance.Spec.ClaudeSession == "" || agentSessionID(instance) != req.PathValue("session_id") {
				continue
			}
			found = true
			if err := revokeClaudeKey(req, instance.Name); err != nil {
				return err
			}
			if err := req.Delete(instance); err != nil {
				return err
			}
		}
		if !found {
			return types.NewErrHTTP(http.StatusNotFound, "Claude session not found")
		}
		req.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func visibleAgentInstances(req api.Context, instances []v1.AgentInstance) ([]v1.AgentInstance, error) {
	visible := []v1.AgentInstance{}
	for _, instance := range instances {
		_, err := clauderunner.AuthorizedEnvironment(req.Context(), req.GatewayClient, instance.Spec.ClaudeEnvironment, instance.Spec.ClaudeAccount, instance.Spec.UserID)
		if clauderunner.AccessDenied(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		visible = append(visible, instance)
	}
	return visible, nil
}
