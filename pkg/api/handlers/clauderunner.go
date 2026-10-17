package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClaudeRunnerHandler struct {
	Substrate *substrate.Client
	Verifier  clauderunner.Verifier
}

func (h *ClaudeRunnerHandler) enabled() bool {
	return h.Substrate != nil
}

func randomClaudeToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (h *ClaudeRunnerHandler) Config(req api.Context) error {
	if !h.enabled() {
		return req.Write(map[string]any{"enabled": false})
	}
	environments, err := req.GatewayClient.ClaudeEnvironments(req.Context())
	if err != nil {
		return err
	}
	visible := []gt.ClaudeEnvironment{}
	mappings := []gt.ClaudeOrganizationAccount{}
	seen := map[string]bool{}
	for _, environment := range environments {
		if !req.UserIsAdmin() {
			resolved, err := clauderunner.ResolveEnvironment(req.User, &environment)
			if clauderunner.AccessDenied(err) {
				continue
			}
			if err != nil {
				return err
			}
			environment = resolved.ClaudeEnvironment
		}
		redactClaudeEgressSecrets(&environment)
		visible = append(visible, environment)
		mapping, err := req.GatewayClient.ClaudeOrganizationAccountForUser(req.Context(), environment.OrganizationID, req.User.GetUID())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !seen[environment.OrganizationID] {
			mappings = append(mappings, *mapping)
			seen[environment.OrganizationID] = true
		}
	}
	return req.Write(map[string]any{
		"enabled":            true,
		"environments":       visible,
		"mappings":           mappings,
		"defaultWorkerImage": h.Substrate.Config.WorkerImage,
		"defaultRunnerImage": h.Substrate.Config.RunnerImage,
	})
}

// Spawn is accessible without an Obot login, but authenticates the trusted orchestrator.
func (h *ClaudeRunnerHandler) Spawn(req api.Context) error {
	if !h.enabled() {
		return types.NewErrHTTP(503, "Claude environments are disabled")
	}
	environmentID := req.Request.Header.Get("X-Obot-Claude-Environment")
	environment, err := req.GatewayClient.ClaudeEnvironment(req.Context(), environmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrHTTP(401, "Invalid orchestrator credential")
	}
	if err != nil {
		return err
	}
	credential, err := req.GatewayClient.RevealCredential(req.Context(), []string{"claude-environment"}, environment.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err != nil || len(credential.Secrets["token"]) < 32 || subtle.ConstantTimeCompare([]byte(req.Request.Header.Get("X-Obot-Orchestrator-Token")), []byte(credential.Secrets["token"])) != 1 {
		return types.NewErrHTTP(401, "Invalid orchestrator credential")
	}
	var input struct {
		Order        string                       `json:"order"`
		Session      string                       `json:"session"`
		Account      string                       `json:"account"`
		Email        string                       `json:"email"`
		Pool         string                       `json:"pool"`
		WorkOrder    string                       `json:"workOrder"`
		Repositories []v1.ClaudeSessionRepository `json:"repositories"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}
	id := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	if input.Pool != environment.EnvironmentID || !id.MatchString(input.Order) || !id.MatchString(input.Session) || (input.Account != "" && !regexp.MustCompile(`^user_[a-zA-Z0-9]{1,120}$`).MatchString(input.Account)) || len(input.WorkOrder) < 32 || len(input.WorkOrder) > 32768 {
		return types.NewErrBadRequest("A session-bound work order is required")
	}
	if err := sanitizeClaudeRepositories(input.Repositories); err != nil {
		return types.NewErrBadRequest("%s", err.Error())
	}

	sum := sha256.Sum256([]byte(input.Pool + ":" + input.Order))
	name := fmt.Sprintf("acr-%x", sum[:16])
	err = req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		current, err := req.GatewayClient.ClaudeEnvironment(req.Context(), environment.ID)
		if err != nil {
			return claudeConfigError(err)
		}
		if current.Revision != environment.Revision {
			return types.NewErrHTTP(503, "Environment credential changed; retry provisioning")
		}
		owner := current.ClaudeTagUserID
		if input.Account == "" {
			if !current.ClaudeTagEnabled || owner == "" {
				return types.NewErrHTTP(403, "Claude Tag is not configured for this environment")
			}
			input.Account = clauderunner.ClaudeTagAccount
		} else {
			mapping, err := req.GatewayClient.MatchClaudeOrganizationAccount(req.Context(), environment.OrganizationID, input.Account, input.Email)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.NewErrHTTP(403, "Claude user not found. Ask an Obot administrator to map your Claude account")
			}
			if err != nil {
				return err
			}
			owner = mapping.UserID
		}
		effective, err := clauderunner.AuthorizedEnvironment(req.Context(), req.GatewayClient, environment.ID, input.Account, owner)
		if clauderunner.AccessDenied(err) {
			return types.NewErrHTTP(403, "The connected user does not have access to this environment")
		}
		if err != nil {
			return err
		}

		submitted, err := req.GatewayClient.ClaudeWorkOrderSubmitted(req.Context(), name)
		if err != nil {
			return err
		}
		if submitted {
			return nil
		}
		var existing v1.AgentInstance
		if err := req.Get(&existing, name); err == nil {
			return req.GatewayClient.MarkClaudeWorkOrderSubmitted(req.Context(), name)
		} else if !apierrors.IsNotFound(err) {
			return err
		}
		var all v1.AgentInstanceList
		if err := req.List(&all); err != nil {
			return err
		}
		active := 0
		var previous *v1.AgentInstance
		for i := range all.Items {
			inst := &all.Items[i]
			if inst.Spec.ClaudeEnvironment != environment.ID || inst.Spec.ClaudeOrder == "" || inst.DeletionTimestamp != nil {
				continue
			}
			if inst.Spec.ClaudeAccount == input.Account && inst.Spec.ClaudeSession == input.Session && inst.Spec.UserID == owner {
				if previous == nil || inst.CreationTimestamp.After(previous.CreationTimestamp.Time) {
					previous = inst
				}
			}
			if !inst.Spec.ClaudeDone {
				if inst.Spec.ClaudeAccount == input.Account {
					active++
				}
			}
		}
		if active >= 4 {
			return types.NewErrHTTP(429, "Claude runner capacity reached")
		}
		inst := &v1.AgentInstance{}
		inst.Name, inst.Namespace, inst.Finalizers = name, req.Namespace(), []string{substrate.Finalizer}
		inst.Spec = v1.AgentInstanceSpec{ClaudeRepositories: input.Repositories, UserID: owner, DisplayName: current.DisplayName, ClaudeEnvironment: environment.ID, ClaudeOrder: input.Order, ClaudeSession: input.Session, ClaudeAccount: input.Account}
		if previous != nil {
			mergeSlackMetadata(&inst.Spec, previous.Spec)
			// A new attempt cannot race a previous runner or restore an incomplete checkpoint.
			if !previous.Spec.ClaudeDone || previous.Status.State != "SUSPENDED" {
				return types.NewErrHTTP(503, "Waiting for the previous execution to release and checkpoint")
			}
			if previous.Spec.ClaudeCheckpoint {
				inst.Spec.ClaudePrevious = previous.Name
			}
		}
		runtime, err := json.Marshal(effective.Runtime)
		if err != nil {
			return err
		}
		inst.Spec.ClaudeRuntime = string(runtime)
		credential := gt.Credential{Context: "claude-runner", Name: name, Secrets: map[string]string{"workOrder": input.WorkOrder, "token": randomClaudeToken()}}
		if err := req.GatewayClient.SaveClaudeWorkOrder(req.Context(), credential); err != nil {
			return err
		}
		if err := req.Create(inst); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		return req.GatewayClient.MarkClaudeWorkOrderSubmitted(req.Context(), name)
	})
	if err != nil {
		return err
	}
	return req.Write(map[string]string{"instance": name})
}

func (h *ClaudeRunnerHandler) runner(req api.Context) (*v1.AgentInstance, error) {
	if !h.enabled() {
		return nil, types.NewErrHTTP(503, "Claude environments are disabled")
	}
	var inst v1.AgentInstance
	if err := req.Get(&inst, req.PathValue("instance")); err != nil {
		return nil, err
	}
	if inst.Spec.ClaudeOrder == "" || inst.DeletionTimestamp != nil || inst.Spec.ClaudeDone {
		return nil, types.NewErrHTTP(409, "Runner is no longer active")
	}
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{"claude-runner"}, inst.Name)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(req.Request.Header.Get("X-Obot-Runner-Token")), []byte(cred.Secrets["token"])) != 1 {
		return nil, types.NewErrHTTP(401, "Invalid runner credential")
	}
	return &inst, nil
}

func (h *ClaudeRunnerHandler) identity(req api.Context, inst *v1.AgentInstance) (*clauderunner.Claims, error) {
	environment, err := req.GatewayClient.ClaudeEnvironment(req.Context(), inst.Spec.ClaudeEnvironment)
	if err != nil {
		return nil, types.NewErrHTTP(403, "Claude environment is no longer available")
	}
	claims, err := h.Verifier.Verify(req.Context(), req.Request.Header.Get("X-Claude-Session-Token"), environment.EnvironmentID, environment.OrganizationID)
	if err == nil {
		// Tokens use cse_ while lifecycle hooks use session_ for the same ID.
		session := claims.Session
		if suffix, ok := strings.CutPrefix(session, "cse_"); ok {
			session = "session_" + suffix
		}

		owner, ownerErr := claims.Owner()
		switch {
		case ownerErr != nil:
			err = ownerErr
		case owner != inst.Spec.ClaudeAccount:
			err = fmt.Errorf("claude identity does not match the provisioned account")
		case session != inst.Spec.ClaudeSession:
			err = fmt.Errorf("claude identity does not match the provisioned session")
		}
	}
	if err != nil {
		// Verifier errors contain fixed reasons only, never tokens or claim values.
		slog.WarnContext(req.Context(), "Claude session identity rejected", "instance", inst.Name, "reason", err.Error())
		return nil, types.NewErrHTTP(401, "Invalid Claude session identity")
	}
	return claims, nil
}

func (h *ClaudeRunnerHandler) Credentials(req api.Context) error {
	if !h.enabled() {
		return types.NewErrHTTP(503, "Claude environments are disabled")
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error { return h.credentials(req) })
}

func (h *ClaudeRunnerHandler) credentials(req api.Context) error {
	inst, err := h.runner(req)
	if err != nil {
		return err
	}
	_, err = h.identity(req, inst)
	if err != nil {
		return err
	}
	// identity verified this binding; Tag tokens have no human account claim.
	environment, err := clauderunner.AuthorizedEnvironment(req.Context(), req.GatewayClient, inst.Spec.ClaudeEnvironment, inst.Spec.ClaudeAccount, inst.Spec.UserID)
	if clauderunner.AccessDenied(err) {
		return types.NewErrHTTP(403, "Claude connection or environment access has been removed")
	}
	if err != nil {
		return err
	}
	servers := map[string]any{}
	for _, id := range environment.MCPServerIDs {
		// Tell Claude authentication is configured so a component's 401 does not
		// trigger client registration. Egress replaces this placeholder with the scoped key.
		servers[id] = map[string]any{
			"type": "http",
			"url":  system.MCPConnectURL(h.Substrate.Config.ObotURL, id),
			"headers": map[string]string{
				"Authorization":     "Bearer obot-egress-placeholder",
				"X-Obot-Egress-Key": "placeholder",
			},
		}
	}
	req.ResponseWriter.Header().Set("Cache-Control", "no-store")
	return req.Write(map[string]any{"mcpServers": servers})
}

func revokeClaudeKey(req api.Context, name string) error {
	credential, err := req.GatewayClient.RevealCredential(req.Context(), []string{"substrate-agent"}, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	id, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
	if err != nil {
		return err
	}
	return req.GatewayClient.RevokeAPIKeyByID(req.Context(), uint(id))
}

// Status is polled with the per-runner credential; it exposes no session secrets.
func (h *ClaudeRunnerHandler) Status(req api.Context) error {
	var input v1.AgentInstanceSpec
	if err := req.Read(&input); err != nil {
		return err
	}
	for _, value := range []string{input.SlackSenderName, input.SlackSenderHandle, input.SlackChannelName, input.SlackThreadTimestamp, input.SlackLink} {
		if len(value) > 256 {
			return types.NewErrBadRequest("Slack metadata must not exceed 256 bytes per field")
		}
	}
	if input.SlackThreadTimestamp != "" && !regexp.MustCompile(`^[0-9]+\.[0-9]+$`).MatchString(input.SlackThreadTimestamp) {
		return types.NewErrBadRequest("Invalid Slack thread timestamp")
	}
	if input.SlackLink != "" {
		u, err := url.Parse(input.SlackLink)
		if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".slack.com") || u.User != nil || u.Port() != "" || !strings.HasPrefix(u.Path, "/archives/") {
			return types.NewErrBadRequest("Invalid Slack message link")
		}
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error { return h.status(req, input) })
}

func (h *ClaudeRunnerHandler) status(req api.Context, input v1.AgentInstanceSpec) error {
	inst, err := h.runner(req)
	if err != nil {
		return err
	}
	if inst.Spec.ClaudeAccount == clauderunner.ClaudeTagAccount && mergeSlackMetadata(&inst.Spec, input) {
		if err := req.Update(inst); err != nil {
			return err
		}
	}

	req.ResponseWriter.Header().Set("Cache-Control", "no-store")
	return req.Write(map[string]bool{"suspend": inst.Spec.Suspended})
}

func (h *ClaudeRunnerHandler) Finished(req api.Context) error {
	if !h.enabled() {
		return types.NewErrHTTP(503, "Claude environments are disabled")
	}
	var input struct {
		Checkpoint bool   `json:"checkpoint"`
		Reason     string `json:"reason"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}
	if !slices.Contains([]string{"completed", "failed", "interrupted", "runner-exited"}, input.Reason) {
		return types.NewErrBadRequest("Invalid exit reason")
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error { return h.finished(req, input.Checkpoint) })
}

func (h *ClaudeRunnerHandler) finished(req api.Context, checkpoint bool) error {
	inst, err := h.runner(req)
	if err != nil {
		return err
	}
	if err := revokeClaudeKey(req, inst.Name); err != nil {
		return err
	}
	inst.Spec.ClaudeDone, inst.Spec.ClaudeCheckpoint, inst.Spec.Suspended = true, checkpoint, true
	inst.Spec.ClaudeFinishedAt = metav1.Now()
	if err := req.Update(inst); err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// Repository metadata is for display only; never persist credentials from clone URLs.
func sanitizeClaudeRepositories(repositories []v1.ClaudeSessionRepository) error {
	if len(repositories) > 100 {
		return fmt.Errorf("too many session repositories")
	}

	for i := range repositories {
		repository := &repositories[i]
		if len(repository.URL) > 4096 || len(repository.Revision) > 1024 {
			return fmt.Errorf("session repository metadata is too long")
		}

		raw := repository.URL
		if !strings.Contains(raw, "://") {
			// Git's scp-style SSH remote: git@host:owner/repository.git.
			if host, path, ok := strings.Cut(raw, ":"); ok && strings.Contains(host, "@") {
				raw = "ssh://" + host + "/" + path
			}
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") {
			return fmt.Errorf("invalid session repository URL")
		}
		u.User, u.RawQuery, u.Fragment = nil, "", ""
		u.ForceQuery = false
		repository.URL = u.String()
	}
	return nil
}

// Fill each field once, including an explicit false mention flag.
func mergeSlackMetadata(dst *v1.AgentInstanceSpec, src v1.AgentInstanceSpec) bool {
	changed := false
	for target, value := range map[*string]string{
		&dst.SlackSenderName:      src.SlackSenderName,
		&dst.SlackSenderHandle:    src.SlackSenderHandle,
		&dst.SlackChannelName:     src.SlackChannelName,
		&dst.SlackThreadTimestamp: src.SlackThreadTimestamp,
		&dst.SlackLink:            src.SlackLink,
	} {
		if *target == "" && value != "" {
			*target = value
			changed = true
		}
	}
	if dst.SlackMentioned == nil && src.SlackMentioned != nil {
		value := *src.SlackMentioned
		dst.SlackMentioned = &value
		changed = true
	}
	return changed
}
