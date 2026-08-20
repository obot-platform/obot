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
	PoolID                            string `json:"poolID,omitempty"`
}

// Validate checks the request as a whole, so a caller gets one answer about
// whether what it sent is usable rather than two checks in sequence.
//
// hostedAgentID belongs to the request rather than to the manifest: an instance
// names the agent it is created from once, at creation, and never carries it
// afterwards. So it is checked here instead of in the manifest's own rules,
// which apply equally to an update where it is absent by design.
func (r hostedAgentInstanceRequest) Validate() error {
	if r.HostedAgentID == "" {
		return fmt.Errorf("hostedAgentID is required")
	}
	return r.HostedAgentInstanceManifest.Validate()
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

	icons, err := newIconResolver(req)
	if err != nil {
		return err
	}

	items := make([]types.HostedAgentInstance, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, icons.apply(convertHostedAgentInstance(item), item.Spec.HostedAgentName))
	}

	return req.Write(types.HostedAgentInstanceList{Items: items})
}

func (h *HostedAgentInstanceHandler) Get(req api.Context) error {
	var instance v1.HostedAgentInstance
	if err := req.Get(&instance, req.PathValue("hosted_agent_instance_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent instance: %w", err)
	}

	icons, err := newIconResolver(req)
	if err != nil {
		return err
	}

	return req.Write(icons.apply(convertHostedAgentInstance(instance), instance.Spec.HostedAgentName))
}

func (h *HostedAgentInstanceHandler) Create(req api.Context) error {
	var body hostedAgentInstanceRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("failed to read hosted agent instance manifest: %v", err)
	}

	if err := body.Validate(); err != nil {
		return types.NewErrBadRequest("invalid hosted agent instance request: %v", err)
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

	// Checked here as well as reported on the agent, because the UI's decision
	// not to offer a launch button is a courtesy and not a control: a direct
	// request would otherwise produce a sandbox that cannot serve its first
	// request, and an error the user has no way to act on.
	availability, err := newAgentAvailability(req.Context(), req.Storage, req.Namespace())
	if err != nil {
		return err
	}
	if reasons := availability.reasons(req.Context(), agent.Spec.Manifest); len(reasons) > 0 {
		return types.NewErrBadRequest("%s cannot be launched here: %s",
			agent.Spec.Manifest.Name, strings.Join(reasons, "; "))
	}

	if body.PoolID != "" {
		if err := requirePoolAccess(req, body.PoolID); err != nil {
			return err
		}
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
		GenerateName: system.HostedAgentInstancePrefix,
		Namespace:    req.Namespace(),
		Spec: v1.HostedAgentInstanceSpec{
			UserID:          req.User.GetUID(),
			HostedAgentName: agent.Name,
			PoolID:          body.PoolID,
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
		Name:      req.PathValue("hosted_agent_instance_id"),
		Namespace: req.Namespace(),
	})
}

func convertHostedAgentInstance(instance v1.HostedAgentInstance) types.HostedAgentInstance {
	return types.HostedAgentInstance{
		Metadata:                    MetadataFrom(&instance),
		HostedAgentInstanceManifest: instance.Spec.Manifest,
		HostedAgentID:               instance.Spec.HostedAgentName,
		UserID:                      instance.Spec.UserID,
		PoolID:                      instance.Spec.PoolID,
		Status: types.HostedAgentInstanceStatus{
			State:             instance.Status.State,
			URL:               instance.Status.URL,
			Error:             instance.Status.Error,
			Reason:            instance.Status.Reason,
			Message:           instance.Status.Message,
			ObservedRevision:  instance.Status.ObservedRevision,
			LastObservedTime:  v1.NewTime(instance.Status.LastObservedTime),
			BackendID:         instance.Status.BackendID,
			BackendGeneration: instance.Status.BackendGeneration,
		},
	}
}

// iconResolver resolves an instance's icon through the chain a client would
// otherwise have to walk itself: the instance, then its agent, then the
// harness the agent runs on.
//
// Agents and harnesses are listed once rather than fetched per instance, so
// rendering a page of instances costs two reads instead of two per row.
type iconResolver struct {
	agents    map[string]types.HostedAgentManifest
	harnesses map[string]types.HarnessManifest
}

func newIconResolver(req api.Context) (*iconResolver, error) {
	var agents v1.HostedAgentList
	if err := req.List(&agents); err != nil {
		return nil, fmt.Errorf("failed to list hosted agents: %w", err)
	}
	var harnesses v1.HarnessList
	if err := req.List(&harnesses); err != nil {
		return nil, fmt.Errorf("failed to list harnesses: %w", err)
	}

	resolver := &iconResolver{
		agents:    make(map[string]types.HostedAgentManifest, len(agents.Items)),
		harnesses: make(map[string]types.HarnessManifest, len(harnesses.Items)),
	}
	for _, agent := range agents.Items {
		resolver.agents[agent.Name] = agent.Spec.Manifest
	}
	for _, harness := range harnesses.Items {
		resolver.harnesses[harness.Name] = harness.Spec.Manifest
	}
	return resolver, nil
}

func (r *iconResolver) apply(instance types.HostedAgentInstance, agentID string) types.HostedAgentInstance {
	// The instance's own icon wins: a user who chose one meant it.
	instance.ResolvedIcon, instance.ResolvedIconDark = instance.Icon, instance.Icon

	agent, ok := r.agents[agentID]
	if !ok {
		return instance
	}
	if instance.ResolvedIcon == "" {
		instance.ResolvedIcon, instance.ResolvedIconDark = agent.Icon, agent.IconDark
	}
	if instance.ResolvedIcon == "" {
		harness := r.harnesses[agent.HarnessID]
		instance.ResolvedIcon, instance.ResolvedIconDark = harness.Icon, harness.IconDark
	}
	// A dark variant is optional everywhere; falling back keeps a client from
	// having to repeat this rule.
	if instance.ResolvedIconDark == "" {
		instance.ResolvedIconDark = instance.ResolvedIcon
	}
	return instance
}
