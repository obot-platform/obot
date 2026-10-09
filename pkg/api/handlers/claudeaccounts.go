package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

func claudeOrganization(req api.Context) (string, error) {
	if !req.UserIsAdmin() {
		return "", types.NewErrHTTP(http.StatusForbidden, "Administrator access required")
	}
	organization := req.PathValue("organization")
	environments, err := req.GatewayClient.ClaudeEnvironments(req.Context())
	if err != nil {
		return "", err
	}
	for _, environment := range environments {
		if environment.OrganizationID == organization {
			return organization, nil
		}
	}
	return "", types.NewErrHTTP(http.StatusNotFound, "No Claude environment is configured for this organization")
}

func (h *ClaudeRunnerHandler) OrganizationAccounts(req api.Context) error {
	organization, err := claudeOrganization(req)
	if err != nil {
		return err
	}
	accounts, err := req.GatewayClient.ClaudeOrganizationAccounts(req.Context(), organization)
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"items": accounts})
}

func (h *ClaudeRunnerHandler) SaveOrganizationAccounts(req api.Context) error {
	organization, err := claudeOrganization(req)
	if err != nil {
		return err
	}
	var input struct {
		Items []gt.ClaudeOrganizationAccount `json:"items"`
	}
	req.Request.Body = http.MaxBytesReader(req.ResponseWriter, req.Request.Body, 4<<20)
	if err := req.Read(&input); err != nil {
		return err
	}
	if len(input.Items) > 10000 {
		return types.NewErrBadRequest("Import at most 10,000 accounts at a time")
	}
	seen := map[string]bool{}
	for i := range input.Items {
		account := &input.Items[i]
		normalized, err := clauderunner.NormalizeAccountID(account.Account)
		if err != nil {
			return types.NewErrBadRequest("Each member must have a valid Claude account ID")
		}
		account.Account = normalized
		account.OrganizationID = organization
		account.UserID = strings.TrimSpace(account.UserID)
		account.Email = strings.TrimSpace(account.Email)
		account.DisplayName = strings.TrimSpace(account.DisplayName)
		if len(account.Email) > 320 || len(account.DisplayName) > 500 {
			return types.NewErrBadRequest("Member name or email is too long")
		}
		if account.UUID != "" {
			id, err := uuid.Parse(account.UUID)
			if err != nil {
				return types.NewErrBadRequest("Invalid member UUID")
			}
			account.UUID = id.String()
			normalized, err := clauderunner.NormalizeAccountID(account.UUID)
			if err != nil || normalized != account.Account {
				return types.NewErrBadRequest("Member UUID and account ID do not match")
			}
		}
		if seen[account.Account] {
			return types.NewErrBadRequest("Duplicate Claude account in import")
		}
		seen[account.Account] = true
		if account.UserID != "" {
			id, err := strconv.ParseUint(account.UserID, 10, 64)
			if err != nil || id == 0 {
				return types.NewErrBadRequest("Invalid Obot user ID")
			}
			account.UserID = strconv.FormatUint(id, 10)
		}
	}

	err = req.GatewayClient.WithClaudeProvisionLock(req.Context(), func() error {
		if err := req.GatewayClient.SaveClaudeOrganizationAccounts(req.Context(), organization, input.Items); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return types.NewErrHTTP(409, "Each Obot user can map to only one Claude account per organization")
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.NewErrBadRequest("A mapped Obot user no longer exists; reload and review the mappings")
			}
			return err
		}

		// Revoke credentials immediately; the controller also rechecks mappings on every reconcile.
		environments, err := req.GatewayClient.ClaudeEnvironments(req.Context())
		if err != nil {
			return err
		}
		affected := map[string]bool{}
		for _, environment := range environments {
			if environment.OrganizationID == organization {
				affected[environment.ID] = true
			}
		}
		var instances v1.AgentInstanceList
		if err := req.List(&instances); err != nil {
			return err
		}
		for i := range instances.Items {
			inst := &instances.Items[i]
			if !affected[inst.Spec.ClaudeEnvironment] {
				continue
			}
			_, err := clauderunner.AuthorizedEnvironment(req.Context(), req.GatewayClient, inst.Spec.ClaudeEnvironment, inst.Spec.ClaudeAccount, inst.Spec.UserID)
			if err == nil {
				continue
			}
			if !clauderunner.AccessDenied(err) {
				return err
			}
			if err := revokeClaudeKey(req, inst.Name); err != nil {
				return err
			}
			if err := req.Delete(inst); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.OrganizationAccounts(req)
}
