package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
)

// AgentIdentityHandler serves the admin API of agent identities at /api/agents. The routes are admin-only, which the
// authorizer enforces: the handler never sees an agent's principal, and gives its callers no rights of their own over
// an agent, not even its creator.
//
// The API names an agent AgentIdentity, because "agent" already means a hosted agent and an agent on a device. Agent
// IDs and user IDs are decimal strings in the API, as users' IDs are elsewhere.
type AgentIdentityHandler struct {
	// readiness decides whether an agent can connect to a vMCP, which the readiness routes report as the connect path
	// decides it, with the grants it selected.
	readiness AgentReadinessSource
}

func NewAgentIdentityHandler(readiness AgentReadinessSource) *AgentIdentityHandler {
	return &AgentIdentityHandler{
		readiness: readiness,
	}
}

// RegisterRoutes registers the routes of the admin API with handle. Their authorization is in pkg/api/authz: they are
// the admin routes under /api/agents, and none of them is on the allowlist of agents.
func (h *AgentIdentityHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/agents", h.List)
	handle("POST /api/agents", h.Create)
	handle("GET /api/agents/{agent_id}", h.Get)
	handle("GET /api/agents/{agent_id}/access", h.Access)
	handle("GET /api/agents/{agent_id}/vmcps", h.ListVMCPReadiness)
	handle("GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness", h.GetVMCPReadiness)
	handle("PUT /api/agents/{agent_id}", h.Update)
	handle("DELETE /api/agents/{agent_id}", h.Delete)
	handle("POST /api/agents/{agent_id}/disable", h.Disable)
	handle("POST /api/agents/{agent_id}/enable", h.Enable)
	handle("GET /api/agents/{agent_id}/api-keys", h.ListAPIKeys)
	handle("POST /api/agents/{agent_id}/api-keys", h.CreateAPIKey)
	handle("GET /api/agents/{agent_id}/api-keys/{key_id}", h.GetAPIKey)
	handle("DELETE /api/agents/{agent_id}/api-keys/{key_id}", h.RevokeAPIKey)
}

// List lists agents in the order they were created. Deleted agents are left out unless includeDeleted=true.
func (h *AgentIdentityHandler) List(req api.Context) error {
	var includeDeleted bool
	if q := req.URL.Query(); q.Has("includeDeleted") {
		var err error
		if includeDeleted, err = strconv.ParseBool(q.Get("includeDeleted")); err != nil {
			return types.NewErrBadRequest("invalid includeDeleted %q: must be true or false", q.Get("includeDeleted"))
		}
	}

	agents, err := req.GatewayClient.ListAgents(req.Context(), includeDeleted)
	if err != nil {
		return err
	}
	// One query counts the keys of every agent. The keys of a deleted agent are all revoked, so it has none.
	keys, err := req.GatewayClient.ListAllAPIKeys(req.Context(), gclient.APIKeyListOptions{OwnerKind: principal.KindAgent})
	if err != nil {
		return err
	}
	counts := map[uint]int{}
	now := time.Now()
	for _, key := range keys {
		if key.AgentID != nil && apiKeyIsActive(key, now) {
			counts[*key.AgentID]++
		}
	}

	items := make([]types.AgentIdentity, 0, len(agents))
	for _, agent := range agents {
		items = append(items, convertAgentIdentity(agent, counts[agent.ID]))
	}
	return req.Write(types.AgentIdentityList{Items: items})
}

// Create creates an active agent. The caller is its creator.
func (h *AgentIdentityHandler) Create(req api.Context) error {
	var manifest types.AgentIdentityManifest
	if err := readAgentIdentityRequest(req, &manifest, false); err != nil {
		return err
	}
	agent, err := agentFromManifest(manifest)
	if err != nil {
		return err
	}
	if err := validateAgentProfiles(agent.Profiles); err != nil {
		return err
	}
	// The creator is the calling admin. It is 0 for a caller with no user row, such as the bootstrap admin.
	agent.CreatorUserID = req.UserID()

	created, err := req.GatewayClient.CreateAgent(adminAuditContext(req), agent)
	if err != nil {
		return agentIdentityError(err)
	}
	return req.WriteCreated(convertAgentIdentity(*created, 0))
}

// Get returns an agent, with the number of its active API keys.
func (h *AgentIdentityHandler) Get(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	return h.writeAgent(req, agent)
}

// Access returns the vMCP profiles, model access policies and skill access rules whose subjects match an agent
// (agentGrants), and the problems of its keys that they leave.
func (h *AgentIdentityHandler) Access(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	access, err := agentGrants(req, agent)
	if err != nil {
		return err
	}

	keys, err := req.GatewayClient.ListAgentAPIKeys(req.Context(), agent.ID, gclient.APIKeyListOptions{})
	if err != nil {
		return err
	}
	access.Problems = accessProblems(keys, access, time.Now())
	return req.Write(access)
}

// agentGrants returns the vMCP profiles, model access policies and skill access rules whose subjects match an agent,
// as the policy matchers evaluate the agent (pkg/subjects), with the names people know them by. An agent matches only
// a subject that names it by ID, never the selector wildcard, a user or a group subject, so no groups are evaluated.
// They are what the agent may use, for itself and on behalf of every user its profiles match.
func agentGrants(req api.Context, agent *gtypes.Agent) (types.AgentIdentityAccess, error) {
	caller := subjects.Caller{Principal: principal.Agent(agent.ID)}

	access := types.AgentIdentityAccess{
		AgentID:             strconv.FormatUint(uint64(agent.ID), 10),
		VMCPProfiles:        []types.AgentIdentityVMCPProfileAccess{},
		ModelAccessPolicies: []types.AgentIdentityPolicyAccess{},
		SkillAccessRules:    []types.AgentIdentityPolicyAccess{},
	}

	vmcps, err := listSharedVMCPs(req)
	if err != nil {
		return access, err
	}
	info := agentInfo(agent)
	for _, vmcp := range vmcps {
		for _, profile := range vmcpconfig.MatchingProfiles(info, vmcp.Spec.Manifest.Profiles) {
			access.VMCPProfiles = append(access.VMCPProfiles, types.AgentIdentityVMCPProfileAccess{
				VMCPID:          vmcp.Name,
				VMCPDisplayName: vmcp.Spec.Manifest.DisplayName,
				ProfileID:       profile.ID,
				ProfileName:     profile.Name,
			})
		}
	}

	var policies v1.ModelAccessPolicyList
	if err := req.List(&policies); err != nil {
		return access, fmt.Errorf("failed to list model access policies: %w", err)
	}
	slices.SortFunc(policies.Items, func(a, b v1.ModelAccessPolicy) int { return strings.Compare(a.Name, b.Name) })
	for _, policy := range policies.Items {
		if !policy.DeletionTimestamp.IsZero() {
			continue
		}
		if caller.Matches(policy.Spec.Manifest.Subjects) {
			access.ModelAccessPolicies = append(access.ModelAccessPolicies, types.AgentIdentityPolicyAccess{
				ID:          policy.Name,
				DisplayName: policy.Spec.Manifest.DisplayName,
			})
		}
	}

	var rules v1.SkillAccessRuleList
	if err := req.List(&rules); err != nil {
		return access, fmt.Errorf("failed to list skill access rules: %w", err)
	}
	slices.SortFunc(rules.Items, func(a, b v1.SkillAccessRule) int { return strings.Compare(a.Name, b.Name) })
	for _, rule := range rules.Items {
		if !rule.DeletionTimestamp.IsZero() {
			continue
		}
		if caller.Matches(rule.Spec.Manifest.Subjects) {
			access.SkillAccessRules = append(access.SkillAccessRules, types.AgentIdentityPolicyAccess{
				ID:          rule.Name,
				DisplayName: rule.Spec.Manifest.DisplayName,
			})
		}
	}
	return access, nil
}

// listSharedVMCPs lists the vMCPs agents can use, sorted by name: shared vMCPs that are not being deleted. A personal
// vMCP grants only its owner, through no profile, and one being deleted grants no one.
func listSharedVMCPs(req api.Context) ([]v1.VMCP, error) {
	var vmcps v1.VMCPList
	if err := req.List(&vmcps); err != nil {
		return nil, fmt.Errorf("failed to list vMCPs: %w", err)
	}
	slices.SortFunc(vmcps.Items, func(a, b v1.VMCP) int { return strings.Compare(a.Name, b.Name) })
	return slices.DeleteFunc(vmcps.Items, func(vmcp v1.VMCP) bool {
		return vmcp.IsPersonal() || !vmcp.DeletionTimestamp.IsZero()
	}), nil
}

// accessProblems reports the scopes of an agent's keys that its policies grant nothing for, which are refused or empty
// rather than failing at setup: with no model access policy that names the agent, every LLM request of a key that can
// use the gateway is answered 403, and with no skill access rule, a key that can use skills lists none.
func accessProblems(keys []gtypes.APIKey, access types.AgentIdentityAccess, now time.Time) []types.AgentIdentityAccessProblem {
	problems := []types.AgentIdentityAccessProblem{}
	active := slices.DeleteFunc(slices.Clone(keys), func(key gtypes.APIKey) bool { return !apiKeyIsActive(key, now) })
	if len(access.ModelAccessPolicies) == 0 && slices.ContainsFunc(active, func(key gtypes.APIKey) bool { return key.CanAccessLLMProxy }) {
		problems = append(problems, types.AgentIdentityAccessProblem{Code: types.AgentProblemNoModelAccessPolicy})
	}
	if len(access.SkillAccessRules) == 0 && slices.ContainsFunc(active, func(key gtypes.APIKey) bool { return key.CanAccessSkills }) {
		problems = append(problems, types.AgentIdentityAccessProblem{Code: types.AgentProblemNoSkillAccessRule})
	}
	return problems
}

// Update replaces the metadata, the agent profiles and the allowed return hosts of an agent. It changes nothing else: an
// agent is disabled and deleted by their own routes, and its creator is never changed.
func (h *AgentIdentityHandler) Update(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}

	var manifest types.AgentIdentityManifest
	if err := readAgentIdentityRequest(req, &manifest, false); err != nil {
		return err
	}
	update, err := agentFromManifest(manifest)
	if err != nil {
		return err
	}
	update.ID = agent.ID
	if err := validateAgentProfiles(update.Profiles); err != nil {
		return err
	}

	updated, err := req.GatewayClient.UpdateAgent(adminAuditContext(req), update)
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, updated)
}

// Disable denies an agent access, and records why. Disabling a disabled agent only updates the reason. Its keys are
// kept, and work once it is enabled again.
func (h *AgentIdentityHandler) Disable(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}

	// The reason is optional, so an empty body disables the agent with none.
	var body types.AgentIdentityDisableRequest
	if err := readAgentIdentityRequest(req, &body, true); err != nil {
		return err
	}

	disabled, err := req.GatewayClient.DisableAgent(adminAuditContext(req), agent.ID, strings.TrimSpace(body.Reason))
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, disabled)
}

// Enable restores the access of a disabled agent. Enabling an agent that is not disabled changes nothing.
func (h *AgentIdentityHandler) Enable(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}

	enabled, err := req.GatewayClient.EnableAgent(adminAuditContext(req), agent.ID)
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, enabled)
}

// Delete deletes an agent and revokes its keys. The agent is kept for audit history, and listed with includeDeleted,
// but every other route answers 404 for it, so deleting a deleted agent is 404 too.
func (h *AgentIdentityHandler) Delete(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteAgent(adminAuditContext(req), agent.ID); err != nil {
		return agentIdentityError(err)
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// ListAPIKeys lists the keys of an agent that are not revoked, or all of them with show_revoked=true, as the routes
// of users' keys do.
func (h *AgentIdentityHandler) ListAPIKeys(req api.Context) error {
	// The agent is looked up first, because the client lists no keys, rather than failing, for an agent that is
	// missing.
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}

	var showRevoked bool
	if q := req.URL.Query(); q.Has("show_revoked") {
		if showRevoked, err = strconv.ParseBool(q.Get("show_revoked")); err != nil {
			return types.NewErrBadRequest("invalid show_revoked %q: must be true or false", q.Get("show_revoked"))
		}
	}

	keys, err := req.GatewayClient.ListAgentAPIKeys(req.Context(), agent.ID, gclient.APIKeyListOptions{ShowRevoked: showRevoked})
	if err != nil {
		return err
	}
	items := make([]types.AgentAPIKey, 0, len(keys))
	for _, key := range keys {
		items = append(items, convertAgentAPIKey(key))
	}
	return req.Write(types.AgentAPIKeyList{Items: items})
}

// CreateAPIKey creates a key for an agent, and returns it with its secret, which is only ever shown here. A disabled
// agent gets a key, which works once the agent is enabled.
func (h *AgentIdentityHandler) CreateAPIKey(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}

	// A request that names a scope an agent's key never has, such as canAccessAPI, is refused rather than having it
	// ignored, so that a caller is never told a key was made with a scope that it does not have.
	var body types.AgentAPIKeyCreateRequest
	if err := readAgentIdentityRequest(req, &body, false); err != nil {
		return err
	}

	created, err := req.GatewayClient.CreateAgentAPIKey(adminAuditContext(req), agent.ID, gtypes.AgentAPIKeyCreateRequest{
		Name:              body.Name,
		Description:       body.Description,
		ExpiresAt:         timeFromAPI(body.ExpiresAt),
		CanAccessSkills:   body.CanAccessSkills,
		CanAccessLLMProxy: body.CanAccessLLMProxy,
		MCPServerIDs:      body.MCPServerIDs,
	})
	if err != nil {
		return agentIdentityError(err)
	}
	return req.WriteCreated(types.AgentAPIKeyCreateResponse{
		AgentAPIKey: convertAgentAPIKey(created.APIKey),
		Key:         created.Key,
	})
}

// GetAPIKey returns a key of an agent. A key of another owner is 404, not found as though it were the agent's.
func (h *AgentIdentityHandler) GetAPIKey(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	key, err := h.apiKey(req, agent)
	if err != nil {
		return err
	}
	return req.Write(convertAgentAPIKey(*key))
}

// RevokeAPIKey revokes a key of an agent. Revoking a revoked key succeeds and keeps its first revocation time.
func (h *AgentIdentityHandler) RevokeAPIKey(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	// The key is looked up first, because the client revokes nothing, and succeeds, for a key of another owner.
	key, err := h.apiKey(req, agent)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.RevokeAgentAPIKey(adminAuditContext(req), agent.ID, key.ID); err != nil {
		return agentIdentityError(err)
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// pathAgent returns the agent the request's path names. A deleted or unknown agent, and an ID that is not one, are 404.
func pathAgent(req api.Context) (*gtypes.Agent, error) {
	id, err := strconv.ParseUint(req.PathValue("agent_id"), 10, 64)
	if err != nil {
		return nil, types.NewErrNotFound("agent not found")
	}
	agent, err := req.GatewayClient.GetAgent(req.Context(), uint(id))
	if err != nil {
		return nil, agentIdentityError(err)
	}
	return agent, nil
}

// apiKey returns the key the request's path names, if it is a key of agent.
func (h *AgentIdentityHandler) apiKey(req api.Context, agent *gtypes.Agent) (*gtypes.APIKey, error) {
	id, err := strconv.ParseUint(req.PathValue("key_id"), 10, 64)
	if err != nil {
		return nil, types.NewErrNotFound("API key not found")
	}
	key, err := req.GatewayClient.GetAgentAPIKey(req.Context(), agent.ID, uint(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("API key not found")
	} else if err != nil {
		return nil, err
	}
	return key, nil
}

// writeAgent writes an agent with its active key count.
func (h *AgentIdentityHandler) writeAgent(req api.Context, agent *gtypes.Agent) error {
	keys, err := req.GatewayClient.ListAgentAPIKeys(req.Context(), agent.ID, gclient.APIKeyListOptions{})
	if err != nil {
		return err
	}
	return req.Write(convertAgentIdentity(*agent, countActiveAPIKeys(keys)))
}

// agentFromManifest returns the agent whose metadata, profiles and allowed return hosts manifest describes.
func agentFromManifest(manifest types.AgentIdentityManifest) (gtypes.Agent, error) {
	if strings.TrimSpace(manifest.DisplayName) == "" {
		return gtypes.Agent{}, types.NewErrBadRequest("displayName is required")
	}

	profiles := slices.Clone(manifest.Profiles)
	for i := range profiles {
		profiles[i].Name = strings.TrimSpace(profiles[i].Name)
	}
	hosts, err := agentdelegation.NormalizeAllowedReturnHosts(manifest.AllowedReturnHosts)
	if err != nil {
		return gtypes.Agent{}, types.NewErrBadRequest("invalid allowedReturnHosts: %v", err)
	}
	return gtypes.Agent{
		DisplayName:        strings.TrimSpace(manifest.DisplayName),
		Description:        manifest.Description,
		Profiles:           profiles,
		AllowedReturnHosts: hosts,
	}, nil
}

// validateAgentProfiles checks the agent profiles an agent is given (agentprofile.Validate).
func validateAgentProfiles(profiles []types.AgentProfile) error {
	if err := agentprofile.Validate(profiles); err != nil {
		return types.NewErrBadRequest("invalid profiles: %v", err)
	}
	return nil
}

// readAgentIdentityRequest reads a request body into obj, and answers 400 for one that is not valid. Unknown fields
// are refused, so that a field that is misspelled, or that does not exist for agents, is not silently ignored. An
// empty body is valid only if optional.
func readAgentIdentityRequest(req api.Context, obj any, optional bool) error {
	data, err := req.Body()
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		if optional {
			return nil
		}
		return types.NewErrBadRequest("invalid request body: %v", io.EOF)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(obj); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return types.NewErrBadRequest("invalid request body: unexpected data after the JSON value")
	}
	return nil
}

// agentIdentityError maps an error of the gateway client onto the status the API answers with. Any other error is a
// 500.
func agentIdentityError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return types.NewErrNotFound("agent not found")
	case errors.Is(err, gclient.ErrInvalidAgentAPIKeyRequest), errors.Is(err, gclient.ErrInvalidAgentMetadata):
		return types.NewErrBadRequest("%s", err.Error())
	default:
		return err
	}
}

func convertAgentIdentity(agent gtypes.Agent, activeAPIKeyCount int) types.AgentIdentity {
	identity := types.AgentIdentity{
		ID:                 strconv.FormatUint(uint64(agent.ID), 10),
		Created:            *types.NewTime(agent.CreatedAt),
		Type:               "agentidentity",
		DisplayName:        agent.DisplayName,
		Description:        agent.Description,
		Profiles:           agent.Profiles,
		AllowedReturnHosts: agent.AllowedReturnHosts,
		Status:             agent.Status(),
		DisabledReason:     agent.DisabledReason,
		ActiveAPIKeyCount:  activeAPIKeyCount,
	}
	if agent.CreatorUserID != 0 {
		identity.CreatorUserID = strconv.FormatUint(uint64(agent.CreatorUserID), 10)
	}
	if agent.DisabledAt != nil {
		identity.DisabledAt = types.NewTime(*agent.DisabledAt)
	}
	if agent.DeletedAt.Valid {
		identity.Deleted = types.NewTime(agent.DeletedAt.Time)
	}
	return identity
}

// convertAgentAPIKey converts a key of an agent. It has no field for the API and device scan scopes, which an agent's
// key never has.
func convertAgentAPIKey(key gtypes.APIKey) types.AgentAPIKey {
	converted := types.AgentAPIKey{
		ID:                key.ID,
		Name:              key.Name,
		Description:       key.Description,
		CanAccessSkills:   key.CanAccessSkills,
		CanAccessLLMProxy: key.CanAccessLLMProxy,
		MCPServerIDs:      slices.Clone(key.MCPServerIDs),
		CreatedAt:         *types.NewTime(key.CreatedAt),
		LastUsedAt:        optionalAPITime(key.LastUsedAt),
		ExpiresAt:         optionalAPITime(key.ExpiresAt),
		RevokedAt:         optionalAPITime(key.RevokedAt),
	}
	if key.AgentID != nil {
		converted.AgentID = strconv.FormatUint(uint64(*key.AgentID), 10)
	}
	if converted.MCPServerIDs == nil {
		// Over HTTP, an empty list must not look like a missing one.
		converted.MCPServerIDs = []string{}
	}
	return converted
}

// apiKeyIsActive reports whether key neither is revoked nor has expired at now.
func apiKeyIsActive(key gtypes.APIKey, now time.Time) bool {
	return key.RevokedAt == nil && (key.ExpiresAt == nil || key.ExpiresAt.After(now))
}

func countActiveAPIKeys(keys []gtypes.APIKey) int {
	now := time.Now()
	var count int
	for _, key := range keys {
		if apiKeyIsActive(key, now) {
			count++
		}
	}
	return count
}

func timeFromAPI(t *types.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t.Time
}

// adminAuditContext returns the request's context with the calling administrator recorded as the actor of the admin
// audit events that gateway changes made with it write. The routes that use it are administrators' alone, so the caller
// is one; the bootstrap administrator is recorded under its own ID.
func adminAuditContext(req api.Context) context.Context {
	return gclient.WithAdminUser(req.Context(), req.User)
}
