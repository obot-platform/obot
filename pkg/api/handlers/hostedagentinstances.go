package handlers

import (
	"fmt"
	"strings"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/hostedagentaccessrule"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type HostedAgentInstanceHandler struct {
	accessRuleHelper *hostedagentaccessrule.Helper
	// The three helpers below check the resources a user attaches to their own
	// instance, one per kind. Each kind has its own access model, so there is no
	// single helper to ask.
	acrHelper   *accesscontrolrule.Helper
	skillHelper *skillaccessrule.Helper
	mapHelper   *modelaccesspolicy.Helper
}

func NewHostedAgentInstanceHandler(
	accessRuleHelper *hostedagentaccessrule.Helper,
	acrHelper *accesscontrolrule.Helper,
	skillHelper *skillaccessrule.Helper,
	mapHelper *modelaccesspolicy.Helper,
) *HostedAgentInstanceHandler {
	return &HostedAgentInstanceHandler{
		accessRuleHelper: accessRuleHelper,
		acrHelper:        acrHelper,
		skillHelper:      skillHelper,
		mapHelper:        mapHelper,
	}
}

type hostedAgentInstanceRequest struct {
	types.HostedAgentInstanceManifest `json:",inline"`
	HostedAgentID                     string `json:"hostedAgentID,omitempty"`
}

func (h *HostedAgentInstanceHandler) List(req api.Context) error {
	selector := kclient.MatchingFields{"spec.userID": req.User.GetUID()}
	if hostedAgentID := req.URL.Query().Get("hosted_agent_id"); hostedAgentID != "" {
		selector["spec.hostedAgentName"] = hostedAgentID
	}

	var list v1.HostedAgentInstanceList
	if err := req.List(&list, selector); err != nil {
		return fmt.Errorf("failed to list hosted agent instances: %w", err)
	}

	items := make([]types.HostedAgentInstance, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertHostedAgentInstance(item))
	}

	return req.Write(types.HostedAgentInstanceList{Items: items})
}

func (h *HostedAgentInstanceHandler) Get(req api.Context) error {
	var instance v1.HostedAgentInstance
	if err := req.Get(&instance, req.PathValue("hosted_agent_instance_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent instance: %w", err)
	}

	return req.Write(convertHostedAgentInstance(instance))
}

func (h *HostedAgentInstanceHandler) Create(req api.Context) error {
	var body hostedAgentInstanceRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("failed to read hosted agent instance manifest: %v", err)
	}

	if body.HostedAgentID == "" {
		return types.NewErrBadRequest("hostedAgentID is required")
	}

	if err := body.Validate(); err != nil {
		return types.NewErrBadRequest("invalid hosted agent instance manifest: %v", err)
	}

	var agent v1.HostedAgent
	if err := req.Get(&agent, body.HostedAgentID); apierrors.IsNotFound(err) {
		return types.NewErrBadRequest("hosted agent %s not found", body.HostedAgentID)
	} else if err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	// This route carries no hosted agent ID in its path, so the authorizer cannot
	// gate it. Check access here instead.
	hasAccess, err := h.accessRuleHelper.UserHasAccessToHostedAgent(req.User, &agent)
	if err != nil {
		return fmt.Errorf("failed to check access to hosted agent %s: %w", agent.Name, err)
	}
	if !hasAccess {
		return types.NewErrNotFound("hosted agent %s not found", body.HostedAgentID)
	}

	if !agent.Spec.Manifest.PerUser {
		return types.NewErrBadRequest("hosted agent %s is shared and does not support instances", agent.Name)
	}

	var existing v1.HostedAgentInstanceList
	if err := req.List(&existing, kclient.MatchingFields{
		"spec.userID":          req.User.GetUID(),
		"spec.hostedAgentName": agent.Name,
	}); err != nil {
		return fmt.Errorf("failed to list hosted agent instances: %w", err)
	}

	if maxInstances := agent.Spec.Manifest.MaxInstancesPerUser; maxInstances > 0 && len(existing.Items) >= maxInstances {
		return types.NewErrBadRequest("hosted agent %s allows at most %d instances per user", agent.Name, maxInstances)
	}

	manifest := body.HostedAgentInstanceManifest
	manifest.Answers = agent.Spec.Manifest.ApplyAnswerDefaults(manifest.Answers)
	if err := h.validateInstanceAgainstAgent(req, manifest, agent.Spec.Manifest); err != nil {
		return err
	}

	instance := v1.HostedAgentInstance{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.HostedAgentInstancePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.HostedAgentInstanceSpec{
			UserID:          req.User.GetUID(),
			HostedAgentName: agent.Name,
			Manifest:        manifest,
		},
	}

	if err := req.Create(&instance); err != nil {
		return fmt.Errorf("failed to create hosted agent instance: %w", err)
	}

	return req.WriteCreated(convertHostedAgentInstance(instance))
}

func (h *HostedAgentInstanceHandler) Update(req api.Context) error {
	var manifest types.HostedAgentInstanceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read hosted agent instance manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid hosted agent instance manifest: %v", err)
	}

	var instance v1.HostedAgentInstance
	if err := req.Get(&instance, req.PathValue("hosted_agent_instance_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent instance: %w", err)
	}

	// Re-check against the agent, otherwise an update could smuggle in answers or
	// resources that create would have rejected.
	var agent v1.HostedAgent
	if err := req.Get(&agent, instance.Spec.HostedAgentName); apierrors.IsNotFound(err) {
		return types.NewErrBadRequest("hosted agent %s not found", instance.Spec.HostedAgentName)
	} else if err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	manifest.Answers = agent.Spec.Manifest.ApplyAnswerDefaults(manifest.Answers)
	if err := h.validateInstanceAgainstAgent(req, manifest, agent.Spec.Manifest); err != nil {
		return err
	}

	instance.Spec.Manifest = manifest
	if err := req.Update(&instance); err != nil {
		return fmt.Errorf("failed to update hosted agent instance: %w", err)
	}

	return req.Write(convertHostedAgentInstance(instance))
}

// validateInstanceAgainstAgent enforces the agent's question schema, its
// user-defined resource toggles, and the user's access to every resource they
// attached. Cron parsing happens here rather than in apiclient/types so that
// module stays dependency free.
//
// Resources are checked rather than trusted: the UI only ever offers resources
// the user can reach, but the API is reachable directly, so a request can name
// any ID.
//
// Errors follow the two conventions already in the codebase: a missing resource
// is a bad request naming the ID (as in accesscontrolrules.go), and a resource
// the user cannot use is forbidden (as in llmproxy.go and mcp.go). Existence is
// not hidden, matching how those same paths already behave.
func (h *HostedAgentInstanceHandler) validateInstanceAgainstAgent(req api.Context, manifest types.HostedAgentInstanceManifest, agent types.HostedAgentManifest) error {
	if err := manifest.ValidateAgainstAgent(agent); err != nil {
		return types.NewErrBadRequest("%v", err)
	}

	for key, answer := range agent.ScheduleAnswers(manifest.Answers) {
		if !gronx.IsValid(answer) {
			return types.NewErrBadRequest("answer for %s: must be a valid cron expression", key)
		}
	}

	// Only reached when the agent allows the kind, which ValidateAgainstAgent
	// has already established.
	if err := h.checkUserMCPServerAccess(req, manifest.MCPServers); err != nil {
		return err
	}
	if err := h.checkUserSkillAccess(req, manifest.Skills); err != nil {
		return err
	}
	return h.checkUserModelAccess(req, manifest.Models)
}

// checkUserMCPServerAccess validates MCP gateway IDs, which are polymorphic: an
// ID may name a catalog entry, a server, a server instance, or a system server.
// CheckMCPIDAccess is the same resolution the gateway itself applies at connect
// time, so this cannot drift from it.
func (h *HostedAgentInstanceHandler) checkUserMCPServerAccess(req api.Context, ids []string) error {
	for _, id := range ids {
		// CheckMCPIDAccess surfaces a missing object as a NotFound error rather
		// than a false, so an unknown ID has to be mapped here or it would become
		// a 500.
		hasAccess, err := authz.CheckMCPIDAccess(req.Context(), req.Storage, h.acrHelper, req.User, id)
		if apierrors.IsNotFound(err) {
			return types.NewErrBadRequest("MCP server %s not found", id)
		} else if err != nil {
			return fmt.Errorf("failed to check access to MCP server %s: %w", id, err)
		}
		if !hasAccess {
			return types.NewErrForbidden("you do not have access to MCP server %s", id)
		}
	}

	return nil
}

// checkUserSkillAccess loads each skill so its repo ID is known. Passing an
// empty repo ID to the helper would skip repository-granted access and wrongly
// deny a user who was granted a whole repository.
func (h *HostedAgentInstanceHandler) checkUserSkillAccess(req api.Context, ids []string) error {
	for _, id := range ids {
		var skill v1.Skill
		if err := req.Get(&skill, id); apierrors.IsNotFound(err) {
			return types.NewErrBadRequest("skill %s not found", id)
		} else if err != nil {
			return fmt.Errorf("failed to get skill %s: %w", id, err)
		}

		hasAccess, err := h.skillHelper.UserHasAccessToSkill(req.User, &skill)
		if err != nil {
			return fmt.Errorf("failed to check access to skill %s: %w", id, err)
		}
		if !hasAccess {
			return types.NewErrForbidden("you do not have access to skill %s", id)
		}
	}

	return nil
}

// checkUserModelAccess resolves each reference to a concrete model before
// checking it. Model access policies are keyed by real model IDs, so an
// obot://<alias> reference would never match and would always be denied.
func (h *HostedAgentInstanceHandler) checkUserModelAccess(req api.Context, ids []string) error {
	for _, id := range ids {
		modelID, err := resolveModelReference(req, id)
		if err != nil {
			return err
		}

		hasAccess, err := h.mapHelper.UserHasAccessToModel(req.User, modelID)
		if err != nil {
			return fmt.Errorf("failed to check access to model %s: %w", id, err)
		}
		if !hasAccess {
			return types.NewErrForbidden("you do not have access to model %s", id)
		}
	}

	return nil
}

// resolveModelReference turns a model reference into a concrete model ID. It
// accepts a model ID, an alias name, or an obot://<alias> reference, mirroring
// what the LLM proxy accepts. Wildcards are rejected: they are a way to write a
// policy, not a model an agent can be pointed at.
func resolveModelReference(req api.Context, ref string) (string, error) {
	if ref == "" {
		return "", types.NewErrBadRequest("model reference cannot be empty")
	}
	if strings.Contains(ref, "*") {
		return "", types.NewErrBadRequest("model %s is a pattern; name a specific model", ref)
	}

	name := strings.TrimPrefix(ref, types.DefaultModelAliasRefPrefix)

	obj, err := alias.GetFromScope(req.Context(), req.Storage, "Model", req.Namespace(), name)
	if apierrors.IsNotFound(err) {
		return "", types.NewErrBadRequest("model %s not found", ref)
	} else if err != nil {
		return "", fmt.Errorf("failed to resolve model %s: %w", ref, err)
	}

	switch m := obj.(type) {
	case *v1.DefaultModelAlias:
		if m.Spec.Manifest.Model == "" {
			return "", types.NewErrBadRequest("model alias %s is not configured", ref)
		}
		var model v1.Model
		if err := alias.Get(req.Context(), req.Storage, &model, req.Namespace(), m.Spec.Manifest.Model); apierrors.IsNotFound(err) {
			return "", types.NewErrBadRequest("model alias %s points at a missing model", ref)
		} else if err != nil {
			return "", fmt.Errorf("failed to resolve model alias %s: %w", ref, err)
		}
		return model.Name, nil
	case *v1.Model:
		return m.Name, nil
	}

	return "", types.NewErrBadRequest("model %s not found", ref)
}

func (h *HostedAgentInstanceHandler) Delete(req api.Context) error {
	return req.Delete(&v1.HostedAgentInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("hosted_agent_instance_id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertHostedAgentInstance(instance v1.HostedAgentInstance) types.HostedAgentInstance {
	return types.HostedAgentInstance{
		Metadata:                    MetadataFrom(&instance),
		HostedAgentInstanceManifest: instance.Spec.Manifest,
		HostedAgentID:               instance.Spec.HostedAgentName,
		UserID:                      instance.Spec.UserID,
		Status:                      instance.Status.HostedAgentInstanceStatus,
	}
}
