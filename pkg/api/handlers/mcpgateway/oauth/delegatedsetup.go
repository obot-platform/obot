package oauth

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// delegatedSetupBinding is what the pending authorization of a user's upstream account through an agent is bound to.
type delegatedSetupBinding struct {
	// initiatorUserID is the encoded principal of the user who started it, the pair's own user. Only they can complete
	// it.
	initiatorUserID string
	// returnPath is the Obot page their browser returns to: the approval page or the page where they manage the agent.
	returnPath string
}

// StartDelegatedSetupOAuth starts the authorization of the upstream account that the user of pair, the instance of
// what the user set up for vmcp through an agent, connects for the component with componentID, and returns the URL the
// user opens. It is the user component OAuth flow, with the pair as the owner: the account is authorized against the
// pair's own connection to the component (mcp.SessionManager.PairComponentServer), as a user's is against theirs, and
// stored for the pair and that connection, where an agent acting on behalf of the user finds it. The provider redirects
// back to the MCP OAuth callback, which stores it (completeDelegatedSetupCallback) only for the user who started it,
// and sends their browser back to returnPath, the page that started it.
//
// The caller is the pair's user, which the authorizer and the setup API establish. The component must be one that the
// user's setup covers: the setup API checks that the agent is granted it, and that the vMCP holds no preconfigured
// OAuth grant for it, which the agent would use in place of the user's account. A component whose provider only
// redirects to a localhost callback is authorized through the same relay as a user's own connection.
func (f *MCPOAuthHandlerFactory) StartDelegatedSetupOAuth(req api.Context, vmcp *v1.VMCP, pair *v1.VMCPInstance, componentID, returnPath string) (types.AgentDelegationSetupOAuth, error) {
	var none types.AgentDelegationSetupOAuth
	owner, ok := vmcpconfig.PairOwner(*pair)
	if !ok {
		return none, fmt.Errorf("VMCP instance %q is not a user's setup through an agent", pair.Name)
	}
	initiator, ok := principal.FromUserInfo(req.User)
	if userID, isUser := initiator.UserID(); !ok || !isUser || userID != owner.UserID {
		return none, types.NewErrForbidden("only the user can connect an upstream account through the agent")
	}

	server, cfg, missing, err := f.mcpSessionManager.PairComponentServer(req.Context(), vmcp, pair, componentID)
	if err != nil {
		return none, err
	}
	if cfg.Runtime != types.RuntimeRemote {
		return none, types.NewErrBadRequest("component %q is not a remote server, so it does not use OAuth", componentID)
	}
	if len(missing) > 0 {
		return none, types.NewErrBadRequest("component %q needs %s before its upstream account can be connected", componentID, strings.Join(missing, ", "))
	}
	staticOAuth := server.Spec.Manifest.RemoteConfig != nil && server.Spec.Manifest.RemoteConfig.StaticOAuthRequired
	// The URL may come from the vMCP's fixed configuration, which users never see, so it is never put into an error.
	authURL, err := f.startComponentOAuth(req, server, cfg, pair.Spec.UserID, cmp.Or(cfg.MCPServerInstanceID, cfg.MCPServerName), componentID, staticOAuth, cfg.URL, func(oauthHandler *mcpOAuthHandler) {
		oauthHandler.delegatedSetup = &delegatedSetupBinding{
			initiatorUserID: initiator.String(),
			returnPath:      returnPath,
		}
	})
	if err != nil {
		return none, err
	}
	slog.Info("Started the authorization of a user's upstream account through an agent", "vmcp", vmcp.Name, "component", componentID, "agent", owner.AgentID, "user", owner.UserID, "localhostCallback", cfg.LocalhostCallbackEnabled)
	return types.AgentDelegationSetupOAuth{AuthURL: authURL, LocalhostCallback: cfg.LocalhostCallbackEnabled}, nil
}

// completeDelegatedSetupCallback completes, at the MCP OAuth callback, the authorization of a user's upstream account
// through an agent, for pending, the authorization its state names. The account is stored only after every check
// below passes:
//
//   - The caller is the signed-in user who started the authorization, the pair's own user. Anyone else is refused
//     without consuming the attempt, so its user can still complete it.
//   - The attempt has not expired, and is consumed, so no callback, from anyone, can use it again.
//   - The pair's connection that the attempt is for still exists: what a user set up through an agent is removed when
//     the delegation is revoked, and an account authorized in between is not stored for it.
//
// The user's browser goes back to the page that started the authorization, with oauth_error set when the provider
// refused it or the code could not be exchanged.
func (f *MCPOAuthHandlerFactory) completeDelegatedSetupCallback(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) error {
	owner, ok := principal.ParseDelegatedOwner(pending.UserID)
	if !ok || pending.GrantInitiatorUserID == "" || pending.GrantInitiatorUserID != principal.User(owner.UserID).String() || !localReturnPath(pending.ReturnPath) {
		return types.NewErrForbidden("authentication attempt is not bound to a user")
	}
	if req.User == nil || !req.UserIsAuthenticated() || principal.ClaimsAgent(req.User) || req.User.GetUID() != pending.GrantInitiatorUserID {
		return types.NewErrForbidden("authentication attempt belongs to another user")
	}
	if p, ok := principal.FromUserInfo(req.User); !ok || p.String() != pending.GrantInitiatorUserID {
		return types.NewErrForbidden("authentication attempt belongs to another user")
	}
	relayed, err := f.unexpiredAttempt(req, pending)
	if err != nil {
		return err
	}
	if ok, err := f.pairConnectionExists(req, pending.UserID, pending.MCPID); err != nil {
		return err
	} else if !ok {
		_ = f.stateMgr.gatewayClient.DeleteMCPOAuthPendingState(req.Context(), pending.HashedState)
		return types.NewErrHTTP(http.StatusConflict, "the setup this authorization was for was removed; start a new authorization")
	}

	consumed, err := f.consumeAttempt(req, pending)
	if err != nil {
		return err
	}

	finish := func(oauthErr error, code string) error {
		if relayed {
			completeLocalLogin(req, consumed, oauthErr)
			return nil
		}
		target := consumed.ReturnPath
		if code != "" {
			target += "?" + url.Values{"oauth_error": {code}}.Encode()
		}
		http.Redirect(req.ResponseWriter, req.Request, target, http.StatusFound)
		return nil
	}
	if errorCode := req.URL.Query().Get("error"); errorCode != "" {
		if errorCode == string(ErrAccessDenied) {
			return finish(errors.New("upstream authorization was denied"), string(ErrAccessDenied))
		}
		return finish(errors.New("upstream authentication failed"), string(ErrServerError))
	}
	conf, token, err := f.stateMgr.exchange(req.Context(), consumed, req.URL.Query().Get("code"))
	if err != nil {
		slog.Warn("Failed to exchange the code of a user's upstream account through an agent", "agent", owner.AgentID, "user", owner.UserID, "mcpID", consumed.MCPID, "error", err)
		return finish(errors.New("upstream authentication failed"), string(ErrServerError))
	}
	if err := f.stateMgr.gatewayClient.ReplaceMCPOAuthToken(req.Context(), consumed.UserID, consumed.MCPID, consumed.URL, "", conf, token); err != nil {
		return err
	}
	slog.Info("Stored a user's upstream account through an agent", "agent", owner.AgentID, "user", owner.UserID, "mcpID", consumed.MCPID)
	return finish(nil, "")
}

// pairConnectionExists reports whether connectID still names a connection of the pair with the encoded owner: its
// connection to a component's shared server, or its own component server, not being deleted.
func (f *MCPOAuthHandlerFactory) pairConnectionExists(req api.Context, owner, connectID string) (bool, error) {
	key := kclient.ObjectKey{Namespace: req.Namespace(), Name: connectID}
	var (
		obj      kclient.Object
		ownerOf  func() string
		instance func() string
	)
	switch {
	case system.IsMCPServerInstanceID(connectID):
		connection := &v1.MCPServerInstance{}
		obj, ownerOf, instance = connection, func() string { return connection.Spec.UserID }, func() string { return connection.Spec.VMCPInstanceID }
	case system.IsMCPServerID(connectID):
		server := &v1.MCPServer{}
		obj, ownerOf, instance = server, func() string { return server.Spec.UserID }, func() string { return server.Spec.VMCPInstanceID }
	default:
		return false, nil
	}
	if err := req.Storage.Get(req.Context(), key, obj); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return ownerOf() == owner && instance() != "" && obj.GetDeletionTimestamp().IsZero(), nil
}

// localReturnPath reports whether path is the path of an Obot page: absolute, and on no other host.
func localReturnPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.Contains(path, `\`)
}
