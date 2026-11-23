package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// AgentDelegationRequestTTL is how long a user has to approve or cancel an agent's request.
	AgentDelegationRequestTTL = 10 * time.Minute
	// AgentDelegationCodeTTL is how long an agent has to redeem the code that a user's approval issued.
	AgentDelegationCodeTTL = 2 * time.Minute
	// MaxPendingAgentDelegationRequests is how many requests an agent can have waiting for approval at once.
	MaxPendingAgentDelegationRequests = 100

	// agentDelegationValidationCacheTTL is how long DelegationValid trusts that a delegation is valid, as the API key
	// validation cache does: a revocation on another replica applies within it.
	agentDelegationValidationCacheTTL = 15 * time.Second
	// agentDelegationRequestCleanupInterval is how often the requests that can no longer be used are deleted.
	agentDelegationRequestCleanupInterval = 10 * time.Minute

	// AgentDelegationInvalidCode is the error of the 401 response to an agent whose token that acts on behalf of a
	// user is refused (AgentDelegationInvalidError). The agent mints a new token from the delegation, or asks the user
	// to approve it again.
	AgentDelegationInvalidCode = "agent_delegation_invalid"
)

var (
	// ErrInvalidAgentDelegationRequest reports that CreateAgentDelegationRequest was asked for a request it must not
	// create, such as one whose return URL is not on the agent's allowed hosts. Its message says why.
	ErrInvalidAgentDelegationRequest = errors.New("invalid agent delegation request")
	// ErrAgentDelegationRequestLimit reports that an agent has MaxPendingAgentDelegationRequests requests waiting.
	ErrAgentDelegationRequestLimit = fmt.Errorf("the agent has %d approval requests waiting; wait for some to be approved, cancelled or to expire", MaxPendingAgentDelegationRequests)
	// ErrAgentDelegationRequestNotFound reports a request that does not exist, or no longer does.
	ErrAgentDelegationRequestNotFound = errors.New("agent delegation request not found")
	// ErrAgentDelegationRequestUnusable reports a request that can no longer be approved or cancelled: it expired, or
	// it was approved, redeemed or cancelled already.
	ErrAgentDelegationRequestUnusable = errors.New("the approval request has expired or was already used")
	// ErrAgentDelegationReturnURLNotAllowed reports a request whose return URL is no longer on its agent's allowed
	// return hosts, which an administrator changed since it was created.
	ErrAgentDelegationReturnURLNotAllowed = errors.New("the return URL of the approval request is no longer allowed")
	// ErrAgentDelegationNoProfile reports a user that no profile of the agent matches, so the agent may
	// not act for them.
	ErrAgentDelegationNoProfile = errors.New("the agent may not act on behalf of this user")
	// ErrAgentDelegationOtherUser reports a user approving a request that asks to approve another user's delegation
	// again.
	ErrAgentDelegationOtherUser = errors.New("the approval request is for another user")
	// ErrAgentDelegationNotFound reports a delegation that does not exist, or that is not the caller's.
	ErrAgentDelegationNotFound = errors.New("agent delegation not found")
	// ErrInvalidAgentDelegationGrant reports a code that could not be redeemed. Its message says why.
	ErrInvalidAgentDelegationGrant = errors.New("invalid grant")
)

// AgentDelegationRequestInput is what an agent asks for when it requests a user's approval.
type AgentDelegationRequestInput struct {
	AgentID uint
	// APIKeyID is the agent's key that made the request.
	APIKeyID     uint
	ReturnURL    string
	SecretHash   string
	LoginHint    string
	DelegationID string
}

// AgentDelegationApproval is a request as the user who may approve it sees it.
type AgentDelegationApproval struct {
	Request *types.AgentDelegationRequest
	Agent   *types.Agent
	// Delegation is the user's delegation of the agent: after approval, the one it created or updated, and before it,
	// the one the user already has, if any.
	Delegation *types.AgentDelegation
}

// AgentDelegationInvalidError refuses a token with which an agent acts on behalf of a user that no longer acts for
// anyone: it is malformed or expired, its delegation was revoked, the agent or the user is disabled or deleted, no profile
// of the agent matches the user any more, or the key that minted it is revoked, expired or not the agent's.
// Reason says which, for logs; the response carries only AgentDelegationInvalidCode.
type AgentDelegationInvalidError struct {
	Reason string
}

// agentDelegationValidation is a delegation that DelegationValid found valid.
type agentDelegationValidation struct {
	delegationID string
	agentID      uint
	userID       uint
}

// CreateAgentDelegationRequest records an agent's request for a user's approval, which expires after
// AgentDelegationRequestTTL. It refuses, as an *AgentAccessDeniedError, an agent that is not active, and, as
// ErrInvalidAgentDelegationRequest, a return URL that is not on the agent's allowed return hosts, a secret hash that
// is not one, and a delegation to approve again that is not the agent's. An agent with
// MaxPendingAgentDelegationRequests unexpired requests waiting is refused with ErrAgentDelegationRequestLimit. The
// agent is recorded as the actor in the admin audit.
func (c *Client) CreateAgentDelegationRequest(ctx context.Context, in AgentDelegationRequestInput) (*types.AgentDelegationRequest, error) {
	if err := agentdelegation.CheckSecretHash(in.SecretHash); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAgentDelegationRequest, err)
	}
	if err := agentdelegation.CheckLoginHint(in.LoginHint); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAgentDelegationRequest, err)
	}

	now := time.Now().UTC()
	request := &types.AgentDelegationRequest{
		ID:           newAgentDelegationID(types2.AgentDelegationRequestIDPrefix),
		AgentID:      in.AgentID,
		State:        types.AgentDelegationRequestPending,
		APIKeyID:     in.APIKeyID,
		ReturnURL:    in.ReturnURL,
		SecretHash:   in.SecretHash,
		LoginHint:    in.LoginHint,
		DelegationID: in.DelegationID,
		CreatedAt:    now,
		ExpiresAt:    now.Add(AgentDelegationRequestTTL),
	}
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The agent's lock serializes its requests, so that the limit holds when they race.
		agent, err := lockActiveAgent(tx, in.AgentID)
		if err != nil {
			return err
		}
		if err := agentdelegation.CheckReturnURL(in.ReturnURL, agent.AllowedReturnHosts); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidAgentDelegationRequest, err)
		}
		if in.DelegationID != "" {
			var count int64
			if err := tx.Model(&types.AgentDelegation{}).Where("id = ? AND agent_id = ?", in.DelegationID, in.AgentID).Count(&count).Error; err != nil {
				return fmt.Errorf("failed to read delegation %s: %w", in.DelegationID, err)
			}
			if count == 0 {
				return fmt.Errorf("%w: delegation %q is not a delegation of the agent", ErrInvalidAgentDelegationRequest, in.DelegationID)
			}
		}

		var pending int64
		if err := tx.Model(&types.AgentDelegationRequest{}).
			Where("agent_id = ? AND state = ? AND expires_at > ?", in.AgentID, types.AgentDelegationRequestPending, now).
			Count(&pending).Error; err != nil {
			return fmt.Errorf("failed to count the pending approval requests of agent %d: %w", in.AgentID, err)
		}
		if pending >= MaxPendingAgentDelegationRequests {
			return ErrAgentDelegationRequestLimit
		}

		if err := tx.Create(request).Error; err != nil {
			return fmt.Errorf("failed to create an approval request for agent %d: %w", in.AgentID, err)
		}
		// The return URL's host, never the URL: its path and query are the agent's, and could hold anything.
		returnHost := ""
		if host, err := agentdelegation.ReturnHost(in.ReturnURL); err == nil {
			returnHost = host
		}
		details := map[string]any{
			"apiKeyID":     in.APIKeyID,
			"returnHost":   returnHost,
			"hasLoginHint": in.LoginHint != "",
		}
		if in.DelegationID != "" {
			details["delegationID"] = in.DelegationID
		}
		return recordAdminAuditEventTx(tx, AdminAuditEventInput{
			Actor:        principal.Agent(in.AgentID).String(),
			Action:       types.AdminAuditActionAgentDelegationRequestCreate,
			ResourceKind: types.AdminAuditResourceAgentDelegationRequest,
			ResourceID:   request.ID,
			ParentID:     strconv.FormatUint(uint64(in.AgentID), 10),
			Details:      details,
		})
	}); err != nil {
		return nil, err
	}
	return request, nil
}

// GetAgentDelegationApproval returns a request as approver, the signed-in user with their groups, would approve it,
// once it checks that they may: the request is pending and unexpired (else ErrAgentDelegationRequestUnusable), its
// agent is active (else an *AgentAccessDeniedError), its return URL is still on the agent's allowed return hosts (else
// ErrAgentDelegationReturnURLNotAllowed), a profile of the agent matches the user (else
// ErrAgentDelegationNoProfile), and a request to approve a delegation again is for the user's own (else
// ErrAgentDelegationOtherUser). A request that does not exist is ErrAgentDelegationRequestNotFound.
func (c *Client) GetAgentDelegationApproval(ctx context.Context, requestID string, approver subjects.Caller) (*AgentDelegationApproval, error) {
	return checkAgentDelegationApproval(c.db.WithContext(ctx), requestID, approver, false, time.Now())
}

// ApproveAgentDelegationRequest approves a request as approver, the signed-in user with their groups, once it checks
// all that GetAgentDelegationApproval does. It creates the delegation of the agent and the user, or updates the one
// they have and keeps its ID, and issues a code for the agent to redeem within AgentDelegationCodeTTL, of which only a
// digest is stored. It returns the approval, with the delegation, and the code. The user is recorded as the actor in
// the admin audit.
func (c *Client) ApproveAgentDelegationRequest(ctx context.Context, requestID string, approver subjects.Caller) (*AgentDelegationApproval, string, error) {
	code := newAgentDelegationCode()
	var approval *AgentDelegationApproval
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var err error
		if approval, err = checkAgentDelegationApproval(tx, requestID, approver, true, now); err != nil {
			return err
		}
		request, agent := approval.Request, approval.Agent
		userID, _ := approver.Principal.UserID()

		created := approval.Delegation == nil
		if created {
			approval.Delegation = &types.AgentDelegation{
				ID:              newAgentDelegationID(types2.AgentDelegationIDPrefix),
				AgentID:         agent.ID,
				UserID:          userID,
				APIKeyID:        request.APIKeyID,
				FirstApprovedAt: now,
				LastApprovedAt:  now,
			}
			if err := tx.Create(approval.Delegation).Error; err != nil {
				return fmt.Errorf("failed to create the delegation of agent %d for user %d: %w", agent.ID, userID, err)
			}
		} else {
			approval.Delegation.APIKeyID = request.APIKeyID
			approval.Delegation.LastApprovedAt = now
			if err := tx.Model(approval.Delegation).UpdateColumns(map[string]any{
				"api_key_id":       request.APIKeyID,
				"last_approved_at": now,
			}).Error; err != nil {
				return fmt.Errorf("failed to update delegation %s: %w", approval.Delegation.ID, err)
			}
		}

		codeExpiresAt := now.Add(AgentDelegationCodeTTL)
		// The state is part of the condition, so that a request is approved once even where the row lock is not
		// taken.
		result := tx.Model(&types.AgentDelegationRequest{}).
			Where("id = ? AND state = ?", request.ID, types.AgentDelegationRequestPending).
			UpdateColumns(map[string]any{
				"state":            types.AgentDelegationRequestApproved,
				"approved_user_id": userID,
				"approved_at":      now,
				"delegation_id":    approval.Delegation.ID,
				"code_hash":        agentDelegationCodeHash(code),
				"code_expires_at":  codeExpiresAt,
			})
		if result.Error != nil {
			return fmt.Errorf("failed to approve request %s: %w", request.ID, result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAgentDelegationRequestUnusable
		}
		request.State = types.AgentDelegationRequestApproved
		request.ApprovedUserID = userID
		request.ApprovedAt = &now
		request.DelegationID = approval.Delegation.ID
		request.CodeExpiresAt = &codeExpiresAt

		return recordAdminAuditEventTx(tx, AdminAuditEventInput{
			Actor:        approver.Principal.String(),
			Action:       types.AdminAuditActionAgentDelegationApprove,
			ResourceKind: types.AdminAuditResourceAgentDelegation,
			ResourceID:   approval.Delegation.ID,
			ParentID:     strconv.FormatUint(uint64(agent.ID), 10),
			Details: map[string]any{
				"requestID": request.ID,
				"userID":    strconv.FormatUint(uint64(userID), 10),
				"apiKeyID":  request.APIKeyID,
				"created":   created,
			},
		})
	}); err != nil {
		return nil, "", err
	}
	return approval, code, nil
}

// CancelAgentDelegationRequest cancels a request as approver, once it checks all that GetAgentDelegationApproval
// does, so that only a user who could approve a request learns where it sends the browser back to. It returns the
// cancelled request.
func (c *Client) CancelAgentDelegationRequest(ctx context.Context, requestID string, approver subjects.Caller) (*types.AgentDelegationRequest, error) {
	var request *types.AgentDelegationRequest
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		approval, err := checkAgentDelegationApproval(tx, requestID, approver, true, time.Now())
		if err != nil {
			return err
		}
		request = approval.Request
		result := tx.Model(&types.AgentDelegationRequest{}).
			Where("id = ? AND state = ?", request.ID, types.AgentDelegationRequestPending).
			UpdateColumn("state", types.AgentDelegationRequestCancelled)
		if result.Error != nil {
			return fmt.Errorf("failed to cancel request %s: %w", request.ID, result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAgentDelegationRequestUnusable
		}
		request.State = types.AgentDelegationRequestCancelled
		return nil
	}); err != nil {
		return nil, err
	}
	return request, nil
}

// checkAgentDelegationApproval reads a request and its agent, and checks that approver may approve or cancel it now
// (GetAgentDelegationApproval). With lock, it locks the agent, then the request, then the user's delegation, the order
// in which every change to them takes their locks.
func checkAgentDelegationApproval(tx *gorm.DB, requestID string, approver subjects.Caller, lock bool, now time.Time) (*AgentDelegationApproval, error) {
	userID, ok := approver.Principal.UserID()
	if !ok || userID == 0 {
		return nil, ErrAgentDelegationNoProfile
	}

	request, err := getAgentDelegationRequest(tx, requestID, false)
	if err != nil {
		return nil, err
	}
	var agent *types.Agent
	if lock {
		if agent, err = lockActiveAgent(tx, request.AgentID); err != nil {
			return nil, err
		}
		// Read again under the agent's lock, which every change to the request takes first.
		if request, err = getAgentDelegationRequest(tx, requestID, true); err != nil {
			return nil, err
		}
	} else if agent, err = activeAgent(tx, request.AgentID); err != nil {
		return nil, err
	}
	if request.State != types.AgentDelegationRequestPending || !now.Before(request.ExpiresAt) {
		return nil, ErrAgentDelegationRequestUnusable
	}
	// The browser is sent to the return URL, so it must still be allowed when it is.
	if err := agentdelegation.CheckReturnURL(request.ReturnURL, agent.AllowedReturnHosts); err != nil {
		return nil, ErrAgentDelegationReturnURLNotAllowed
	}

	if !agentprofile.Matches(agent.Profiles, approver) {
		return nil, ErrAgentDelegationNoProfile
	}

	delegation, err := findAgentDelegation(tx, "agent_id = ? AND user_id = ?", []any{agent.ID, userID}, lock)
	if err != nil {
		return nil, err
	}
	if request.DelegationID != "" && (delegation == nil || delegation.ID != request.DelegationID) {
		// The request asks to approve a delegation again, which is not this user's: another user's, or one that was
		// revoked since.
		named, err := findAgentDelegation(tx, "id = ?", []any{request.DelegationID}, false)
		if err != nil {
			return nil, err
		}
		if named == nil {
			return nil, ErrAgentDelegationRequestUnusable
		}
		return nil, ErrAgentDelegationOtherUser
	}

	return &AgentDelegationApproval{
		Request:    request,
		Agent:      agent,
		Delegation: delegation,
	}, nil
}

// RedeemAgentDelegationCode redeems the code that a user's approval of a request issued, for the agent with agentID,
// with its key apiKeyID, and returns the delegation the approval created or updated. The request must be the agent's
// and approved, the code its own and unexpired, and secret the one whose hash the request was created with. A code is
// redeemed once: a wrong secret uses it up too, so a code that came back to a browser session without the secret, as
// one does when an approval link is forwarded to someone else, can never be redeemed. A code that is wrong, expired or
// presented by another agent is refused without using it up. Every refusal is ErrInvalidAgentDelegationGrant, with a
// message that says why. The agent is recorded as the actor in the admin audit, of a refused redemption that used up
// the code as well.
func (c *Client) RedeemAgentDelegationCode(ctx context.Context, agentID, apiKeyID uint, requestID, code, secret string) (*types.AgentDelegation, error) {
	// A request of another agent is reported as one that does not exist, so that an agent learns nothing of another's
	// requests, and its code is not used up.
	unknown := fmt.Errorf("%w: unknown request", ErrInvalidAgentDelegationGrant)
	if requestID == "" || code == "" {
		return nil, fmt.Errorf("%w: requestID and code are required", ErrInvalidAgentDelegationGrant)
	}

	var (
		delegation *types.AgentDelegation
		refusal    error
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		request, err := getAgentDelegationRequest(tx, requestID, false)
		if errors.Is(err, ErrAgentDelegationRequestNotFound) {
			return unknown
		} else if err != nil {
			return err
		}
		if request.AgentID != agentID {
			return unknown
		}
		// The agent's lock first, as every change to its requests and delegations takes it.
		if _, err := lockActiveAgent(tx, agentID); err != nil {
			return err
		}
		if request, err = getAgentDelegationRequest(tx, requestID, true); errors.Is(err, ErrAgentDelegationRequestNotFound) {
			return unknown
		} else if err != nil {
			return err
		}

		switch request.State {
		case types.AgentDelegationRequestApproved:
		case types.AgentDelegationRequestPending:
			return fmt.Errorf("%w: the request has not been approved", ErrInvalidAgentDelegationGrant)
		case types.AgentDelegationRequestCancelled:
			return fmt.Errorf("%w: the user cancelled the request", ErrInvalidAgentDelegationGrant)
		default:
			return fmt.Errorf("%w: the code was already used", ErrInvalidAgentDelegationGrant)
		}
		if request.CodeExpiresAt == nil || !now.Before(*request.CodeExpiresAt) {
			return fmt.Errorf("%w: the code has expired", ErrInvalidAgentDelegationGrant)
		}
		if request.CodeHash == "" || subtle.ConstantTimeCompare([]byte(agentDelegationCodeHash(code)), []byte(request.CodeHash)) != 1 {
			return fmt.Errorf("%w: the code is not the request's", ErrInvalidAgentDelegationGrant)
		}

		// From here on, the code is used up whatever happens.
		if err := tx.Model(&types.AgentDelegationRequest{}).
			Where("id = ? AND state = ?", request.ID, types.AgentDelegationRequestApproved).
			UpdateColumns(map[string]any{
				"state":     types.AgentDelegationRequestRedeemed,
				"code_hash": "",
			}).Error; err != nil {
			return fmt.Errorf("failed to redeem request %s: %w", request.ID, err)
		}

		details := map[string]any{
			"apiKeyID":     apiKeyID,
			"delegationID": request.DelegationID,
			"userID":       strconv.FormatUint(uint64(request.ApprovedUserID), 10),
			"redeemed":     false,
		}
		audit := AdminAuditEventInput{
			Actor:        principal.Agent(agentID).String(),
			Action:       types.AdminAuditActionAgentDelegationRequestRedeem,
			ResourceKind: types.AdminAuditResourceAgentDelegationRequest,
			ResourceID:   request.ID,
			ParentID:     strconv.FormatUint(uint64(agentID), 10),
			Details:      details,
		}

		if !agentdelegation.SecretMatches(secret, request.SecretHash) {
			details["refusal"] = "secret_mismatch"
			refusal = fmt.Errorf("%w: the secret does not match the request; the code is used up, and the user must approve again", ErrInvalidAgentDelegationGrant)
			return recordAdminAuditEventTx(tx, audit)
		}

		if delegation, err = findAgentDelegation(tx, "id = ? AND agent_id = ? AND user_id = ?", []any{request.DelegationID, agentID, request.ApprovedUserID}, true); err != nil {
			return err
		}
		if delegation == nil {
			details["refusal"] = "delegation_revoked"
			refusal = fmt.Errorf("%w: the delegation was revoked; the user must approve again", ErrInvalidAgentDelegationGrant)
			return recordAdminAuditEventTx(tx, audit)
		}
		delegation.LastUsedAt = &now
		if err := tx.Model(delegation).UpdateColumn("last_used_at", now).Error; err != nil {
			return fmt.Errorf("failed to update delegation %s: %w", delegation.ID, err)
		}
		details["redeemed"] = true
		return recordAdminAuditEventTx(tx, audit)
	}); err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, refusal
	}
	return delegation, nil
}

// GetAgentDelegation returns the delegation with id, or ErrAgentDelegationNotFound.
func (c *Client) GetAgentDelegation(ctx context.Context, id string) (*types.AgentDelegation, error) {
	return c.getAgentDelegation(ctx, "id = ?", id)
}

// GetAgentDelegationOfAgent returns the delegation with id if it is a delegation of the agent with agentID, and
// ErrAgentDelegationNotFound otherwise, so that a delegation is never found through another agent.
func (c *Client) GetAgentDelegationOfAgent(ctx context.Context, id string, agentID uint) (*types.AgentDelegation, error) {
	return c.getAgentDelegation(ctx, "id = ? AND agent_id = ?", id, agentID)
}

// getAgentDelegation returns the delegation that query, with args, selects, or ErrAgentDelegationNotFound.
func (c *Client) getAgentDelegation(ctx context.Context, query string, args ...any) (*types.AgentDelegation, error) {
	delegation, err := findAgentDelegation(c.db.WithContext(ctx), query, args, false)
	if err != nil {
		return nil, err
	}
	if delegation == nil {
		return nil, ErrAgentDelegationNotFound
	}
	return delegation, nil
}

// TouchAgentDelegation records that the agent got a token from the delegation with id at now.
func (c *Client) TouchAgentDelegation(ctx context.Context, id string, now time.Time) error {
	if err := c.db.WithContext(ctx).Model(&types.AgentDelegation{}).Where("id = ?", id).UpdateColumn("last_used_at", now.UTC()).Error; err != nil {
		return fmt.Errorf("failed to update delegation %s: %w", id, err)
	}
	return nil
}

// ListAgentDelegations returns the delegations of the agent with agentID, in the order they were first approved,
// leaving out those of deleted users, which their cleanup removes.
func (c *Client) ListAgentDelegations(ctx context.Context, agentID uint) ([]types.AgentDelegation, error) {
	var delegations []types.AgentDelegation
	if err := c.db.WithContext(ctx).
		Joins("JOIN users ON users.id = agent_delegations.user_id AND users.deleted_at IS NULL").
		Where("agent_delegations.agent_id = ?", agentID).
		Order("agent_delegations.first_approved_at").Order("agent_delegations.id").
		Find(&delegations).Error; err != nil {
		return nil, fmt.Errorf("failed to list the delegations of agent %d: %w", agentID, err)
	}
	return delegations, nil
}

// ListUserAgentDelegations returns the delegations of the user with userID, in the order they were first approved,
// leaving out those of deleted agents, which their cleanup removes.
func (c *Client) ListUserAgentDelegations(ctx context.Context, userID uint) ([]types.AgentDelegation, error) {
	var delegations []types.AgentDelegation
	if err := c.db.WithContext(ctx).
		Joins("JOIN agents ON agents.id = agent_delegations.agent_id AND agents.deleted_at IS NULL").
		Where("agent_delegations.user_id = ?", userID).
		Order("agent_delegations.first_approved_at").Order("agent_delegations.id").
		Find(&delegations).Error; err != nil {
		return nil, fmt.Errorf("failed to list the delegations of user %d: %w", userID, err)
	}
	return delegations, nil
}

// RevokeAgentDelegation deletes the delegation with id, and the requests that would approve it again or redeem a code
// for it, and returns it. actor, the user or administrator who revoked it, is recorded in the admin audit. Tokens
// minted from it are refused from their next use on this replica, and within agentDelegationValidationCacheTTL on
// others (DelegationValid). What the user set up through the agent is the caller's to remove
// (agentdelegation.Cleanup). A delegation that does not exist is ErrAgentDelegationNotFound.
func (c *Client) RevokeAgentDelegation(ctx context.Context, id, actor string) (*types.AgentDelegation, error) {
	var delegation *types.AgentDelegation
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if delegation, err = findAgentDelegation(tx, "id = ?", []any{id}, false); err != nil {
			return err
		}
		if delegation == nil {
			return ErrAgentDelegationNotFound
		}
		// The agent's lock first, as every change to its requests and delegations takes it. The agent may be deleted
		// already, and its row is kept.
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", delegation.AgentID).Take(&types.Agent{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to lock agent %d: %w", delegation.AgentID, err)
		}
		requests := tx.Where("delegation_id = ? OR (agent_id = ? AND approved_user_id = ?)", delegation.ID, delegation.AgentID, delegation.UserID).
			Delete(&types.AgentDelegationRequest{})
		if requests.Error != nil {
			return fmt.Errorf("failed to delete the requests of delegation %s: %w", delegation.ID, requests.Error)
		}
		result := tx.Where("id = ?", delegation.ID).Delete(&types.AgentDelegation{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete delegation %s: %w", delegation.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			// Revoked at the same time by someone else, who recorded it.
			return ErrAgentDelegationNotFound
		}
		return recordAdminAuditEventTx(tx, AdminAuditEventInput{
			Actor:        actor,
			Action:       types.AdminAuditActionAgentDelegationRevoke,
			ResourceKind: types.AdminAuditResourceAgentDelegation,
			ResourceID:   delegation.ID,
			ParentID:     strconv.FormatUint(uint64(delegation.AgentID), 10),
			Details: map[string]any{
				"userID":           strconv.FormatUint(uint64(delegation.UserID), 10),
				"requestsDeleted":  requests.RowsAffected,
				"revokedByTheUser": actor == principal.User(delegation.UserID).String(),
			},
		})
	}); err != nil {
		return nil, err
	}
	c.invalidateAgentDelegationValidations(func(v agentDelegationValidation) bool { return v.delegationID == delegation.ID })
	return delegation, nil
}

// DeleteAgentDelegationsOfAgent deletes every delegation and request of the agent with agentID, which was deleted. It
// is idempotent.
func (c *Client) DeleteAgentDelegationsOfAgent(ctx context.Context, agentID uint) (int64, error) {
	var deleted int64
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", agentID).Delete(&types.AgentDelegationRequest{}).Error; err != nil {
			return fmt.Errorf("failed to delete the approval requests of agent %d: %w", agentID, err)
		}
		result := tx.Where("agent_id = ?", agentID).Delete(&types.AgentDelegation{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete the delegations of agent %d: %w", agentID, result.Error)
		}
		deleted = result.RowsAffected
		return nil
	}); err != nil {
		return 0, err
	}
	c.invalidateAgentDelegationValidations(func(v agentDelegationValidation) bool { return v.agentID == agentID })
	return deleted, nil
}

// DeleteAgentDelegationsOfUser deletes every delegation of the user with userID, which was deleted, and every request
// the user approved. It is idempotent.
func (c *Client) DeleteAgentDelegationsOfUser(ctx context.Context, userID uint) (int64, error) {
	var deleted int64
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("approved_user_id = ?", userID).Delete(&types.AgentDelegationRequest{}).Error; err != nil {
			return fmt.Errorf("failed to delete the approval requests of user %d: %w", userID, err)
		}
		result := tx.Where("user_id = ?", userID).Delete(&types.AgentDelegation{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete the delegations of user %d: %w", userID, result.Error)
		}
		deleted = result.RowsAffected
		return nil
	}); err != nil {
		return 0, err
	}
	c.invalidateAgentDelegationValidations(func(v agentDelegationValidation) bool { return v.userID == userID })
	return deleted, nil
}

// DelegationValid reports whether the delegation with delegationID, of the agent with agentID and the user with
// userID, exists, and both are active: neither disabled nor deleted. Whether a profile of the agent still
// matches the user is the caller's to check, with the user's groups.
//
// A delegation found valid is trusted for agentDelegationValidationCacheTTL, as API keys are, so that a revocation, or
// a user or agent disabled or deleted, on another replica applies within it. On this replica, it applies at once.
func (c *Client) DelegationValid(ctx context.Context, delegationID string, agentID, userID uint) (bool, error) {
	if delegationID == "" || agentID == 0 || userID == 0 {
		return false, nil
	}
	key := agentDelegationValidation{delegationID: delegationID, agentID: agentID, userID: userID}
	now := time.Now()
	if c.agentDelegationValidationCached(key, now) {
		return true, nil
	}

	var count int64
	if err := c.db.WithContext(ctx).Model(&types.AgentDelegation{}).
		Joins("JOIN agents ON agents.id = agent_delegations.agent_id AND agents.deleted_at IS NULL AND agents.disabled_at IS NULL").
		Joins("JOIN users ON users.id = agent_delegations.user_id AND users.deleted_at IS NULL AND users.disabled_at IS NULL").
		Where("agent_delegations.id = ? AND agent_delegations.agent_id = ? AND agent_delegations.user_id = ?", delegationID, agentID, userID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check delegation %s: %w", delegationID, err)
	}
	if count == 0 {
		return false, nil
	}
	c.cacheAgentDelegationValidation(key, now)
	return true, nil
}

// DeleteExpiredAgentDelegationRequests deletes the requests that can no longer be used at now: those that expired
// longer ago than a code issued just before their expiry lasts. It returns how many it deleted.
func (c *Client) DeleteExpiredAgentDelegationRequests(ctx context.Context, now time.Time) (int64, error) {
	result := c.db.WithContext(ctx).Where("expires_at < ?", now.UTC().Add(-AgentDelegationCodeTTL)).Delete(&types.AgentDelegationRequest{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete expired agent delegation requests: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// runAgentDelegationRequestCleanup deletes expired requests periodically.
func (c *Client) runAgentDelegationRequestCleanup(ctx context.Context) {
	ticker := time.NewTicker(agentDelegationRequestCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if deleted, err := c.DeleteExpiredAgentDelegationRequests(ctx, now); err != nil {
				slog.Error("Failed to delete expired agent delegation requests", "error", err)
			} else if deleted > 0 {
				slog.Debug("Deleted expired agent delegation requests", "count", deleted)
			}
		}
	}
}

func (c *Client) agentDelegationValidationCached(key agentDelegationValidation, now time.Time) bool {
	if c.agentDelegationCacheTTL <= 0 {
		return false
	}
	c.agentDelegationCacheLock.Lock()
	defer c.agentDelegationCacheLock.Unlock()
	expiresAt, ok := c.agentDelegationCache[key]
	if ok && now.After(expiresAt) {
		delete(c.agentDelegationCache, key)
		return false
	}
	return ok
}

func (c *Client) cacheAgentDelegationValidation(key agentDelegationValidation, now time.Time) {
	if c.agentDelegationCacheTTL <= 0 {
		return
	}
	c.agentDelegationCacheLock.Lock()
	defer c.agentDelegationCacheLock.Unlock()
	if c.agentDelegationCache == nil {
		c.agentDelegationCache = map[agentDelegationValidation]time.Time{}
	}
	c.agentDelegationCache[key] = now.Add(c.agentDelegationCacheTTL)
}

// invalidateAgentDelegationValidations forgets the validations that match, so that this replica refuses them at once.
func (c *Client) invalidateAgentDelegationValidations(match func(agentDelegationValidation) bool) {
	c.agentDelegationCacheLock.Lock()
	defer c.agentDelegationCacheLock.Unlock()
	for key := range c.agentDelegationCache {
		if match(key) {
			delete(c.agentDelegationCache, key)
		}
	}
}

// pruneExpiredAgentDelegationValidations forgets the validations that expired by now.
func (c *Client) pruneExpiredAgentDelegationValidations(now time.Time) {
	c.invalidateAgentDelegationValidations(func(key agentDelegationValidation) bool {
		return now.After(c.agentDelegationCache[key])
	})
}

// getAgentDelegationRequest reads the request with id, locking it with lock. One that does not exist is
// ErrAgentDelegationRequestNotFound.
func getAgentDelegationRequest(tx *gorm.DB, id string, lock bool) (*types.AgentDelegationRequest, error) {
	if lock {
		tx = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	request := new(types.AgentDelegationRequest)
	if err := tx.Where("id = ?", id).Take(request).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentDelegationRequestNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to read agent delegation request %s: %w", id, err)
	}
	return request, nil
}

// findAgentDelegation returns the delegation that query, with args, selects, locking it with lock, or nil if there is
// none.
func findAgentDelegation(tx *gorm.DB, query string, args []any, lock bool) (*types.AgentDelegation, error) {
	if lock {
		tx = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	delegation := new(types.AgentDelegation)
	if err := tx.Where(query, args...).Take(delegation).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read agent delegation: %w", err)
	}
	return delegation, nil
}

// activeAgent reads the agent with id, which must be active: a disabled, deleted or missing one is an
// *AgentAccessDeniedError.
func activeAgent(tx *gorm.DB, id uint) (*types.Agent, error) {
	agent := new(types.Agent)
	if err := tx.Where("id = ?", id).Take(agent).Error; err != nil {
		return nil, agentReadError(id, err)
	}
	return checkAgentActive(agent)
}

// lockActiveAgent is activeAgent, with the agent locked.
func lockActiveAgent(tx *gorm.DB, id uint) (*types.Agent, error) {
	agent, err := lockAgent(tx, id)
	if err != nil {
		return nil, agentReadError(id, err)
	}
	return checkAgentActive(agent)
}

func agentReadError(id uint, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &AgentAccessDeniedError{
			AgentID: id,
			Status:  types2.UserStatusDeleted,
		}
	}
	return &AgentAccessLookupError{
		AgentID: id,
		Err:     err,
	}
}

func checkAgentActive(agent *types.Agent) (*types.Agent, error) {
	if status := agent.Status(); status != types2.UserStatusActive {
		return nil, &AgentAccessDeniedError{
			AgentID: agent.ID,
			Status:  status,
		}
	}
	return agent, nil
}

// newAgentDelegationID returns a random ID with prefix, unguessable since a request's ID is all its approval link
// carries.
func newAgentDelegationID(prefix string) string {
	id := make([]byte, 16)
	// crypto/rand.Read never fails; it crashes the program instead.
	_, _ = rand.Read(id)
	return prefix + hex.EncodeToString(id)
}

// newAgentDelegationCode returns a random code, of which only agentDelegationCodeHash is stored.
func newAgentDelegationCode() string {
	code := make([]byte, 32)
	_, _ = rand.Read(code)
	return base64.RawURLEncoding.EncodeToString(code)
}

// agentDelegationCodeHash returns the digest of a code that is stored in its place.
func agentDelegationCodeHash(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}

func (e *AgentDelegationInvalidError) Error() string {
	return AgentDelegationInvalidCode + ": " + e.Reason
}
