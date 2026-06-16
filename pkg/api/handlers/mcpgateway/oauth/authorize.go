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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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

const (
	consentActionApprove = "approve"
	consentActionDeny    = "deny"
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

	clientNamespace, clientName, ok := strings.Cut(clientID, ":")
	if !ok {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client_id is invalid",
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

	var oauthClient v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &oauthClient); apierrors.IsNotFound(err) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: fmt.Sprintf("client_id does not exist: %s", clientID),
			State:       state,
		})
	} else if err != nil {
		return Error{
			Code:        ErrServerError,
			Description: fmt.Sprintf("failed to get OAuth client: %v", err),
			State:       state,
		}
	}

	if !slices.Contains(oauthClient.Spec.Manifest.RedirectURIs, redirectURI) {
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
		serverOrInstanceID, audience, err := handlers.MCPIDAndAudienceFromConnectURL(req, mcpID)
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
	if oauthAppAuthRequest.Spec.ConsentPrepared {
		if oauthAppAuthRequest.Spec.ConsentCSRFToken == "" {
			oauthAppAuthRequest.Spec.ConsentCSRFToken = strings.ToLower(rand.Text())
			return req.Update(oauthAppAuthRequest)
		}
		return nil
	}

	// Check whether the MCP server needs authentication.
	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, oauthAppAuthRequest.Spec.MCPID)
	if err != nil {
		return err
	}

	u, err := h.oauthChecker.CheckForMCPAuth(req, mcpServer, mcpServerConfig, req.User.GetUID(), mcpID, oauthAppAuthRequest.Name)
	if err != nil {
		return err
	}

	oauthAppAuthRequest.Spec.ConsentPrepared = true
	oauthAppAuthRequest.Spec.ConsentMCPAuthRequired = u != ""
	oauthAppAuthRequest.Spec.ConsentMCPAuthURL = u
	oauthAppAuthRequest.Spec.UserHasSecondLevelOAuthed = u == "" && mcpServer.Status.UserHasAuthenticated
	oauthAppAuthRequest.Spec.ConsentMCPServerName = mcpServerDisplayName(mcpServer)
	oauthAppAuthRequest.Spec.ConsentMCPServerURL = mcpServerConfig.URL
	oauthAppAuthRequest.Spec.ConsentCSRFToken = strings.ToLower(rand.Text())

	return req.Update(oauthAppAuthRequest)
}

func (h *handler) consent(req api.Context) error {
	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, req.PathValue("oauth_auth_request")); err != nil {
		return err
	}

	if _, _, ok := authenticatedOAuthUser(req, oauthAppAuthRequest, "OAuth consent"); !ok {
		return nil
	}
	if !authenticatedOAuthConsentUser(req, oauthAppAuthRequest) {
		return nil
	}

	if !oauthAppAuthRequest.Spec.ConsentPrepared {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "OAuth consent has not been prepared",
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	oauthClient, err := h.oauthClientForAuthRequest(req, oauthAppAuthRequest)
	if err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: fmt.Sprintf("failed to get OAuth client: %v", err),
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	return req.Write(oauthConsentPageData(oauthAppAuthRequest, oauthClient))
}

func (h *handler) approveConsent(req api.Context) error {
	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, req.PathValue("oauth_auth_request")); err != nil {
		return err
	}

	if _, _, ok := authenticatedOAuthUser(req, oauthAppAuthRequest, "OAuth consent approval"); !ok {
		return nil
	}
	if !authenticatedOAuthConsentUser(req, oauthAppAuthRequest) {
		return nil
	}

	if !oauthAppAuthRequest.Spec.ConsentPrepared {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrInvalidRequest,
			Description: "OAuth consent has not been prepared",
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	if err := req.ParseForm(); err != nil {
		return err
	}
	if req.FormValue("csrf_token") == "" || req.FormValue("csrf_token") != oauthAppAuthRequest.Spec.ConsentCSRFToken {
		return types.NewErrBadRequest("invalid OAuth consent token")
	}

	switch req.FormValue("action") {
	case consentActionDeny:
		log.Infof("User denied OAuth consent: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
		return req.Write(map[string]string{"redirectURL": authorizeErrorURL(oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrAccessDenied,
			Description: "user denied OAuth consent",
			State:       oauthAppAuthRequest.Spec.State,
		})})
	case consentActionApprove:
	default:
		return types.NewErrBadRequest("invalid OAuth consent action")
	}

	if oauthAppAuthRequest.Spec.ConsentMCPAuthRequired {
		u := oauthAppAuthRequest.Spec.ConsentMCPAuthURL
		if u == "" {
			redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
				Code:        ErrServerError,
				Description: "MCP OAuth URL is missing from consent state",
				State:       oauthAppAuthRequest.Spec.State,
			})
			return nil
		}

		log.Infof("OAuth consent approved; redirecting to second-level MCP authentication: authRequest=%s mcpID=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.MCPID)
		return req.Write(map[string]string{"redirectURL": u})
	}

	code := strings.ToLower(rand.Text() + rand.Text())
	oauthAppAuthRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       oauthAppAuthRequest.Spec.State,
		})
		return nil
	}

	log.Infof("OAuth consent approved; issuing authorization code and redirecting to client: authRequest=%s client=%s", oauthAppAuthRequest.Name, oauthAppAuthRequest.Spec.ClientID)
	return req.Write(map[string]string{"redirectURL": authorizeResponseURL(oauthAppAuthRequest, code, oauthAppAuthRequest.Spec.Scope)})
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
		// Redirect to the login complete page.
		log.Infof("Completed MCP OAuth callback without first-level OAuth auth request context")
		http.Redirect(req.ResponseWriter, req.Request, "/login_complete", http.StatusFound)
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
		// Redirect to login complete page; the checkCompositeAuth handler will redirect back
		// to the 1st level OAuth redirect URL when all pending 2nd level OAuth for the composite server's
		// component servers are completed.
		log.Infof("MCP OAuth callback completed for composite component server, awaiting composite finalization: authRequest=%s mcpServer=%s composite=%s", oauthAppAuthRequest.Name, server.Name, server.Spec.CompositeName)
		http.Redirect(req.ResponseWriter, req.Request, "/login_complete", http.StatusFound)
		return nil
	}

	// Not a component of a composite MCP server, redirect to complete 1st level OAuth
	// Update the authorization code since we only saved the hash of it the first time.
	code := strings.ToLower(rand.Text() + rand.Text())
	oauthAppAuthRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
		})
		return nil
	}

	log.Infof("Completed MCP OAuth callback and issuing first-level OAuth authorization code: authRequest=%s mcpServer=%s", oauthAppAuthRequest.Name, mcpServerID)
	redirectWithAuthorizeResponse(req, oauthAppAuthRequest, code, oauthAppAuthRequest.Spec.Scope)

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

func (h *handler) oauthClientForAuthRequest(req api.Context, oauthAppAuthRequest v1.OAuthAuthRequest) (v1.OAuthClient, error) {
	var oauthClient v1.OAuthClient
	return oauthClient, req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: oauthAppAuthRequest.Namespace,
		Name:      oauthAppAuthRequest.Spec.ClientID,
	}, &oauthClient)
}

type oauthConsentData struct {
	AuthRequestID             string `json:"authRequestID"`
	CSRFToken                 string `json:"csrfToken"`
	ClientName                string `json:"clientName"`
	ClientURI                 string `json:"clientURI,omitempty"`
	RedirectURI               string `json:"redirectURI"`
	Scope                     string `json:"scope,omitempty"`
	PolicyURI                 string `json:"policyURI,omitempty"`
	TOSURI                    string `json:"tosURI,omitempty"`
	MCPAuthRequired           bool   `json:"mcpAuthRequired"`
	UserHasSecondLevelOAuthed bool   `json:"userHasSecondLevelOAuthed"`
	MCPServerName             string `json:"mcpServerName,omitempty"`
	MCPServerURL              string `json:"mcpServerURL,omitempty"`
	ThirdPartyAuthURL         string `json:"thirdPartyAuthURL,omitempty"`
}

func oauthConsentPageData(oauthAppAuthRequest v1.OAuthAuthRequest, oauthClient v1.OAuthClient) oauthConsentData {
	clientName := oauthClient.Spec.Manifest.ClientName
	if clientName == "" {
		clientName = fmt.Sprintf("%s:%s", oauthClient.Namespace, oauthClient.Name)
	}

	return oauthConsentData{
		AuthRequestID:             oauthAppAuthRequest.Name,
		CSRFToken:                 oauthAppAuthRequest.Spec.ConsentCSRFToken,
		ClientName:                clientName,
		ClientURI:                 oauthClient.Spec.Manifest.ClientURI,
		RedirectURI:               oauthAppAuthRequest.Spec.RedirectURI,
		Scope:                     oauthAppAuthRequest.Spec.Scope,
		PolicyURI:                 oauthClient.Spec.Manifest.PolicyURI,
		TOSURI:                    oauthClient.Spec.Manifest.TOSURI,
		MCPAuthRequired:           oauthAppAuthRequest.Spec.ConsentMCPAuthRequired,
		UserHasSecondLevelOAuthed: oauthAppAuthRequest.Spec.UserHasSecondLevelOAuthed,
		MCPServerName:             oauthAppAuthRequest.Spec.ConsentMCPServerName,
		MCPServerURL:              originOnly(oauthAppAuthRequest.Spec.ConsentMCPServerURL),
		ThirdPartyAuthURL:         originOnly(oauthAppAuthRequest.Spec.ConsentMCPAuthURL),
	}
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

func redirectWithAuthorizeResponse(req api.Context, oauthAuthRequest v1.OAuthAuthRequest, code, scope string) {
	http.Redirect(req.ResponseWriter, req.Request, authorizeResponseURL(oauthAuthRequest, code, scope), http.StatusFound)
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
