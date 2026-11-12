package handlers

import (
	"bytes"
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
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"gorm.io/gorm"
)

// AgentIdentityHandler serves the admin API of agent identities at /api/agents. The routes are admin-only, which the
// authorizer enforces: the handler never sees an agent's principal, and gives its callers no rights of their own over
// an agent, not even its creator.
//
// The API names an agent AgentIdentity, because "agent" already means a hosted agent and an agent on a device. Agent
// IDs and user IDs are decimal strings in the API, as users' IDs are elsewhere.
type AgentIdentityHandler struct {
	// enabled is the server's EnableAgentIdentities option. While it is unset, every route answers 404 as though
	// agents did not exist.
	enabled bool
	// authEnabled is whether the server authenticates callers. Agents authenticate only with authentication, so
	// without it the routes refuse, rather than create agents that could never connect.
	authEnabled bool
}

func NewAgentIdentityHandler(enabled, authEnabled bool) *AgentIdentityHandler {
	return &AgentIdentityHandler{
		enabled:     enabled,
		authEnabled: authEnabled,
	}
}

// RegisterRoutes registers the routes of the admin API with handle. Their authorization is in pkg/api/authz: they are
// the admin routes under /api/agents, and none of them is on the allowlist of agents.
func (h *AgentIdentityHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/agents", h.List)
	handle("POST /api/agents", h.Create)
	handle("GET /api/agents/{agent_id}", h.Get)
	handle("GET /api/agents/{agent_id}/access", h.Access)
	handle("PUT /api/agents/{agent_id}", h.Update)
	handle("DELETE /api/agents/{agent_id}", h.Delete)
	handle("POST /api/agents/{agent_id}/disable", h.Disable)
	handle("POST /api/agents/{agent_id}/enable", h.Enable)
	handle("GET /api/agents/{agent_id}/api-keys", h.ListAPIKeys)
	handle("POST /api/agents/{agent_id}/api-keys", h.CreateAPIKey)
	handle("GET /api/agents/{agent_id}/api-keys/{key_id}", h.GetAPIKey)
	handle("DELETE /api/agents/{agent_id}/api-keys/{key_id}", h.RevokeAPIKey)
}

// check answers whether agent identities are available. It must come first in every route.
func (h *AgentIdentityHandler) check() error {
	if !h.enabled {
		return types.NewErrNotFound("not found")
	}
	if !h.authEnabled {
		return types.NewErrHTTP(http.StatusNotImplemented, "agent identities require authentication to be enabled: agents authenticate with API keys, which are only checked when authentication is enabled")
	}
	return nil
}

// List lists agents in the order they were created. Deleted agents are left out unless includeDeleted=true.
func (h *AgentIdentityHandler) List(req api.Context) error {
	if err := h.check(); err != nil {
		return err
	}

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
	if err := h.check(); err != nil {
		return err
	}

	var manifest types.AgentIdentityManifest
	if err := readAgentIdentityRequest(req, &manifest, false); err != nil {
		return err
	}
	agent, err := agentFromManifest(manifest)
	if err != nil {
		return err
	}
	// The creator is the calling admin. It is 0 for a caller with no user row, such as the bootstrap admin.
	agent.CreatorUserID = req.UserID()

	created, err := req.GatewayClient.CreateAgent(req.Context(), agent)
	if err != nil {
		return agentIdentityError(err)
	}
	return req.WriteCreated(convertAgentIdentity(*created, 0))
}

// Get returns an agent, with the number of its active API keys.
func (h *AgentIdentityHandler) Get(req api.Context) error {
	agent, err := h.agent(req)
	if err != nil {
		return err
	}
	return h.writeAgent(req, agent)
}

// Access returns the vMCP profiles, model access policies and skill access rules whose subjects match an agent, and
// how each matches, as the policy matchers evaluate the agent (pkg/subjects). An agent matches only its own ID, the
// agent wildcard and the wildcard for everyone, never a user or group subject, so no groups are evaluated.
func (h *AgentIdentityHandler) Access(req api.Context) error {
	agent, err := h.agent(req)
	if err != nil {
		return err
	}
	caller := subjects.Caller{Principal: principal.Agent(agent.ID)}

	access := types.AgentIdentityAccess{
		AgentID:             strconv.FormatUint(uint64(agent.ID), 10),
		VMCPProfiles:        []types.AgentIdentityVMCPProfileAccess{},
		ModelAccessPolicies: []types.AgentIdentityPolicyAccess{},
		SkillAccessRules:    []types.AgentIdentityPolicyAccess{},
	}

	var vmcps v1.VMCPList
	if err := req.List(&vmcps); err != nil {
		return fmt.Errorf("failed to list vMCPs: %w", err)
	}
	slices.SortFunc(vmcps.Items, func(a, b v1.VMCP) int { return strings.Compare(a.Name, b.Name) })
	for _, vmcp := range vmcps.Items {
		// A personal vMCP grants only its owner, through no profile, and one being deleted grants no one.
		if vmcp.IsPersonal() || !vmcp.DeletionTimestamp.IsZero() {
			continue
		}
		for _, profile := range vmcp.Spec.Manifest.Profiles {
			if match, ok := agentSubjectMatch(caller.Best(profile.Subjects)); ok {
				access.VMCPProfiles = append(access.VMCPProfiles, types.AgentIdentityVMCPProfileAccess{
					VMCPID:          vmcp.Name,
					VMCPDisplayName: vmcp.Spec.Manifest.DisplayName,
					ProfileID:       profile.ID,
					ProfileName:     profile.Name,
					Match:           match,
				})
			}
		}
	}

	var policies v1.ModelAccessPolicyList
	if err := req.List(&policies); err != nil {
		return fmt.Errorf("failed to list model access policies: %w", err)
	}
	slices.SortFunc(policies.Items, func(a, b v1.ModelAccessPolicy) int { return strings.Compare(a.Name, b.Name) })
	for _, policy := range policies.Items {
		if !policy.DeletionTimestamp.IsZero() {
			continue
		}
		if match, ok := agentSubjectMatch(caller.Best(policy.Spec.Manifest.Subjects)); ok {
			access.ModelAccessPolicies = append(access.ModelAccessPolicies, types.AgentIdentityPolicyAccess{
				ID:          policy.Name,
				DisplayName: policy.Spec.Manifest.DisplayName,
				Match:       match,
			})
		}
	}

	var rules v1.SkillAccessRuleList
	if err := req.List(&rules); err != nil {
		return fmt.Errorf("failed to list skill access rules: %w", err)
	}
	slices.SortFunc(rules.Items, func(a, b v1.SkillAccessRule) int { return strings.Compare(a.Name, b.Name) })
	for _, rule := range rules.Items {
		if !rule.DeletionTimestamp.IsZero() {
			continue
		}
		if match, ok := agentSubjectMatch(caller.Best(rule.Spec.Manifest.Subjects)); ok {
			access.SkillAccessRules = append(access.SkillAccessRules, types.AgentIdentityPolicyAccess{
				ID:          rule.Name,
				DisplayName: rule.Spec.Manifest.DisplayName,
				Match:       match,
			})
		}
	}

	return req.Write(access)
}

// agentSubjectMatch returns how a policy whose strongest match of an agent is match names the agent, and false if it
// does not name it. An agent is in no group, so a group match is not one of an agent's, and is reported as none.
func agentSubjectMatch(match subjects.Match) (types.SubjectMatch, bool) {
	switch match {
	case subjects.MatchExplicit:
		return types.SubjectMatchExplicit, true
	case subjects.MatchWildcardKind:
		return types.SubjectMatchKindWildcard, true
	case subjects.MatchWildcardAll:
		return types.SubjectMatchWildcard, true
	}
	return "", false
}

// Update replaces the metadata of an agent. It changes nothing else: an agent is disabled and deleted by their own
// routes, and its creator is never changed.
func (h *AgentIdentityHandler) Update(req api.Context) error {
	agent, err := h.agent(req)
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

	updated, err := req.GatewayClient.UpdateAgent(req.Context(), update)
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, updated)
}

// Disable denies an agent access, and records why. Disabling a disabled agent only updates the reason. Its keys are
// kept, and work once it is enabled again.
func (h *AgentIdentityHandler) Disable(req api.Context) error {
	agent, err := h.agent(req)
	if err != nil {
		return err
	}

	// The reason is optional, so an empty body disables the agent with none.
	var body types.AgentIdentityDisableRequest
	if err := readAgentIdentityRequest(req, &body, true); err != nil {
		return err
	}

	disabled, err := req.GatewayClient.DisableAgent(req.Context(), agent.ID, strings.TrimSpace(body.Reason))
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, disabled)
}

// Enable restores the access of a disabled agent. Enabling an agent that is not disabled changes nothing.
func (h *AgentIdentityHandler) Enable(req api.Context) error {
	agent, err := h.agent(req)
	if err != nil {
		return err
	}

	enabled, err := req.GatewayClient.EnableAgent(req.Context(), agent.ID)
	if err != nil {
		return agentIdentityError(err)
	}
	return h.writeAgent(req, enabled)
}

// Delete deletes an agent and revokes its keys. The agent is kept for audit history, and listed with includeDeleted,
// but every other route answers 404 for it, so deleting a deleted agent is 404 too.
func (h *AgentIdentityHandler) Delete(req api.Context) error {
	agent, err := h.agent(req)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteAgent(req.Context(), agent.ID); err != nil {
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
	agent, err := h.agent(req)
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
	agent, err := h.agent(req)
	if err != nil {
		return err
	}

	// A request that names a scope an agent's key never has, such as canAccessAPI, is refused rather than having it
	// ignored, so that a caller is never told a key was made with a scope that it does not have.
	var body types.AgentAPIKeyCreateRequest
	if err := readAgentIdentityRequest(req, &body, false); err != nil {
		return err
	}

	created, err := req.GatewayClient.CreateAgentAPIKey(req.Context(), agent.ID, gtypes.AgentAPIKeyCreateRequest{
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
	agent, err := h.agent(req)
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
	agent, err := h.agent(req)
	if err != nil {
		return err
	}
	// The key is looked up first, because the client revokes nothing, and succeeds, for a key of another owner.
	key, err := h.apiKey(req, agent)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.RevokeAgentAPIKey(req.Context(), agent.ID, key.ID); err != nil {
		return agentIdentityError(err)
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// agent returns the agent the request's path names, once agent identities are known to be available. A deleted or
// unknown agent, and an ID that is not one, are 404.
func (h *AgentIdentityHandler) agent(req api.Context) (*gtypes.Agent, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
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

// agentFromManifest returns the agent whose metadata manifest describes.
func agentFromManifest(manifest types.AgentIdentityManifest) (gtypes.Agent, error) {
	if strings.TrimSpace(manifest.DisplayName) == "" {
		return gtypes.Agent{}, types.NewErrBadRequest("displayName is required")
	}

	return gtypes.Agent{
		DisplayName: strings.TrimSpace(manifest.DisplayName),
		Description: manifest.Description,
	}, nil
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
		ID:                strconv.FormatUint(uint64(agent.ID), 10),
		Created:           *types.NewTime(agent.CreatedAt),
		Type:              "agentidentity",
		DisplayName:       agent.DisplayName,
		Description:       agent.Description,
		Status:            agent.Status(),
		DisabledReason:    agent.DisabledReason,
		ActiveAPIKeyCount: activeAPIKeyCount,
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
