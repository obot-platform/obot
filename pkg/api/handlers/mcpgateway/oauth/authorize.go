package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/auth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrorCode defines the set of OAuth 2.0 error codes as per RFC 6749.
type ErrorCode string

const (
	ErrInvalidRequest          ErrorCode = "invalid_request"
	ErrUnauthorizedClient      ErrorCode = "unauthorized_client"
	ErrAccessDenied            ErrorCode = "access_denied"
	ErrUnsupportedResponseType ErrorCode = "unsupported_response_type"
	ErrInvalidScope            ErrorCode = "invalid_scope"
	ErrServerError             ErrorCode = "server_error"
	ErrTemporarilyUnavailable  ErrorCode = "temporarily_unavailable"
	ErrInvalidClientMetadata   ErrorCode = "invalid_client_metadata"
)

// Error represents an OAuth 2.0 error response.
type Error struct {
	Code        ErrorCode `json:"error"`
	Description string    `json:"error_description,omitempty"`
	State       string    `json:"state,omitempty"`
}

func (e Error) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return string(e.Code) + ": " + e.Description
	}
	return string(b)
}

func (e Error) toQuery() url.Values {
	q := url.Values{}
	q.Set("error", string(e.Code))
	if e.Description != "" {
		q.Set("error_description", e.Description)
	}
	if e.State != "" {
		q.Set("state", e.State)
	}
	return q
}

func (h *handler) authorize(req api.Context) error {
	if err := req.ParseForm(); err != nil {
		return err
	}

	state := req.FormValue("state")
	codeChallenge := req.FormValue("code_challenge")
	codeChallengeMethod := req.FormValue("code_challenge_method")
	if codeChallenge != "" && (codeChallengeMethod == "" || !slices.Contains(h.oauthConfig.CodeChallengeMethodsSupported, codeChallengeMethod)) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "code_challenge_method is invalid",
			State:       state,
		})
	}

	clientID := req.FormValue("client_id")
	if clientID == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client_id is required",
			State:       state,
		})
	}

	redirectURI := req.FormValue("redirect_uri")
	if redirectURI == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "redirect_uri is required",
			State:       state,
		})
	}

	responseType := req.FormValue("response_type")
	if responseType == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "response_type is required",
			State:       state,
		})
	}
	if !slices.Contains(h.oauthConfig.ResponseTypesSupported, responseType) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "response_type is invalid",
			State:       state,
		})
	}

	oauthClient, err := h.resolveOAuthClient(req.Context(), req.Storage, clientID)
	if err != nil {
		if oauthErr, ok := errors.AsType[Error](err); ok {
			oauthErr.State = state
			return types.NewErrBadRequest("%v", oauthErr)
		}
		return err
	}

	if !isRedirectURIAllowed(oauthClient.Spec.Manifest, redirectURI) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "redirect_uri is invalid for this client",
			State:       state,
		})
	}

	if len(oauthClient.Spec.Manifest.ResponseTypes) > 0 && !slices.Contains(oauthClient.Spec.Manifest.ResponseTypes, responseType) || len(oauthClient.Spec.Manifest.ResponseTypes) == 0 && responseType != "code" {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrUnsupportedResponseType,
			Description: "response_type is not allowed for this client",
			State:       state,
		})
		return nil
	}

	if oauthClient.Spec.Manifest.TokenEndpointAuthMethod == "none" && codeChallenge == "" {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "code_challenge is required when using token endpoint auth method none",
		})
		return nil
	}

	scope := req.FormValue("scope")
	if scope != "" {
		var (
			supported []string
			scopes    = make(map[string]struct{})
		)
		for s := range strings.SplitSeq(oauthClient.Spec.Manifest.Scope, " ") {
			scopes[s] = struct{}{}
		}

		for s := range strings.SplitSeq(scope, " ") {
			if _, ok := scopes[s]; s != "" && ok {
				supported = append(supported, s)
			}
		}

		scope = strings.Join(supported, " ")
	}

	mcpID := req.PathValue("mcp_id")
	resource := req.FormValue("resource")
	if resource != "" {
		u, err := url.Parse(resource)
		if err != nil {
			redirectWithAuthorizeError(req, redirectURI, Error{
				Code:        ErrInvalidRequest,
				Description: fmt.Sprintf("invalid resource URL: %s", resource),
				State:       state,
			})
			return nil
		}

		if mcpID == "" {
			mcpID = strings.TrimPrefix(u.Path, "/mcp-connect/")
		} else if !strings.HasSuffix(u.Path, "/"+mcpID) {
			redirectWithAuthorizeError(req, redirectURI, Error{
				Code:        ErrInvalidRequest,
				Description: fmt.Sprintf("resource doesn't match mcp_id: %s", mcpID),
				State:       state,
			})
			return nil
		}
	}

	if mcpID == "" {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "cannot determine MCP server, resource parameter required",
			State:       state,
		})
		return nil
	}

	oauthAppAuthRequest := v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.OAuthAuthRequestPrefix,
			Namespace:    oauthClient.Namespace,
		},
		Spec: v1.OAuthAuthRequestSpec{
			Scope:               scope,
			Resource:            resource,
			State:               state,
			ClientID:            oauthClient.Name,
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			GrantType:           "authorization_code",
			MCPID:               mcpID,
		},
	}

	if err := req.Create(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       state,
		})

		return nil
	}

	log.Infof("Created OAuth authorization request and redirecting user to authenticate: authRequest=%s client=%s requestedMCPID=%s", oauthAppAuthRequest.Name, oauthClient.Name, mcpID)
	http.Redirect(req.ResponseWriter, req.Request, fmt.Sprintf("/?rd=/oauth/callback/%s", oauthAppAuthRequest.Name), http.StatusFound)

	return nil
}

// callback handles the OAuth callback for the first-level Obot-based OAuth.
func (h *handler) callback(req api.Context) error {
	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, req.PathValue("oauth_auth_request")); err != nil {
		return err
	}

	authProviderName, authProviderNamespace, ok := authenticatedOAuthUser(req, oauthAppAuthRequest, "OAuth callback")
	if !ok {
		return nil
	}

	mcpID := oauthAppAuthRequest.Spec.MCPID
	if mcpID != "" {
		serverOrInstanceID, audience, err := handlers.MCPIDAndAudienceFromConnectURL(req, mcpID, h.oauthChecker.secretBindingAllowedLabel)
		if err != nil {
			if errHTTP := (*types.ErrHTTP)(nil); errors.As(err, &errHTTP) {
				redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
					Code:        ErrInvalidRequest,
					Description: errHTTP.Message,
					State:       oauthAppAuthRequest.Spec.State,
				})
			} else {
				redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
					Code:        ErrServerError,
					Description: fmt.Sprintf("failed to get MCP ID from connect URL: %v", err),
					State:       oauthAppAuthRequest.Spec.State,
				})
			}
			return nil
		}

		mcpID = serverOrInstanceID
		audience = "/" + audience
		if !strings.HasSuffix(oauthAppAuthRequest.Spec.Resource, audience) || oauthAppAuthRequest.Spec.MCPID != mcpID {
			// Ensure the audience is what the server expects.
			oauthAppAuthRequest.Spec.Resource = fmt.Sprintf("%s/mcp-connect%s", h.baseURL, audience)
			oauthAppAuthRequest.Spec.MCPID = mcpID
			if err = req.Update(&oauthAppAuthRequest); err != nil {
				redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
					Code:        ErrServerError,
					Description: fmt.Sprintf("failed to update OAuth app auth request: %v", err),
					State:       oauthAppAuthRequest.Spec.State,
				})
				return nil
			}
		}
	}

	oauthAppAuthRequest.Spec.UserID = req.UserID()
	oauthAppAuthRequest.Spec.AuthProviderUserID = auth.FirstExtraValue(req.User.GetExtra(), "auth_provider_user_id")
	oauthAppAuthRequest.Spec.AuthProviderNamespace = authProviderNamespace
	oauthAppAuthRequest.Spec.AuthProviderName = authProviderName
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
		})
		return nil
	}

	if err := h.prepareOAuthConsent(req, &oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	log.Infof("Prepared OAuth consent and redirecting user to consent screen: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
	http.Redirect(req.ResponseWriter, req.Request, fmt.Sprintf("/auth/oauth/consent/%s", oauthAppAuthRequest.Name), http.StatusFound)

	return nil
}

func (h *handler) prepareOAuthConsent(req api.Context, oauthAppAuthRequest *v1.OAuthAuthRequest) error {
	if oauthAppAuthRequest.Spec.ConsentPrepared && !oauthAppAuthRequest.Spec.ConsentMCPConfigRequired {
		return nil
	}

	// Check whether the MCP server needs authentication.
	mcpID, mcpServer, mcpServerConfig, missingConfig, err := handlers.ServerForActionWithConnectIDAllowMissingConfig(req, oauthAppAuthRequest.Spec.MCPID, h.oauthChecker.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	if len(missingConfig) > 0 {
		oauthAppAuthRequest.Spec.ConsentPrepared = true
		oauthAppAuthRequest.Spec.ConsentMCPConfigRequired = true
		oauthAppAuthRequest.Spec.ConsentMCPAuthRequired = false
		oauthAppAuthRequest.Spec.ConsentMCPAuthURL = ""
		oauthAppAuthRequest.Spec.UserHasSecondLevelOAuthed = false
		oauthAppAuthRequest.Spec.ConsentMCPServerName = mcpServerDisplayName(mcpServer)
		oauthAppAuthRequest.Spec.ConsentMCPServerURL = mcpServerConfig.URL
		return req.Update(oauthAppAuthRequest)
	}

	u, err := h.oauthChecker.CheckForMCPAuth(req, mcpServer, mcpServerConfig, req.User.GetUID(), mcpID, oauthAppAuthRequest.Name)
	if err != nil {
		return err
	}

	oauthAppAuthRequest.Spec.ConsentPrepared = true
	oauthAppAuthRequest.Spec.ConsentMCPConfigRequired = false
	oauthAppAuthRequest.Spec.ConsentMCPAuthRequired = u != ""
	oauthAppAuthRequest.Spec.ConsentMCPAuthURL = u
	oauthAppAuthRequest.Spec.UserHasSecondLevelOAuthed = u == "" && mcpServer.Status.UserHasAuthenticated
	oauthAppAuthRequest.Spec.ConsentMCPServerName = mcpServerDisplayName(mcpServer)
	oauthAppAuthRequest.Spec.ConsentMCPServerURL = mcpServerConfig.URL

	return req.Update(oauthAppAuthRequest)
}

func (h *handler) consent(req api.Context) error {
	oauthAppAuthRequest, ok, err := h.oauthConsentRequest(req, "OAuth consent")
	if err != nil || !ok {
		return err
	}

	if _, _, ok := authenticatedOAuthUser(req, oauthAppAuthRequest, "OAuth consent"); !ok {
		return nil
	}
	if !authenticatedOAuthConsentUser(req, oauthAppAuthRequest) {
		return nil
	}

	if !oauthAppAuthRequest.Spec.ConsentPrepared || oauthAppAuthRequest.Spec.ConsentMCPConfigRequired {
		if err := h.prepareOAuthConsent(req, &oauthAppAuthRequest); err != nil {
			redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
				Code:        ErrServerError,
				Description: err.Error(),
				State:       oauthAppAuthRequest.Spec.State,
			})
			return nil
		}
	}

	if !oauthAppAuthRequest.Spec.ConsentPrepared {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "OAuth consent has not been prepared",
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	clientID := oauthAppAuthRequest.Spec.ClientID
	if !isClientIDMetadataDocumentURL(clientID) {
		clientID = oauthAppAuthRequest.Namespace + ":" + clientID
	}

	oauthClient, err := h.resolveOAuthClient(req.Context(), req.Storage, clientID)
	if err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: fmt.Sprintf("failed to get OAuth client: %v", err),
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	continueURL, cancelURL := oauthConsentURLs(oauthAppAuthRequest)
	var (
		mcpServer         *types.MCPServer
		mcpServerInstance *types.MCPServerInstance
	)
	mcpServer, mcpServerInstance, err = handlers.ConfigurationTargetForConnectID(req, oauthAppAuthRequest.Spec.MCPID, h.baseURL, h.oauthChecker.secretBindingAllowedLabel)
	if err != nil {
		if oauthAppAuthRequest.Spec.ConsentMCPConfigRequired {
			redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
				Code:        ErrServerError,
				Description: err.Error(),
				State:       oauthAppAuthRequest.Spec.State,
			})
			return nil
		}
		log.Warnf("Failed to load optional MCP configuration target for OAuth consent: authRequest=%s mcpID=%s error=%v", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.MCPID, err)
	}

	return req.Write(oauthConsentPageData(oauthAppAuthRequest, oauthClient, continueURL, cancelURL, mcpServer, mcpServerInstance))
}

func (h *handler) approveConsent(req api.Context) error {
	oauthAppAuthRequest, ok, err := h.oauthConsentRequest(req, "OAuth consent approval")
	if err != nil || !ok {
		return err
	}

	oauthAppAuthRequest.Spec.ConsentApproved = true
	if oauthAppAuthRequest.Spec.ConsentMCPConfigRequired {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "MCP server configuration is required before consent can be approved",
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	if oauthAppAuthRequest.Spec.ConsentMCPAuthRequired {
		if oauthAppAuthRequest.Spec.ConsentMCPAuthURL == "" {
			redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
				Code:        ErrServerError,
				Description: "MCP OAuth URL is missing from consent state",
				State:       oauthAppAuthRequest.Spec.State,
			})
			return nil
		}

		log.Infof("OAuth consent approved; redirecting to second-level MCP authentication: authRequest=%s mcpID=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.MCPID)
		http.Redirect(req.ResponseWriter, req.Request, oauthAppAuthRequest.Spec.ConsentMCPAuthURL, http.StatusFound)
		return nil
	}

	log.Infof("OAuth consent approved; redirecting to completion screen: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
	redirectWithOAuthCompletion(req, oauthAppAuthRequest.Name)
	return nil
}

func (h *handler) cancelConsent(req api.Context) error {
	oauthAppAuthRequest, ok, err := h.oauthConsentRequest(req, "OAuth consent cancellation")
	if err != nil || !ok {
		return err
	}

	log.Infof("User denied OAuth consent: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
	redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
		Code:        ErrAccessDenied,
		Description: "user denied OAuth consent",
		State:       oauthAppAuthRequest.Spec.State,
	})
	return nil
}

func (h *handler) oauthConsentRequest(req api.Context, phase string) (v1.OAuthAuthRequest, bool, error) {
	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, req.PathValue("oauth_auth_request")); err != nil {
		return oauthAppAuthRequest, false, err
	}

	if _, _, ok := authenticatedOAuthUser(req, oauthAppAuthRequest, phase); !ok {
		return oauthAppAuthRequest, false, nil
	}
	if !authenticatedOAuthConsentUser(req, oauthAppAuthRequest) {
		return oauthAppAuthRequest, false, nil
	}

	if !oauthAppAuthRequest.Spec.ConsentPrepared {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "OAuth consent has not been prepared",
			State:       oauthAppAuthRequest.Spec.State,
		})
		return oauthAppAuthRequest, false, nil
	}

	return oauthAppAuthRequest, true, nil
}

func oauthConsentURLs(oauthAppAuthRequest v1.OAuthAuthRequest) (continueURL, cancelURL string) {
	base := "/oauth/consent/" + url.PathEscape(oauthAppAuthRequest.Name)
	return base + "/approve", base + "/cancel"
}

func (h *handler) oauthComplete(req api.Context) error {
	oauthAppAuthRequest, ok, err := h.oauthConsentRequest(req, "OAuth completion")
	if err != nil || !ok {
		return err
	}

	if !oauthAppAuthRequest.Spec.ConsentApproved {
		return types.NewErrBadRequest("OAuth consent has not been approved")
	}

	if oauthAppAuthRequest.Spec.ConsentMCPAuthRequired {
		if err := h.ensureMCPAuthComplete(req, oauthAppAuthRequest); err != nil {
			return err
		}
	}

	code := strings.ToLower(rand.Text() + rand.Text())
	oauthAppAuthRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		return types.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}

	log.Infof("OAuth completion issued authorization code for client redirect: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
	http.Redirect(req.ResponseWriter, req.Request, authorizeResponseURL(oauthAppAuthRequest, code, oauthAppAuthRequest.Spec.Scope), http.StatusFound)
	return nil
}

func (h *handler) ensureMCPAuthComplete(req api.Context, oauthAppAuthRequest v1.OAuthAuthRequest) error {
	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, oauthAppAuthRequest.Spec.MCPID, h.oauthChecker.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	u, err := h.oauthChecker.CheckForMCPAuth(req, mcpServer, mcpServerConfig, req.User.GetUID(), mcpID, oauthAppAuthRequest.Name)
	if err != nil {
		return err
	}
	if u != "" {
		return types.NewErrBadRequest("MCP OAuth is not complete")
	}

	return nil
}

// oauthCallback handles the second-level third-party OAuth for MCP servers.
func (h *handler) oauthCallback(req api.Context) error {
	if handled, err := h.maybeHandleDebuggerCallback(req); err != nil || handled {
		return err
	}

	oauthAuthRequestID, mcpServerID, err := h.oauthChecker.stateMgr.createToken(req.Context(), req.URL.Query().Get("state"), req.URL.Query().Get("code"), req.URL.Query().Get("error"), req.URL.Query().Get("error_description"))
	if err != nil {
		return types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if oauthAuthRequestID == "" {
		// If there is no OAuth request object, then MCP OAuth wasn't started by OAuth; likely the UI kicked it off.
		// Redirect to the OAuth completion page.
		log.Infof("Completed MCP OAuth callback without first-level OAuth auth request context")
		http.Redirect(req.ResponseWriter, req.Request, "/auth/oauth/complete", http.StatusFound)
		return nil
	}

	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, oauthAuthRequestID); err != nil {
		return err
	}

	authProviderName, authProviderNamespace := req.AuthProviderNameAndNamespace()

	if !req.UserIsAuthenticated() || req.User.GetName() == "bootstrap" || authProviderName == "bootstrap" || authProviderNamespace == "bootstrap" {
		// The user is either not authenticated or is authenticated as the bootstrap user.
		log.Infof("Denied MCP OAuth callback because user is not authenticated with a non-bootstrap identity: authRequest=%s", oauthAppAuthRequest.Name)
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrAccessDenied,
			Description: "user is not authenticated",
		})
		return nil
	}

	// Check if the MCP server is a component of a composite; only finalize if it's not
	var server v1.MCPServer
	if err := req.Get(&server, mcpServerID); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
		})
		return nil
	}

	if server.Spec.CompositeName != "" {
		// MCP server is a component of a composite.
		// Redirect to OAuth completion page; the checkCompositeAuth handler will redirect back
		// to the 1st level OAuth redirect URL when all pending 2nd level OAuth for the composite server's
		// component servers are completed.
		log.Infof("MCP OAuth callback completed for composite component server, awaiting composite finalization: authRequest=%s mcpServer=%s composite=%s", oauthAppAuthRequest.Name, server.Name, server.Spec.CompositeName)
		http.Redirect(req.ResponseWriter, req.Request, "/auth/oauth/complete", http.StatusFound)
		return nil
	}

	log.Infof("Completed MCP OAuth callback and redirecting to completion screen: authRequest=%s mcpServer=%s", oauthAppAuthRequest.Name, mcpServerID)
	redirectWithOAuthCompletion(req, oauthAppAuthRequest.Name)

	return nil
}

func authenticatedOAuthUser(req api.Context, oauthAppAuthRequest v1.OAuthAuthRequest, phase string) (authProviderName, authProviderNamespace string, ok bool) {
	authProviderName, authProviderNamespace = req.AuthProviderNameAndNamespace()
	if req.UserIsAuthenticated() &&
		req.User.GetName() != "bootstrap" &&
		authProviderName != "bootstrap" &&
		authProviderNamespace != "bootstrap" {
		return authProviderName, authProviderNamespace, true
	}

	log.Infof("Denied %s because user is not authenticated with a non-bootstrap identity: authRequest=%s", phase, oauthAppAuthRequest.Name)
	redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
		Code:        ErrAccessDenied,
		Description: "user is not authenticated",
		State:       oauthAppAuthRequest.Spec.State,
	})
	return "", "", false
}

func authenticatedOAuthConsentUser(req api.Context, oauthAppAuthRequest v1.OAuthAuthRequest) bool {
	if oauthAppAuthRequest.Spec.UserID == 0 || oauthAppAuthRequest.Spec.UserID == req.UserID() {
		return true
	}

	log.Infof("Denied OAuth consent because authenticated user does not match auth request user: authRequest=%s", oauthAppAuthRequest.Name)
	redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
		Code:        ErrAccessDenied,
		Description: "user is not authorized for this OAuth request",
		State:       oauthAppAuthRequest.Spec.State,
	})
	return false
}

type oauthConsentData struct {
	AuthRequestID             string                   `json:"authRequestID"`
	ContinueURL               string                   `json:"continueURL"`
	CancelURL                 string                   `json:"cancelURL"`
	ClientName                string                   `json:"clientName"`
	ClientCredentialSource    string                   `json:"clientCredentialSource"`
	ClientURI                 string                   `json:"clientURI,omitempty"`
	RedirectURI               string                   `json:"redirectURI"`
	Scope                     string                   `json:"scope,omitempty"`
	PolicyURI                 string                   `json:"policyURI,omitempty"`
	TOSURI                    string                   `json:"tosURI,omitempty"`
	MCPConfigRequired         bool                     `json:"mcpConfigRequired"`
	MCPServer                 *types.MCPServer         `json:"mcpServer,omitempty"`
	MCPServerInstance         *types.MCPServerInstance `json:"mcpServerInstance,omitempty"`
	MCPAuthRequired           bool                     `json:"mcpAuthRequired"`
	UserHasSecondLevelOAuthed bool                     `json:"userHasSecondLevelOAuthed"`
	MCPServerName             string                   `json:"mcpServerName,omitempty"`
	MCPServerURL              string                   `json:"mcpServerURL,omitempty"`
	ThirdPartyAuthURL         string                   `json:"thirdPartyAuthURL,omitempty"`
}

func oauthConsentPageData(oauthAppAuthRequest v1.OAuthAuthRequest, oauthClient v1.OAuthClient, continueURL, cancelURL string, mcpServer *types.MCPServer, mcpServerInstance *types.MCPServerInstance) oauthConsentData {
	clientName := oauthClient.Spec.Manifest.ClientName
	if clientName == "" {
		clientName = fmt.Sprintf("%s:%s", oauthClient.Namespace, oauthClient.Name)
	}

	return oauthConsentData{
		AuthRequestID:             oauthAppAuthRequest.Name,
		ContinueURL:               continueURL,
		CancelURL:                 cancelURL,
		ClientName:                clientName,
		ClientCredentialSource:    oauthConsentClientCredentialSource(oauthClient),
		ClientURI:                 oauthClient.Spec.Manifest.ClientURI,
		RedirectURI:               oauthAppAuthRequest.Spec.RedirectURI,
		Scope:                     oauthAppAuthRequest.Spec.Scope,
		PolicyURI:                 oauthClient.Spec.Manifest.PolicyURI,
		TOSURI:                    oauthClient.Spec.Manifest.TOSURI,
		MCPConfigRequired:         oauthAppAuthRequest.Spec.ConsentMCPConfigRequired,
		MCPServer:                 mcpServer,
		MCPServerInstance:         mcpServerInstance,
		MCPAuthRequired:           oauthAppAuthRequest.Spec.ConsentMCPAuthRequired,
		UserHasSecondLevelOAuthed: oauthAppAuthRequest.Spec.UserHasSecondLevelOAuthed,
		MCPServerName:             oauthAppAuthRequest.Spec.ConsentMCPServerName,
		MCPServerURL:              originOnly(oauthAppAuthRequest.Spec.ConsentMCPServerURL),
		ThirdPartyAuthURL:         originOnly(oauthAppAuthRequest.Spec.ConsentMCPAuthURL),
	}
}

func oauthConsentClientCredentialSource(oauthClient v1.OAuthClient) string {
	if isClientIDMetadataDocumentURL(oauthClient.Name) {
		return "client_id_metadata_document"
	}
	if oauthClient.Spec.Static {
		return "static_client_credentials"
	}
	return "dynamic_client"
}

func mcpServerDisplayName(mcpServer v1.MCPServer) string {
	if mcpServer.Spec.Alias != "" {
		return mcpServer.Spec.Alias
	}
	if mcpServer.Spec.Manifest.Name != "" {
		return mcpServer.Spec.Manifest.Name
	}
	return mcpServer.Name
}

func originOnly(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}

	return u.Scheme + "://" + u.Host
}

func (h *handler) maybeHandleDebuggerCallback(req api.Context) (bool, error) {
	state := req.URL.Query().Get("state")
	if state == "" {
		return false, nil
	}

	pendingState, err := h.oauthChecker.stateMgr.gatewayClient.GetMCPOAuthPendingState(req.Context(), state)
	if errors.Is(err, gorm.ErrRecordNotFound) || pendingState != nil && pendingState.OAuthAuthRequestID != handlers.OAuthDebuggerPendingStateMarker {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to get pending state: %w", err)
	}

	code := req.URL.Query().Get("code")
	errorCode := req.URL.Query().Get("error")
	errorDescription := req.URL.Query().Get("error_description")

	q := url.Values{}
	q.Set("state", state)
	if errorCode != "" {
		q.Set("error", errorCode)
		if errorDescription != "" {
			q.Set("error_description", errorDescription)
		}
	} else {
		q.Set("code", code)
	}

	dest := url.URL{Path: "/oauth-debugger/callback", RawQuery: q.Encode()}
	http.Redirect(req.ResponseWriter, req.Request, dest.String(), http.StatusFound)
	return true, nil
}

func redirectWithAuthorizeError(req api.Context, redirectURI string, err Error) {
	http.Redirect(req.ResponseWriter, req.Request, authorizeErrorURL(redirectURI, err), http.StatusFound)
}

func redirectWithOAuthCompletion(req api.Context, oauthAuthRequestID string) {
	http.Redirect(req.ResponseWriter, req.Request, oauthCompletionURL(oauthAuthRequestID), http.StatusFound)
}

func oauthCompletionURL(oauthAuthRequestID string) string {
	if oauthAuthRequestID == "" {
		return "/auth/oauth/complete"
	}
	return "/auth/oauth/complete/" + url.PathEscape(oauthAuthRequestID)
}

func authorizeErrorURL(redirectURI string, err Error) string {
	return redirectURI + "?" + err.toQuery().Encode()
}

func authorizeResponseURL(oauthAuthRequest v1.OAuthAuthRequest, code, scope string) string {
	q := url.Values{
		"code":  {code},
		"state": {oauthAuthRequest.Spec.State},
	}
	q.Set("scope", scope)

	return oauthAuthRequest.Spec.RedirectURI + "?" + q.Encode()
}
