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

// profileGrantBinding is what the pending authorization of a vMCP profile's grant is bound to.
type profileGrantBinding struct {
	// initiatorUserID is the encoded principal of the administrator who started the authorization. Only they can
	// complete it.
	initiatorUserID string
	// definitionHash is the component's vmcpconfig.ProfileOAuthGrantDefinitionHash when the authorization started.
	definitionHash string
}

// SetProfileOAuthGrants sets the store of the OAuth grants administrators authorize on vMCP profiles. Set it before
// the factory serves requests.
func (f *MCPOAuthHandlerFactory) SetProfileOAuthGrants(grants *vmcpconfig.ProfileOAuthGrantStore) {
	f.profileGrants = grants
}

// StartProfileOAuthGrant starts an administrator's authorization of the OAuth grant a vMCP profile holds for one of
// the vMCP's components, which every agent that resolves to the profile then uses headlessly. It returns the URL the
// administrator opens; the provider redirects back to the MCP OAuth callback (oauthCallback), which stores the grant
// (completeProfileGrantCallback).
//
// The profile must be agent-capable, and the component a remote server. The grant is bound to the upstream URL that an
// agent using the profile's grants reaches (profileGrantTargetURL): the URL the component's definition fixes, or else
// the one its URL configuration resolves to for the profile. The pending authorization is stored for the profile's
// grant owner, never the administrator, bound to the administrator, to that URL, and to the component's definition. The
// OAuth client is resolved as for any authorization: the catalog's static credentials first, then a client ID
// metadata document, then dynamic registration. The grant holds only the authorization; the static credentials stay
// the catalog's.
func (f *MCPOAuthHandlerFactory) StartProfileOAuthGrant(req api.Context, vmcpID, profileID, componentID string) (string, error) {
	if f.profileGrants == nil {
		return "", fmt.Errorf("vMCP profile OAuth grants are not configured")
	}
	initiator, ok := principal.FromUserInfo(req.User)
	if !ok || initiator.Kind != principal.KindUser || !req.UserIsAdmin() && !req.UserIsOwner() {
		return "", types.NewErrForbidden("only an administrator can authorize a vMCP profile's OAuth grant")
	}

	var vmcp v1.VMCP
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: vmcpID}, &vmcp); apierrors.IsNotFound(err) {
		return "", types.NewErrNotFound("vMCP %q not found", vmcpID)
	} else if err != nil {
		return "", err
	}
	profile, component, err := vmcpconfig.ProfileOAuthGrantTarget(&vmcp, profileID, componentID)
	if err != nil {
		return "", err
	}
	target, err := f.profileGrantTargetURL(req.Context(), vmcp, profile, component)
	if err != nil {
		return "", err
	}
	server, cfg, err := profileGrantServer(vmcp, component, target)
	if err != nil {
		return "", err
	}
	if cfg.TunnelName == "" {
		if err := mcp.ValidateRemoteMCPURL(req.Context(), cfg.URL, f.mcpSessionManager.RemoteMCPURLValidationConfig()); err != nil {
			return "", types.NewErrBadRequest("component %q has an upstream URL that cannot be reached: %v", component.ID, err)
		}
	}

	owner := principal.ProfileGrant(vmcp.Name, profileID)
	mcpID := vmcpconfig.ProfileOAuthGrantMCPID(vmcp.Name, component.ID)
	oauthHandler, opt, err := f.prepareUpstreamOAuth(req, server, cfg, owner, mcpID, "")
	if err != nil {
		return "", err
	}
	oauthHandler.profileGrant = &profileGrantBinding{
		initiatorUserID: initiator.String(),
		definitionHash:  vmcpconfig.ProfileOAuthGrantDefinitionHash(component),
	}

	authURL, err := f.mcpSessionManager.StartOAuthAuthorization(req.Context(), cfg, opt, server.Spec.Manifest.RemoteConfig.StaticOAuthRequired)
	if errors.Is(err, mcp.ErrOAuthNotRequired) {
		return "", types.NewErrBadRequest("component %q does not require OAuth", component.ID)
	} else if err != nil {
		return "", fmt.Errorf("start OAuth authorization of component %q: %w", component.ID, err)
	}
	slog.Info("Started the authorization of a vMCP profile's OAuth grant", "vmcp", vmcp.Name, "profile", profileID, "component", component.ID, "administrator", initiator.ID)
	return authURL, nil
}

// profileGrantTargetURL returns the upstream URL a profile's grant for component is bound to: the URL that the
// connection of an agent using the profile's grants reaches (vmcpconfig.ProfileGrantSelection). For a component whose
// definition fixes the URL, that is the fixed URL. Otherwise it is resolved from configuration as the agent's component
// server is configured (mcp.SessionManager.ComponentTarget): the profile's own values, then, for the required values
// the profile does not set, the wildcard profile's, or the wildcard profile's alone when the profile is it, and the
// vMCP's fixed values. A URL that needs a value no profile sets is refused until an administrator configures the
// profile; one that needs it from an ambiguous fallback is a conflict, as for the vMCP's other ambiguous cases; and one
// that does not satisfy the component's hostname is refused, as configuring it is.
func (f *MCPOAuthHandlerFactory) profileGrantTargetURL(ctx context.Context, vmcp v1.VMCP, profile types.VMCPProfile, component types.VMCPComponent) (string, error) {
	target, refused, err := vmcpconfig.ProfileGrantTarget(ctx, f.mcpSessionManager, vmcp, profile, component)
	switch {
	case err != nil:
		return "", err
	case refused != nil:
		return "", refused
	case target.AmbiguousFallback:
		return "", types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("the upstream URL of component %q needs a value that profile %q falls back for, and more than one profile of vMCP %q names every agent; leave at most one such profile first", component.ID, profile.ID, vmcp.Name))
	case target.URL == "":
		return "", types.NewErrBadRequest("the upstream URL of component %q needs %s, which profile %q does not configure; configure the profile first", component.ID, strings.Join(target.Missing, ", "), profile.ID)
	}
	return target.URL, nil
}

// profileGrantServer returns the server a profile's grant for component is authorized against, at target, the URL the
// grant is bound to (profileGrantTargetURL), and otherwise built from the component's definition alone: no component
// server is needed. Neither carries a configured header or credential: OAuth discovery and the code exchange never send
// them.
func profileGrantServer(vmcp v1.VMCP, component types.VMCPComponent, target string) (v1.MCPServer, mcp.ServerConfig, error) {
	remote := component.CatalogEntry.Manifest.RemoteConfig
	if remote.LocalhostCallbackEnabled {
		return v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("component %q authorizes through a localhost callback, which a profile's OAuth grant cannot use", component.ID)
	}
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
					URL:                 target,
					TunnelName:          remote.TunnelName,
					StaticOAuthRequired: remote.StaticOAuthRequired,
				},
			},
		},
	}
	return server, mcp.ServerConfig{
		Runtime:              types.RuntimeRemote,
		URL:                  target,
		TunnelName:           remote.TunnelName,
		MCPServerName:        vmcpconfig.ProfileOAuthGrantMCPID(vmcp.Name, component.ID),
		MCPServerDisplayName: component.Name,
		MCPServerNamespace:   vmcp.Namespace,
		MCPCatalogEntryName:  component.MCPServerCatalogEntryID,
		ComponentMCPServer:   true,
	}, nil
}

// completeProfileGrantCallback completes the authorization of a vMCP profile's grant at the MCP OAuth callback, for
// pending, the authorization its state names. The grant is stored only after every check below passes:
//
//   - The caller is the administrator who started the authorization, still an administrator. Anyone else is refused
//     without consuming the attempt, so its administrator can still complete it.
//   - The attempt has not expired, and is consumed, so no callback, from anyone, can use it again.
//   - The vMCP, the profile and the component still exist, the profile can still hold grants, and the component
//     still has the definition the authorization started with, and still reaches the same upstream URL for the
//     profile: a change to the profile's URL configuration in between refuses the grant. This is checked before the
//     code is exchanged, and again, from a fresh read, as the grant is stored (vmcpconfig.ProfileOAuthGrantStore.Authorize).
func (f *MCPOAuthHandlerFactory) completeProfileGrantCallback(req api.Context, pending *gatewaytypes.MCPOAuthPendingState) error {
	if f.profileGrants == nil {
		return fmt.Errorf("vMCP profile OAuth grants are not configured")
	}
	vmcpID, profileID, ok := principal.ParseProfileGrant(pending.UserID)
	componentID, hasComponent := strings.CutPrefix(pending.MCPID, vmcpID+"/")
	if !ok || !hasComponent || componentID == "" || pending.GrantInitiatorUserID == "" || pending.GrantDefinitionHash == "" {
		return types.NewErrForbidden("authentication attempt is not bound to an administrator")
	}
	if req.User == nil || !req.UserIsAuthenticated() || principal.ClaimsAgent(req.User) || req.User.GetUID() != pending.GrantInitiatorUserID {
		return types.NewErrForbidden("authentication attempt belongs to another administrator")
	}
	if !req.UserIsAdmin() && !req.UserIsOwner() {
		return types.NewErrForbidden("only an administrator can authorize a vMCP profile's OAuth grant")
	}
	if time.Since(pending.CreatedAt) > client.MCPOAuthPendingStateTTL {
		_ = f.stateMgr.gatewayClient.DeleteMCPOAuthPendingState(req.Context(), pending.HashedState)
		return types.NewErrBadRequest("authentication attempt expired; start a new authorization")
	}

	key := kclient.ObjectKey{Namespace: req.Namespace(), Name: vmcpID}
	check := func(vmcp *v1.VMCP, profile types.VMCPProfile, component types.VMCPComponent) error {
		changed := types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("component %q of vMCP %q changed since its authorization started; start a new authorization", component.ID, vmcp.Name))
		if vmcpconfig.ProfileOAuthGrantDefinitionHash(component) != pending.GrantDefinitionHash {
			return changed
		}
		target, err := f.profileGrantTargetURL(req.Context(), *vmcp, profile, component)
		if _, refused := errors.AsType[*types.ErrHTTP](err); refused || err == nil && target != pending.URL {
			// The URL the profile's agents reach is not the one the administrator authorized, or no longer resolves.
			return changed
		}
		return err
	}
	var vmcp v1.VMCP
	err := req.Storage.Get(req.Context(), key, &vmcp)
	if err == nil {
		var profile types.VMCPProfile
		var component types.VMCPComponent
		if profile, component, err = vmcpconfig.ProfileOAuthGrantTarget(&vmcp, profileID, componentID); err == nil {
			err = check(&vmcp, profile, component)
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

	consumed, err := f.stateMgr.gatewayClient.ConsumeMCPOAuthPendingState(req.Context(), req.URL.Query().Get("state"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrBadRequest("authentication attempt already consumed")
	} else if err != nil {
		return fmt.Errorf("failed to get oauth state: %w", err)
	}
	if consumed.UserID != pending.UserID || consumed.MCPID != pending.MCPID || consumed.GrantInitiatorUserID != pending.GrantInitiatorUserID {
		return types.NewErrForbidden("authentication attempt changed")
	}
	if errorCode := req.URL.Query().Get("error"); errorCode != "" {
		if errorCode == string(ErrAccessDenied) {
			return types.NewErrBadRequest("upstream authorization was denied")
		}
		return types.NewErrBadRequest("upstream authentication failed")
	}

	conf, token, err := f.stateMgr.exchange(req.Context(), consumed, req.URL.Query().Get("code"))
	if err != nil {
		slog.Warn("Failed to exchange the code of a vMCP profile's OAuth grant", "vmcp", vmcpID, "profile", profileID, "component", componentID, "error", err)
		return types.NewErrBadRequest("upstream authentication failed")
	}
	if _, err := f.profileGrants.Authorize(req.Context(), req.Storage, key, profileID, componentID, check, func(ctx context.Context, owner, mcpID, grantID string) error {
		return f.stateMgr.gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, consumed.URL, grantID, consumed.GrantInitiatorUserID, conf, token)
	}); err != nil {
		return err
	}

	slog.Info("Stored a vMCP profile's OAuth grant", "vmcp", vmcpID, "profile", profileID, "component", componentID, "administrator", consumed.GrantInitiatorUserID, "scopes", strings.Join(conf.Scopes, " "))
	http.Redirect(req.ResponseWriter, req.Request, "/auth/oauth/complete", http.StatusFound)
	return nil
}
