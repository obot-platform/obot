package oauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"k8s.io/client-go/util/retry"
)

func upstreamRequirementKey(requirement v1.UpstreamOAuthRequirement) string {
	// Scopes are excluded so repeated requirements for the same provider can merge.
	identity := strings.Join([]string{
		requirement.Component,
		requirement.ConnectID,
		requirement.TargetHash,
		requirement.ResourceMetadata,
		requirement.Realm,
		strconv.Itoa(requirement.StatusCode),
		requirement.ErrorCode,
	}, "\x00")

	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

// componentConnections is a request-local membership snapshot; each later request
// and callback builds a fresh one before validating captured targets.
func componentConnections(aggregate mcp.ServerConfig) map[string]string {
	connections := make(map[string]string, len(aggregate.Components))

	for _, component := range aggregate.Components {
		connections[component.Name] = component.ConnectID()
	}

	return connections
}

func (f *MCPOAuthHandlerFactory) validateUpstreamRequirement(req api.Context, connections map[string]string, requirement v1.UpstreamOAuthRequirement) (v1.MCPServer, mcp.ServerConfig, error) {
	if connection, ok := connections[requirement.Component]; !ok || connection != requirement.ConnectID {
		return v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrForbidden("captured component no longer belongs to this vMCP connection")
	}

	server, cfg, err := f.mcpSessionManager.ServerForAction(req.Context(), requirement.ConnectID, req.User)
	if err != nil {
		return server, cfg, err
	}

	if requirement.TargetHash == "" || cfg.Runtime != types.RuntimeRemote || cfg.URL != requirement.URL || mcp.OAuthTargetHash(server, cfg) != requirement.TargetHash {
		return server, cfg, types.NewErrForbidden("captured upstream configuration has changed")
	}

	return server, cfg, nil
}

func (h *handler) handoffUpstreamChallenges(req api.Context, request *v1.OAuthAuthRequest, aggregate mcp.ServerConfig) error {
	connections := componentConnections(aggregate)

	var ids []string

	for scope := range strings.FieldsSeq(request.Spec.Scope) {
		if !strings.HasPrefix(scope, client.UpstreamOAuthScopePrefix) {
			continue
		}

		id := strings.TrimPrefix(scope, client.UpstreamOAuthScopePrefix)

		// rand.Text produces 26-character IDs; malformed and unknown scopes are inert.
		if len(id) != 26 {
			continue
		}

		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		batch, err := req.GatewayClient.ClaimMCPOAuthChallenge(req.Context(), id, req.User.GetUID(), req.Namespace(), request.Spec.MCPID, request.Spec.ClientID, request.Spec.Resource, request.Name)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}

		if errors.Is(err, client.ErrMCPOAuthChallengeBinding) {
			return newOAuthError(ErrAccessDenied, "upstream challenge does not belong to this authorization", request.Spec.State)
		}

		if err != nil {
			return err
		}

		if len(batch.Requirements) == 0 {
			return types.NewErrBadRequest("invalid upstream requirement count")
		}

		for _, requirement := range batch.Requirements {
			if _, _, err := h.oauthChecker.validateUpstreamRequirement(req, connections, requirement); err != nil {
				return err
			}
		}

		// A retry merges into the latest transaction and preserves callback progress.
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var current v1.OAuthAuthRequest
			if err := req.Get(&current, request.Name); err != nil {
				return err
			}

			if current.Spec.UserID != req.UserID() {
				return types.NewErrForbidden("OAuth request belongs to another user")
			}

			for _, requirement := range batch.Requirements {
				key := upstreamRequirementKey(requirement)
				index := slices.IndexFunc(current.Spec.UpstreamOAuth, func(existing v1.UpstreamOAuthRequirement) bool {
					return upstreamRequirementKey(existing) == key
				})
				if index < 0 {
					current.Spec.UpstreamOAuth = append(current.Spec.UpstreamOAuth, requirement)
					continue
				}

				existing := &current.Spec.UpstreamOAuth[index]

				// Merge scopes only for the same component, provider, target and challenge type.
				existingScopes := strings.Fields(existing.Scope)
				slices.Sort(existingScopes)
				normalized := strings.Join(slices.Compact(existingScopes), " ")

				scopes := append(strings.Fields(existing.Scope), strings.Fields(requirement.Scope)...)
				slices.Sort(scopes)
				merged := strings.Join(slices.Compact(scopes), " ")
				if merged != normalized && existing.Started {
					return types.NewErrBadRequest("upstream scopes changed during authorization")
				}

				existing.Scope = merged
			}

			if err := req.Update(&current); err != nil {
				return err
			}

			*request = current

			return nil
		}); err != nil {
			return err
		}

		// The durable OAuth transaction owns the payload before this deletion.
		if err := req.GatewayClient.DeleteClaimedMCPOAuthChallenge(req.Context(), id, request.Name); err != nil {
			return err
		}
	}

	return nil
}

func (f *MCPOAuthHandlerFactory) pendingCapturedAuth(req api.Context, requestID, componentID string) ([]pendingComponentAuth, error) {
	var request v1.OAuthAuthRequest
	if err := req.Get(&request, requestID); err != nil {
		return nil, err
	}

	if request.Spec.UserID != req.UserID() || !request.Spec.ConsentApproved {
		return nil, types.NewErrForbidden("OAuth request is not approved for this user")
	}

	_, _, aggregate, err := f.mcpSessionManager.ServerForActionWithConnectID(req.Context(), request.Spec.MCPID, req.User)
	if err != nil {
		return nil, err
	}

	connections := componentConnections(aggregate)

	var result []pendingComponentAuth
	seen := make(map[string]bool)

	for _, requirement := range request.Spec.UpstreamOAuth {
		if componentID != "" && requirement.Component != componentID {
			continue
		}

		server, cfg, err := f.validateUpstreamRequirement(req, connections, requirement)
		if err != nil {
			return nil, err
		}

		// The UI has one OAuth link per component. Additional provider challenges
		// for that component become available after the current one completes.
		if requirement.Completed || seen[requirement.Component] {
			continue
		}

		if completed, err := reconcileCapturedRequirement(req, request.Name, requirement); err != nil {
			return nil, err
		} else if completed {
			continue
		}

		authURL, err := f.capturedAuthURL(req, request.Name, requirement, server, cfg)
		if err != nil {
			return nil, err
		}

		if authURL == "" {
			continue
		}

		seen[requirement.Component] = true
		result = append(result, pendingComponentAuth{
			CatalogEntryID: server.Spec.MCPServerCatalogEntryName,
			ComponentID:    server.Spec.VMCPComponentID,
			MCPServerID:    server.Name,
			Name:           pendingComponentDisplayName(server),
			Icon:           server.Spec.Manifest.Icon,
			AuthURL:        authURL,
		})
	}

	return result, nil
}

func (f *MCPOAuthHandlerFactory) capturedAuthURL(req api.Context, requestID string, requirement v1.UpstreamOAuthRequirement, server v1.MCPServer, cfg mcp.ServerConfig) (string, error) {
	key := upstreamRequirementKey(requirement)
	var (
		request v1.OAuthAuthRequest
		launch  bool
	)

	// Concurrent UI polls must claim the start before any provider-side work.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		launch = false
		if err := req.Get(&request, requestID); err != nil {
			return err
		}

		for i := range request.Spec.UpstreamOAuth {
			current := &request.Spec.UpstreamOAuth[i]
			if upstreamRequirementKey(*current) != key {
				continue
			}

			if current.Completed {
				return nil
			}

			if current.Failed {
				return types.NewErrBadRequest("upstream authentication failed; start a new authorization")
			}

			if current.Started {
				return nil
			}

			current.Started = true
			current.StartedAt = time.Now().Unix()
			if err := req.Update(&request); err != nil {
				return err
			}

			launch = true

			return nil
		}

		return nil
	}); err != nil {
		return "", err
	}

	if !launch {
		for _, current := range request.Spec.UpstreamOAuth {
			if upstreamRequirementKey(current) == key {
				if current.Completed {
					return "", nil
				}

				return f.pendingCapturedAuthURL(req, requestID, current, cfg)
			}
		}

		return "", types.NewErrBadRequest("upstream requirement is missing")
	}

	// Fresh tickets must share the same attempt budget for this user and target.
	binding := strings.Join([]string{
		req.User.GetUID(),
		request.Spec.MCPID,
		request.Spec.ClientID,
		requirement.Component,
		requirement.ConnectID,
		requirement.TargetHash,
	}, "\x00")
	budget := fmt.Sprintf("%x", sha256.Sum256([]byte(binding)))
	authURL, err := func() (string, error) {
		if err := req.GatewayClient.ReserveMCPOAuthAttempt(req.Context(), budget); err != nil {
			return "", err
		}

		oauthHandler, opt, err := f.prepareUpstreamOAuth(req, server, cfg, req.User.GetUID(), requirement.ConnectID, requestID)
		if err != nil {
			return "", err
		}

		oauthHandler.upstreamChallenge = key
		authURL, err := f.mcpSessionManager.StartOAuthFromChallenge(req.Context(), cfg, opt, requirement.ResourceMetadata, requirement.Scope)
		if err != nil {
			return "", err
		}

		return f.localLoginURL(authURL, cfg)
	}()

	if err != nil {
		// A disconnected caller must not leave the persisted setup claim stuck.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
		defer cancel()

		cleanupReq := req
		cleanupReq.Request = req.WithContext(cleanupCtx)
		if saveErr := updateCapturedRequirement(cleanupReq, requestID, key, func(current *v1.UpstreamOAuthRequirement) {
			// Setup failed before returning an authorization URL. Allow another poll
			// to retry, still subject to the shared per-target attempt budget.
			current.Started = false
			current.StartedAt = 0
		}); saveErr != nil {
			return "", errors.Join(err, saveErr)
		}

		return "", err
	}

	return authURL, nil
}

// Callbacks for different components update the same request; retry against the
// latest version so one completion cannot overwrite another.
func updateCapturedRequirement(req api.Context, requestID, key string, update func(*v1.UpstreamOAuthRequirement)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var request v1.OAuthAuthRequest
		if err := req.Get(&request, requestID); err != nil {
			return err
		}

		if request.Spec.UserID != req.UserID() {
			return types.NewErrForbidden("OAuth request belongs to another user")
		}

		for i := range request.Spec.UpstreamOAuth {
			if upstreamRequirementKey(request.Spec.UpstreamOAuth[i]) == key {
				update(&request.Spec.UpstreamOAuth[i])

				return req.Update(&request)
			}
		}

		return types.NewErrForbidden("upstream requirement is missing")
	})
}

func (f *MCPOAuthHandlerFactory) validateCapturedCallback(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) error {
	if pending.UpstreamChallenge == "" {
		return nil
	}

	if time.Since(pending.CreatedAt) > client.MCPOAuthPendingStateTTL {
		return types.NewErrBadRequest("upstream OAuth attempt expired")
	}

	// Route authorization admits logged-in users but does not bind the opaque
	// callback state to its owner. Check that binding before storing credentials.
	if req.User == nil || !req.UserIsAuthenticated() || pending.UserID != req.User.GetUID() {
		return types.NewErrForbidden("upstream OAuth attempt belongs to another user")
	}

	var request v1.OAuthAuthRequest
	if err := req.Get(&request, pending.OAuthAuthRequestID); err != nil {
		return err
	}

	if request.Spec.UserID != req.UserID() || !request.Spec.ConsentApproved {
		return types.NewErrForbidden("OAuth request is not approved for this user")
	}

	for _, requirement := range request.Spec.UpstreamOAuth {
		if upstreamRequirementKey(requirement) == pending.UpstreamChallenge && requirement.ConnectID == pending.MCPID && requirement.URL == pending.URL && requirement.Started && !requirement.Failed {
			_, _, aggregate, err := f.mcpSessionManager.ServerForActionWithConnectID(req.Context(), request.Spec.MCPID, req.User)
			if err != nil {
				return err
			}

			_, _, err = f.validateUpstreamRequirement(req, componentConnections(aggregate), requirement)

			return err
		}
	}

	return types.NewErrForbidden("upstream OAuth attempt no longer belongs to this connection")
}

// Completion follows token storage and a fresh membership check; a failed or
// duplicate callback must never clear an earlier successful completion.
func (f *MCPOAuthHandlerFactory) completeCapturedCallback(req api.Context, pending *gatewaytypes.MCPOAuthPendingState, callbackErr error) error {
	if pending.UpstreamChallenge == "" {
		return nil
	}

	if callbackErr == nil {
		if err := f.validateCapturedCallback(req, pending); err != nil {
			return err
		}
	}

	return updateCapturedRequirement(req, pending.OAuthAuthRequestID, pending.UpstreamChallenge, func(requirement *v1.UpstreamOAuthRequirement) {
		if requirement.Completed {
			return
		}

		requirement.Completed = callbackErr == nil
		requirement.Failed = callbackErr != nil
	})
}

// The token records the callback outcome atomically, so a failed progress write can be retried without exchanging the code again.
func reconcileCapturedRequirement(req api.Context, requestID string, requirement v1.UpstreamOAuthRequirement) (bool, error) {
	if requirement.Completed || !requirement.Started {
		return requirement.Completed, nil
	}

	grant, err := req.GatewayClient.GetMCPOAuthToken(req.Context(), req.User.GetUID(), requirement.ConnectID, requirement.URL)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if grant.URL != requirement.URL || grant.OAuthAuthRequestID != requestID || grant.UpstreamChallenge != upstreamRequirementKey(requirement) {
		return false, nil
	}

	err = updateCapturedRequirement(req, requestID, upstreamRequirementKey(requirement), func(current *v1.UpstreamOAuthRequirement) {
		current.Completed = true
		current.Failed = false
	})

	return err == nil, err
}

func (f *MCPOAuthHandlerFactory) pendingCapturedAuthURL(req api.Context, requestID string, requirement v1.UpstreamOAuthRequirement, cfg mcp.ServerConfig) (string, error) {
	pending, err := req.GatewayClient.FindMCPOAuthPendingState(req.Context(), req.User.GetUID(), requirement.ConnectID, cfg.URL, requestID, upstreamRequirementKey(requirement))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Concurrent polls can arrive before setup saves state. After a bounded
		// grace period, a crash or expired attempt requires a fresh authorization.
		const setupGracePeriod = 2 * time.Minute
		if time.Since(time.Unix(requirement.StartedAt, 0)) >= setupGracePeriod {
			return "", types.NewErrBadRequest("upstream authorization expired or was interrupted; start a new authorization")
		}

		return "", types.NewErrBadRequest("upstream authorization is being prepared; retry shortly")
	}

	if err != nil {
		return "", err
	}

	// Reuse the durable registration, state and PKCE verifier; never start another attempt.
	conf := &oauth2.Config{
		ClientID:    pending.ClientID,
		RedirectURL: pending.RedirectURL,
		Scopes:      strings.Fields(pending.Scopes),
		Endpoint:    oauth2.Endpoint{AuthURL: pending.AuthURL},
	}
	authURL, err := mcp.AuthCodeURL(conf, pending.AuthURL, pending.ResourceURL, pending.State, pending.Verifier)
	if err != nil {
		return "", err
	}

	return f.localLoginURL(authURL, cfg)
}
