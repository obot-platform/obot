package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

func (h *ClaudeRunnerHandler) SaveEnvironment(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrHTTP(403, "Administrator access required")
	}
	if !h.enabled() {
		return types.NewErrBadRequest("Claude environments are disabled")
	}
	var input struct {
		gt.ClaudeEnvironment
		EnvironmentKey string `json:"environmentKey"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" || len(input.DisplayName) > 200 {
		return types.NewErrBadRequest("A display name of at most 200 characters is required")
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		environment := &gt.ClaudeEnvironment{ID: req.PathValue("environment")}
		if environment.ID == "" {
			environment.ID = "cen-" + randomClaudeToken()[:24]
		} else {
			var err error
			environment, err = req.GatewayClient.ClaudeEnvironment(req.Context(), environment.ID)
			if err != nil {
				return claudeConfigError(err)
			}
		}
		if err := applyClaudeEnvironmentKey(environment, input.EnvironmentKey); err != nil {
			return err
		}
		input.EnvironmentKey = strings.TrimSpace(input.EnvironmentKey)
		if err := prepareClaudeEgressSecrets(&input.ClaudeEnvironment, environment); err != nil {
			return err
		}
		environment.EgressSecrets = input.EgressSecrets
		environment.DisplayName = input.DisplayName

		environment.Profiles, environment.Runtime = input.Profiles, input.Runtime
		if err := h.validateEnvironment(req, environment, req.PathValue("environment") == ""); err != nil {
			return err
		}
		var secrets map[string]string
		if input.EnvironmentKey != "" {
			environment.Revision = randomClaudeToken()
			secrets = map[string]string{"key": input.EnvironmentKey, "token": randomClaudeToken()}
		}
		if err := req.GatewayClient.SaveClaudeEnvironment(req.Context(), environment, secrets); err != nil {
			return claudeConfigError(err)
		}
		if err := refreshEnvironmentCredentials(req, environment); err != nil {
			return err
		}
		redactClaudeEgressSecrets(environment)
		return req.Write(environment)
	})
}

// applyClaudeEnvironmentKey extracts configuration metadata, not an authenticated
// identity. Anthropic authenticates the key; session tokens are verified separately.
func applyClaudeEnvironmentKey(environment *gt.ClaudeEnvironment, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		if environment.OrganizationID == "" || environment.EnvironmentID == "" {
			return types.NewErrBadRequest("Environment key is required")
		}
		return nil
	}
	if len(key) > 32768 {
		return types.NewErrBadRequest("Environment key is too large")
	}

	claims := &clauderunner.Claims{}
	token := regexp.MustCompile(`^sk-ant-[a-z0-9]+-`).ReplaceAllString(key, "")
	_, _, err := jwt.NewParser().ParseUnverified(token, claims)
	if err != nil || !regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(claims.OrgUUID) || !regexp.MustCompile(`^ccpool_[a-zA-Z0-9]{1,120}$`).MatchString(claims.Pool) {
		return types.NewErrBadRequest("Environment key must contain a valid organization_uuid and ccr:pool_id")
	}

	organizationID := strings.ToLower(claims.OrgUUID)
	if environment.OrganizationID != "" && (environment.OrganizationID != organizationID || environment.EnvironmentID != claims.Pool) {
		return types.NewErrBadRequest("The key belongs to a different Claude environment; create a new environment instead")
	}
	environment.OrganizationID, environment.EnvironmentID = organizationID, claims.Pool
	return nil
}

func (h *ClaudeRunnerHandler) DeleteEnvironment(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrHTTP(403, "Administrator access required")
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		var instances v1.AgentInstanceList
		if err := req.List(&instances); err != nil {
			return err
		}
		for i := range instances.Items {
			instance := &instances.Items[i]
			if instance.Spec.ClaudeEnvironment != req.PathValue("environment") {
				continue
			}
			if err := revokeClaudeKey(req, instance.Name); err != nil {
				return err
			}
			if err := req.Delete(instance); err != nil {
				return err
			}
		}

		if err := req.GatewayClient.DeleteClaudeEnvironment(req.Context(), req.PathValue("environment")); err != nil {
			return err
		}
		req.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func claudeConfigError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrHTTP(404, "Claude configuration not found")
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return types.NewErrHTTP(409, "This environment is already configured")
	}
	return err
}

// Rotate active scopes under the admission lock so removed vMCP grants cannot
// remain usable until token expiry. Claude's headers helper picks up the new key.
func refreshEnvironmentCredentials(req api.Context, environment *gt.ClaudeEnvironment) error {
	var instances v1.AgentInstanceList
	if err := req.List(&instances); err != nil {
		return err
	}
	for _, instance := range instances.Items {
		if instance.Spec.ClaudeEnvironment != environment.ID || instance.Spec.ClaudeDone || instance.DeletionTimestamp != nil {
			continue
		}
		effective, err := clauderunner.AuthorizedEnvironment(req.Context(), req.GatewayClient, instance.Spec.ClaudeEnvironment, instance.Spec.ClaudeAccount, instance.Spec.UserID)
		if clauderunner.AccessDenied(err) {
			// Revoke the LLM key too, including for sessions that never had MCP tools.
			if err := revokeClaudeKey(req, instance.Name); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		credential, err := req.GatewayClient.RevealCredential(req.Context(), []string{"substrate-agent"}, instance.Name)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		keyID, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
		if err != nil {
			return err
		}
		key, err := req.GatewayClient.GetAPIKeyByID(req.Context(), uint(keyID))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Expired keys may already be deleted; renewal checks the current grants.
			continue
		}
		if err != nil {
			return err
		}
		if key.RevokedAt != nil {
			continue
		}
		if len(effective.MCPServerIDs) == 0 {
			// No tools are available now; a future grant may issue a new scoped key.
			if err := req.GatewayClient.RevokeClaudeCredential(req.Context(), "substrate-agent", instance.Name); err != nil {
				return err
			}
			if _, err := req.GatewayClient.DeleteCredential(req.Context(), "substrate-agent", instance.Name); err != nil {
				return err
			}
		} else if key.ExpiresAt != nil && key.ExpiresAt.After(time.Now()) {
			if _, err := req.GatewayClient.EnsureClaudeCredential(req.Context(), key.UserID, instance.Name, effective.MCPServerIDs, *key.ExpiresAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *ClaudeRunnerHandler) validateEnvironment(req api.Context, environment *gt.ClaudeEnvironment, creating bool) error {
	if len(environment.Profiles) > 100 {
		return types.NewErrBadRequest("Environment configuration is too large")
	}
	manifest := types.VMCPManifest{DisplayName: environment.DisplayName}
	if environment.Profiles != nil {
		manifest.Profiles = []types.VMCPProfile{}
		for _, profile := range environment.Profiles {
			manifest.Profiles = append(manifest.Profiles, types.VMCPProfile{Name: profile.Name, Subjects: profile.Subjects})
		}
	}
	if creating {
		manifest.Default(false)
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("%s", err)
	}
	if environment.Profiles == nil {
		environment.Profiles = []gt.ClaudeEnvironmentProfile{}
		for _, profile := range manifest.Profiles {
			environment.Profiles = append(environment.Profiles, gt.ClaudeEnvironmentProfile{Name: profile.Name, Subjects: profile.Subjects})
		}
	}
	environment.Runtime.Image = strings.TrimSpace(environment.Runtime.Image)
	environment.Runtime.WorkerImage = strings.TrimSpace(environment.Runtime.WorkerImage)
	clauderunner.DefaultRuntime(&environment.Runtime)
	if err := clauderunner.ValidateRuntime(clauderunner.WithDefaultImages(environment.Runtime, h.Substrate.Config.WorkerImage, h.Substrate.Config.RunnerImage)); err != nil {
		return types.NewErrBadRequest("%s", err)
	}
	egressHosts := slices.Clone(environment.Runtime.EgressHosts)
	validatedVMCPs := map[string]bool{}
	for _, profile := range environment.Profiles {
		if err := clauderunner.ValidateEgressHosts(profile.EgressHosts); err != nil {
			return types.NewErrBadRequest("Profile %s: %s", profile.Name, err)
		}
		for _, host := range profile.EgressHosts {
			if !slices.Contains(egressHosts, host) {
				egressHosts = append(egressHosts, host)
			}
		}
		for _, id := range profile.MCPServerIDs {
			if validatedVMCPs[id] {
				continue
			}

			var server v1.VMCP
			if err := req.Get(&server, id); err != nil {
				return types.NewErrBadRequest("Select existing vMCPs")
			}
			if server.Spec.UserID != "" {
				return types.NewErrBadRequest("Profiles require shared vMCPs")
			}

			validatedVMCPs[id] = true
		}
		if err := clauderunner.ValidateResources(environment.Runtime.Worker, clauderunner.EffectiveResources(environment.Runtime.Resources, profile.Resources)); err != nil {
			return types.NewErrBadRequest("Profile %s: %s", profile.Name, err)
		}
	}
	if err := clauderunner.ValidateEgressHosts(egressHosts); err != nil {
		return types.NewErrBadRequest("Combined environment and profile egress: %s", err)
	}

	return nil
}
