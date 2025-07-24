package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &oauthClient); err != nil {
		return err
	}

	if !slices.Contains(oauthClient.Spec.Manifest.RedirectURIs, redirectURI) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "redirect_uri is invalid for this client",
			State:       state,
		})
	}

	if !slices.Contains(oauthClient.Spec.Manifest.ResponseTypes, responseType) {
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

	if scope := req.FormValue("scope"); scope != "" {
		var (
			unsupported []string
			scopes      = make(map[string]struct{})
		)
		for _, s := range strings.Split(scope, " ") {
			scopes[s] = struct{}{}
		}

		for _, s := range strings.Split(oauthClient.Spec.Manifest.Scope, " ") {
			if _, ok := scopes[s]; !ok {
				unsupported = append(unsupported, s)
			}
		}

		if len(unsupported) > 0 {
			redirectWithAuthorizeError(req, redirectURI, Error{
				Code:        ErrInvalidScope,
				Description: fmt.Sprintf("scopes %s are not allowed for this client", strings.Join(unsupported, ", ")),
				State:       state,
			})
			return nil
		}
	}

	oauthAppAuthRequest := v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.OAuthAppPrefix,
			Namespace:    oauthClient.Namespace,
		},
		Spec: v1.OAuthAuthRequestSpec{
			State:               state,
			ClientID:            oauthClient.Name,
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			GrantType:           "authorization_code",
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

	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		mcpID = "/" + mcpID
	}

	// We need to authenticate the user.
	http.Redirect(req.ResponseWriter, req.Request, fmt.Sprintf("/?rd=/oauth/callback/%s%s", oauthAppAuthRequest.Name, mcpID), http.StatusFound)
	return nil
}

// callback handles the OAuth callback for the first-level Obot-based OAuth.
func (h *handler) callback(req api.Context) error {
	var oauthAppAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAppAuthRequest, req.PathValue("oauth_auth_request")); err != nil {
		return err
	}

	authProviderName, authProviderNamespace := req.AuthProviderNameAndNamespace()

	if !req.UserIsAuthenticated() || req.User.GetName() == "bootstrap" || authProviderName == "bootstrap" || authProviderNamespace == "bootstrap" {
		// The user is either not authenticated or is authenticated as the bootstrap user.
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrAccessDenied,
			Description: "user is not authenticated",
		})
		return nil
	}

	code := strings.ToLower(rand.Text() + rand.Text())
	oauthAppAuthRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	oauthAppAuthRequest.Spec.UserID = req.UserID()
	oauthAppAuthRequest.Spec.AuthProviderNamespace = authProviderNamespace
	oauthAppAuthRequest.Spec.AuthProviderName = authProviderName
	if err := req.Update(&oauthAppAuthRequest); err != nil {
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
		})
		return nil
	}

	// Check whether the MCP server needs authentication.
	var (
		mcpID           = req.PathValue("mcp_id")
		mcpServer       v1.MCPServer
		mcpServerConfig mcp.ServerConfig
		err             error
	)

	if mcpID != "" {
		if strings.HasPrefix(mcpID, system.MCPServerInstancePrefix) {
			mcpServer, mcpServerConfig, err = handlers.ServerFromMCPServerInstance(req, h.tokenService, mcpID)
		} else {
			mcpServer, mcpServerConfig, err = handlers.ServerForActionWithID(req, h.tokenService, mcpID)
		}
		if err != nil {
			return err
		}

		// For now, we only need to check for OAuth if the MCP server is remote.
		// This may change in the future as the protocol matures, but, for now, this is an optimization for loading times for the redirects.
		if mcpServerConfig.Command == "" {
			u, err := h.oauthChecker.CheckForMCPAuth(req.Context(), mcpServer, mcpServerConfig, mcpID, oauthAppAuthRequest.Name)
			if err != nil {
				redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
					Code:        ErrServerError,
					Description: err.Error(),
				})
				return nil
			}

			if u != "" {
				http.Redirect(req.ResponseWriter, req.Request, u, http.StatusFound)
				return nil
			}
		}
	}

	redirectWithAuthorizeResponse(req, oauthAppAuthRequest, code)

	return nil
}

// oauthCallback handles the second-level third-party OAuth for MCP servers.
func (h *handler) oauthCallback(req api.Context) error {
	oauthAuthRequestID, err := h.oauthChecker.stateCache.createToken(req.Context(), req.URL.Query().Get("state"), req.URL.Query().Get("code"), req.URL.Query().Get("error"), req.URL.Query().Get("error_description"))
	if err != nil {
		return types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if oauthAuthRequestID == "" {
		// If there is no OAuth request object, then MCP OAuth wasn't started by OAuth; likely the UI kicked it off.
		// Redirect to the login complete page.
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
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrAccessDenied,
			Description: "user is not authenticated",
		})
		return nil
	}

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

	redirectWithAuthorizeResponse(req, oauthAppAuthRequest, code)

	return nil
}

func redirectWithAuthorizeError(req api.Context, redirectURI string, err Error) {
	http.Redirect(req.ResponseWriter, req.Request, redirectURI+"?"+err.toQuery().Encode(), http.StatusFound)
}

func redirectWithAuthorizeResponse(req api.Context, oauthAuthRequest v1.OAuthAuthRequest, code string) {
	q := url.Values{
		"code":  {code},
		"state": {oauthAuthRequest.Spec.State},
	}

	http.Redirect(req.ResponseWriter, req.Request, oauthAuthRequest.Spec.RedirectURI+"?"+q.Encode(), http.StatusFound)
}
