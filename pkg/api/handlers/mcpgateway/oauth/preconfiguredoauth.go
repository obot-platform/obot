package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// preconfiguredOAuthBinding is what the pending authorization of a vMCP component's preconfigured OAuth is bound to.
type preconfiguredOAuthBinding struct {
	// initiatorUserID is the encoded principal of the administrator who started the authorization. Only they can
	// complete it.
	initiatorUserID string
	// definitionHash is the component's vmcpconfig.PreconfiguredOAuthDefinitionHash when the authorization started.
	definitionHash string
}

// SetPreconfiguredOAuth sets the store of the OAuth grants administrators preconfigure for the remote components of
// shared vMCPs. Set it before the factory serves requests.
func (f *MCPOAuthHandlerFactory) SetPreconfiguredOAuth(grants *vmcpconfig.PreconfiguredOAuthStore) {
	f.preconfiguredOAuth = grants
}

// StartPreconfiguredOAuth starts an administrator's authorization of the OAuth a shared vMCP preconfigures for one of
// its remote components, which everyone who reaches the component through the vMCP then uses: every agent a profile
// grants it, headlessly, and every user connecting to the vMCP themselves, in place of an account of their own. It
// returns the URL the administrator opens; the provider redirects back to the MCP OAuth callback (oauthCallback), which
// stores the grant (completePreconfiguredOAuthCallback).
//
// Who may call it is decided by pkg/api/authz: only administrators and owners reach its route. Any remote component of
// a shared vMCP can be authorized (vmcpconfig.PreconfiguredOAuthTarget), whichever profiles grant it. The grant is
// bound to the upstream URL that the component's preconfigured OAuth is bound to (preconfiguredOAuthTargetURL): the URL
// the component's definition fixes, or else the one built from the values the vMCP and the catalog entry fix. The
// pending authorization is stored for the vMCP's grant owner, never the administrator, bound to the administrator, to
// that URL, and to the component's definition. The OAuth client is resolved as for any authorization: the catalog's
// static credentials first, then a client ID metadata document, then dynamic registration. The grant holds only the
// authorization; the static credentials stay the catalog's.
//
// How the component asks for OAuth is learned with unauthenticated requests to that URL, which carry none of its
// configured headers and no grant's token (mcp.SessionManager.StartOAuthAuthorization): a challenge to initialize, a
// challenge to tools/list for a server that only challenges once tools are requested, or else the protected resource
// metadata the server publishes, for one that only challenges when a tool is called. A component that shows none of
// them is refused as not requiring OAuth.
//
// A component whose provider only redirects to a localhost callback is authorized through the same relay as a user's
// own connection: the callback URL is a random loopback port (upstreamRedirectURL), the pending state is a local login
// state, and the administrator gets Obot's local login URL (localLoginURL), which `obot mcp login` opens and relays the
// provider's callback from (LocalhostCallback). Everything else about the grant is the same.
func (f *MCPOAuthHandlerFactory) StartPreconfiguredOAuth(req api.Context, vmcpID, componentID string) (types.VMCPPreconfiguredOAuthAuthorization, error) {
	var none types.VMCPPreconfiguredOAuthAuthorization
	if f.preconfiguredOAuth == nil {
		return none, fmt.Errorf("vMCP preconfigured OAuth is not configured")
	}
	// Authorization admitted only administrators and owners. The grant records who started it, a user's encoded
	// principal, and only they can complete it.
	initiator, ok := principal.FromUserInfo(req.User)
	if !ok || initiator.Kind != principal.KindUser {
		return none, types.NewErrForbidden("only an administrator can authorize a vMCP's preconfigured OAuth")
	}

	var vmcp v1.VMCP
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: vmcpID}, &vmcp); apierrors.IsNotFound(err) {
		return none, types.NewErrNotFound("vMCP %q not found", vmcpID)
	} else if err != nil {
		return none, err
	}
	component, err := vmcpconfig.PreconfiguredOAuthTarget(&vmcp, componentID)
	if err != nil {
		return none, err
	}
	target, err := f.preconfiguredOAuthTargetURL(req.Context(), vmcp, component)
	if err != nil {
		return none, err
	}
	// A URL that comes from the vMCP's fixed configuration is never put into an error, which reaches the administrator
	// who started the authorization, and may be seen by those who do not configure it.
	var redactURL string
	if _, fixed := vmcpconfig.StaticComponentTargetURL(component); !fixed {
		redactURL = target
	}
	server, cfg := preconfiguredOAuthServer(vmcp, component, target)
	authURL, err := f.startComponentOAuth(req, server, cfg, principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpconfig.PreconfiguredOAuthMCPID(vmcp.Name, component.ID),
		component.ID, server.Spec.Manifest.RemoteConfig.StaticOAuthRequired, redactURL, func(oauthHandler *mcpOAuthHandler) {
			oauthHandler.preconfiguredOAuth = &preconfiguredOAuthBinding{
				initiatorUserID: initiator.String(),
				definitionHash:  vmcpconfig.PreconfiguredOAuthDefinitionHash(component),
			}
		})
	if err != nil {
		return none, err
	}
	slog.Info("Started the authorization of a vMCP's preconfigured OAuth grant", "vmcp", vmcp.Name, "component", component.ID, "administrator", initiator.ID, "localhostCallback", cfg.LocalhostCallbackEnabled)
	return types.VMCPPreconfiguredOAuthAuthorization{AuthURL: authURL, LocalhostCallback: cfg.LocalhostCallbackEnabled}, nil
}

// startComponentOAuth starts the authorization of the upstream account of the component with componentID, reached with
// server and cfg, which its callback stores for owner and mcpID, and returns the URL to open. bind binds the pending
// authorization to who started it. It is the start of a vMCP's preconfigured OAuth and of a user's setup through an
// agent. Errors never carry redactURL, when it is set: a URL from configuration that whoever started it may not see.
func (f *MCPOAuthHandlerFactory) startComponentOAuth(req api.Context, server v1.MCPServer, cfg mcp.ServerConfig, owner, mcpID, componentID string, staticOAuth bool, redactURL string, bind func(*mcpOAuthHandler)) (string, error) {
	redact := func(err error) error {
		if redactURL == "" {
			return err
		}
		if httpErr, isHTTP := errors.AsType[*types.ErrHTTP](err); isHTTP {
			return types.NewErrHTTP(httpErr.Code, vmcpconfig.RedactURL(httpErr.Message, redactURL))
		}
		return errors.New(vmcpconfig.RedactURL(err.Error(), redactURL))
	}
	if cfg.TunnelName == "" {
		if err := mcp.ValidateRemoteMCPURL(req.Context(), cfg.URL, f.mcpSessionManager.RemoteMCPURLValidationConfig()); err != nil {
			return "", redact(types.NewErrBadRequest("component %q has an upstream URL that cannot be reached: %v", componentID, err))
		}
	}

	oauthHandler, opt, err := f.prepareUpstreamOAuth(req, server, cfg, owner, mcpID, "")
	if err != nil {
		return "", redact(err)
	}
	bind(oauthHandler)

	authURL, err := f.mcpSessionManager.StartOAuthAuthorization(req.Context(), cfg, opt, staticOAuth)
	if errors.Is(err, mcp.ErrOAuthNotRequired) {
		return "", types.NewErrBadRequest("component %q does not require OAuth: it answered without asking for authorization, and publishes no OAuth protected resource metadata", componentID)
	} else if err != nil {
		return "", redact(fmt.Errorf("start OAuth authorization of component %q: %w", componentID, err))
	}
	// For a localhost callback, whoever started it gets the relay's local login URL, as a user does.
	if authURL, err = f.localLoginURL(authURL, cfg); err != nil {
		return "", redact(err)
	}
	return authURL, nil
}

// unexpiredAttempt returns whether pending, an attempt that a preconfigured OAuth or delegated setup callback
// completes, is relayed by `obot mcp login` from a localhost callback. A relayed attempt must have the relay's
// loopback-only redirect, and lives only as long as the relay waits for it. An expired attempt is deleted and refused.
func (f *MCPOAuthHandlerFactory) unexpiredAttempt(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) (bool, error) {
	relayed := strings.HasPrefix(pending.State, localLoginStatePrefix)
	if relayed && !localLoginState(pending) {
		return false, types.NewErrForbidden("authentication attempt has an invalid localhost callback")
	}
	ttl := client.MCPOAuthPendingStateTTL
	if relayed {
		ttl = localLoginTTL
	}
	if time.Since(pending.CreatedAt) > ttl {
		_ = f.stateMgr.gatewayClient.DeleteMCPOAuthPendingState(req.Context(), pending.HashedState)
		return false, types.NewErrBadRequest("authentication attempt expired; start a new authorization")
	}
	return relayed, nil
}

// consumeAttempt consumes the attempt that the callback's state names, so that no callback, from anyone, can use it
// again, and refuses it unless it is still pending, as the callback checked it.
func (f *MCPOAuthHandlerFactory) consumeAttempt(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) (*gatewaytypes.MCPOAuthPendingState, error) {
	consumed, err := f.stateMgr.gatewayClient.ConsumeMCPOAuthPendingState(req.Context(), req.URL.Query().Get("state"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrBadRequest("authentication attempt already consumed")
	} else if err != nil {
		return nil, fmt.Errorf("failed to get oauth state: %w", err)
	}
	if consumed.UserID != pending.UserID || consumed.MCPID != pending.MCPID || consumed.GrantInitiatorUserID != pending.GrantInitiatorUserID || consumed.ReturnPath != pending.ReturnPath {
		return nil, types.NewErrForbidden("authentication attempt changed")
	}
	return consumed, nil
}

// preconfiguredOAuthTargetURL returns the upstream URL the vMCP's grant for component is bound to
// (vmcpconfig.ResolveComponentTarget). For a component whose definition fixes the URL, that is the fixed URL. Otherwise
// it is resolved as an agent's component server is configured (mcp.SessionManager.ComponentTarget): from the values the
// vMCP and the catalog entry fix, which are everyone's. A URL that needs a value neither fixes is refused until an
// administrator fixes it, since one account cannot be bound to a URL each user configures, and one that does not
// satisfy the component's hostname is refused, as fixing it is.
func (f *MCPOAuthHandlerFactory) preconfiguredOAuthTargetURL(ctx context.Context, vmcp v1.VMCP, component types.VMCPComponent) (string, error) {
	target, refused, err := vmcpconfig.ResolveComponentTarget(ctx, f.mcpSessionManager, vmcp, component)
	switch {
	case err != nil:
		return "", err
	case refused != nil:
		return "", refused
	case target.URL == "":
		// The keys are named, never values, so an administrator is pointed at the exact fields to set.
		return "", api.NewErrHTTPWithReasons(
			http.StatusBadRequest,
			fmt.Sprintf("the upstream URL of component %q needs %s, which is not fixed for everyone: set it as fixed on the vMCP or on the catalog entry first", component.ID, strings.Join(target.Missing, ", ")),
			vmcpconfig.ConfigurationRequiredReasons(component.ID, target.Missing),
		)
	}
	return target.URL, nil
}

// preconfiguredOAuthServer returns the server the vMCP's grant for component is authorized against, at target, the URL
// the grant is bound to (preconfiguredOAuthTargetURL), and otherwise built from the component's definition alone,
// including whether its provider redirects to a localhost callback: no component server is needed. Neither carries a
// configured header or credential: OAuth discovery and the code exchange never send them.
func preconfiguredOAuthServer(vmcp v1.VMCP, component types.VMCPComponent, target string) (v1.MCPServer, mcp.ServerConfig) {
	remote := component.CatalogEntry.Manifest.RemoteConfig
	server := v1.MCPServer{
		Namespace: vmcp.Namespace,
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: component.MCPServerCatalogEntryID,
			VMCPID:                    vmcp.Name,
			VMCPComponentID:           component.ID,
			Manifest: types.MCPServerManifest{
				Name:    component.Name,
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:                      target,
					TunnelName:               remote.TunnelName,
					StaticOAuthRequired:      remote.StaticOAuthRequired,
					LocalhostCallbackEnabled: remote.LocalhostCallbackEnabled,
					LocalhostCallbackPath:    remote.LocalhostCallbackPath,
				},
			},
		},
	}
	return server, mcp.ServerConfig{
		Runtime:                  types.RuntimeRemote,
		URL:                      target,
		TunnelName:               remote.TunnelName,
		LocalhostCallbackEnabled: remote.LocalhostCallbackEnabled,
		LocalhostCallbackPath:    remote.LocalhostCallbackPath,
		MCPServerName:            vmcpconfig.PreconfiguredOAuthMCPID(vmcp.Name, component.ID),
		MCPServerDisplayName:     component.Name,
		MCPServerNamespace:       vmcp.Namespace,
		MCPCatalogEntryName:      component.MCPServerCatalogEntryID,
		ComponentMCPServer:       true,
	}
}

// completePreconfiguredOAuthCallback completes the authorization of a vMCP component's preconfigured OAuth at the MCP
// OAuth callback, for pending, the authorization its state names. The grant is stored only after every check below
// passes:
//
//   - The caller is the administrator who started the authorization, still an administrator. Anyone else is refused
//     without consuming the attempt, so its administrator can still complete it.
//   - The attempt has not expired, and is consumed, so no callback, from anyone, can use it again.
//   - The vMCP and the component still exist, the component can still hold preconfigured OAuth, and the
//     component still has the definition the authorization started with, and still reaches the same upstream URL: a
//     change to the URL the vMCP fixes in between refuses the grant. This is checked before the code is exchanged, and
//     again, from a fresh read, as the grant is stored (vmcpconfig.PreconfiguredOAuthStore.Authorize).
func (f *MCPOAuthHandlerFactory) completePreconfiguredOAuthCallback(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) error {
	if f.preconfiguredOAuth == nil {
		return fmt.Errorf("vMCP preconfigured OAuth grants are not configured")
	}
	vmcpID, ok := principal.ParsePreconfiguredOAuthOwner(pending.UserID)
	componentID, hasComponent := strings.CutPrefix(pending.MCPID, vmcpID+"/")
	if !ok || !hasComponent || componentID == "" || pending.GrantInitiatorUserID == "" || pending.GrantDefinitionHash == "" {
		return types.NewErrForbidden("authentication attempt is not bound to an administrator")
	}
	if req.User == nil || !req.UserIsAuthenticated() || principal.ClaimsAgent(req.User) || req.User.GetUID() != pending.GrantInitiatorUserID {
		return types.NewErrForbidden("authentication attempt belongs to another administrator")
	}
	// The callback route is every user's MCP OAuth callback, so pkg/api/authz cannot tell a grant's callback from a
	// user's own: only the pending state, read here, binds it to an administrator. That administrator must still be
	// one.
	if !req.UserIsAdmin() && !req.UserIsOwner() {
		return types.NewErrForbidden("only an administrator can authorize a vMCP's preconfigured OAuth")
	}
	relayed, err := f.unexpiredAttempt(req, pending)
	if err != nil {
		return err
	}

	key := kclient.ObjectKey{Namespace: req.Namespace(), Name: vmcpID}
	check := func(vmcp *v1.VMCP, component types.VMCPComponent) error {
		changed := types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("component %q of vMCP %q changed since its authorization started; start a new authorization", component.ID, vmcp.Name))
		if vmcpconfig.PreconfiguredOAuthDefinitionHash(component) != pending.GrantDefinitionHash {
			return changed
		}
		target, err := f.preconfiguredOAuthTargetURL(req.Context(), *vmcp, component)
		if _, refused := errors.AsType[*types.ErrHTTP](err); refused || err == nil && target != pending.URL {
			// The URL the grant would be bound to is not the one the administrator authorized, or no longer resolves.
			return changed
		}
		return err
	}
	var vmcp v1.VMCP
	err = req.Storage.Get(req.Context(), key, &vmcp)
	if err == nil {
		var component types.VMCPComponent
		if component, err = vmcpconfig.PreconfiguredOAuthTarget(&vmcp, componentID); err == nil {
			err = check(&vmcp, component)
		}
	} else if apierrors.IsNotFound(err) {
		err = types.NewErrNotFound("vMCP %q not found", vmcpID)
	}
	if err != nil {
		if _, refused := errors.AsType[*types.ErrHTTP](err); refused {
			// The attempt can never succeed, so it is not left for anyone. A failure to check is left to retry.
			_ = f.stateMgr.gatewayClient.DeleteMCPOAuthPendingState(req.Context(), pending.HashedState)
		}
		return err
	}

	consumed, err := f.consumeAttempt(req, pending)
	if err != nil {
		return err
	}
	if errorCode := req.URL.Query().Get("error"); errorCode != "" {
		if errorCode == string(ErrAccessDenied) {
			return types.NewErrBadRequest("upstream authorization was denied")
		}
		return types.NewErrBadRequest("upstream authentication failed")
	}

	conf, token, err := f.stateMgr.exchange(req.Context(), consumed, req.URL.Query().Get("code"))
	if err != nil {
		slog.Warn("Failed to exchange the code of a vMCP's preconfigured OAuth grant", "vmcp", vmcpID, "component", componentID, "error", err)
		return types.NewErrBadRequest("upstream authentication failed")
	}
	if _, err := f.preconfiguredOAuth.Authorize(req.Context(), req.Storage, key, componentID, check, func(ctx context.Context, owner, mcpID, grantID string) error {
		return f.stateMgr.gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, consumed.URL, grantID, consumed.GrantInitiatorUserID, conf, token)
	}); err != nil {
		return err
	}

	// Recorded once the grant is stored and published, as the administrator who authorized it. The token row keeps only
	// the latest authorization, so this is the history. The target is its hostname: the URL may carry configured values.
	f.stateMgr.gatewayClient.RecordAdminAuditEvent(req.Context(), client.AdminAuditEventInput{
		Actor:        consumed.GrantInitiatorUserID,
		Action:       gatewaytypes.AdminAuditActionVMCPPreconfiguredOAuthAuthorize,
		ResourceKind: gatewaytypes.AdminAuditResourceVMCP,
		ResourceID:   vmcpID,
		Details: map[string]any{
			"componentID": componentID,
			"scopes":      conf.Scopes,
			"hostname":    vmcpconfig.URLHostname(consumed.URL),
		},
	})
	slog.Info("Stored a vMCP's preconfigured OAuth grant", "vmcp", vmcpID, "component", componentID, "administrator", consumed.GrantInitiatorUserID, "scopes", strings.Join(conf.Scopes, " "))
	if relayed {
		// Tell the waiting relay that the authorization is complete.
		completeLocalLogin(req, consumed, nil)
		return nil
	}
	http.Redirect(req.ResponseWriter, req.Request, "/auth/oauth/complete", http.StatusFound)
	return nil
}
