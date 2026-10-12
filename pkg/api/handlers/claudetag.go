package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (h *ClaudeRunnerHandler) SaveClaudeTag(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrHTTP(403, "Administrator access required")
	}
	if !h.enabled() {
		return types.NewErrBadRequest("Claude environments are disabled")
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}
	return req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		environment, err := req.GatewayClient.ClaudeEnvironment(req.Context(), req.PathValue("environment"))
		if err != nil {
			return claudeConfigError(err)
		}
		if input.Enabled {
			if err := req.GatewayClient.EnsureClaudeTagUser(req.Context(), environment); err != nil {
				return claudeConfigError(err)
			}
		}
		environment.ClaudeTagEnabled = input.Enabled
		if err := req.GatewayClient.SaveClaudeEnvironment(req.Context(), environment, nil); err != nil {
			return err
		}

		var instances v1.AgentInstanceList
		if err := req.List(&instances); err != nil {
			return err
		}
		for i := range instances.Items {
			inst := &instances.Items[i]
			if inst.Spec.ClaudeEnvironment != environment.ID || inst.Spec.ClaudeAccount != clauderunner.ClaudeTagAccount {
				continue
			}
			if input.Enabled && inst.Spec.UserID == environment.ClaudeTagUserID {
				continue
			}
			if err := revokeClaudeKey(req, inst.Name); err != nil {
				return err
			}
			if err := req.Delete(inst); err != nil {
				return err
			}
		}
		redactClaudeEgressSecrets(environment)
		return req.Write(environment)
	})
}
