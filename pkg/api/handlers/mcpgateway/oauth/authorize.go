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
			unsupported []string
			scopes      = make(map[string]struct{})
		)
		for s := range strings.SplitSeq(oauthClient.Spec.Manifest.Scope, " ") {
			scopes[s] = struct{}{}
		}

		for s := range strings.SplitSeq(scope, " ") {
			if _, ok := scopes[s]; s != "" && !ok {
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

	if mcpID != "" {
		mcpID = "/" + mcpID
	}
	oauthAppAuthRequest := v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.OAuthAppPrefix,
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
			MCPID:               strings.TrimPrefix(mcpID, "/"),
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

	if !req.UserIsAuthenticated() ||
		req.User.GetName() == "bootstrap" ||
		authProviderName == "bootstrap" ||
		authProviderNamespace == "bootstrap" {
		// The user is either not authenticated or is authenticated as the bootstrap user.
		redirectWithAuthorizeError(req, oauthAppAuthRequest.Spec.RedirectURI, Error{
			Code:        ErrAccessDenied,
			Description: "user is not authenticated",
		})
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

	code := strings.ToLower(rand.Text() + rand.Text())
	oauthAppAuthRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
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

	if mcpID == "" {
		mcpID = req.PathValue("mcp_id")
	}
	if mcpID != "" {
		// Check whether the MCP server needs authentication.
		mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, mcpID)
		if err != nil {
			return err
		}

		u, err := h.oauthChecker.CheckForMCPAuth(req, mcpServer, mcpServerConfig, req.User.GetUID(), mcpID, oauthAppAuthRequest.Name)
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

	redirectWithAuthorizeResponse(req, oauthAppAuthRequest, code, oauthAppAuthRequest.Spec.Scope)

	return nil
}

// oauthCallback handles the second-level third-party OAuth for MCP servers.
func (h *handler) oauthCallback(req api.Context) error {
	oauthAuthRequestID, mcpServerID, err := h.oauthChecker.stateCache.createToken(req.Context(), req.URL.Query().Get("state"), req.URL.Query().Get("code"), req.URL.Query().Get("error"), req.URL.Query().Get("error_description"))
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

	redirectWithAuthorizeResponse(req, oauthAppAuthRequest, code, oauthAppAuthRequest.Spec.Scope)

	return nil
}

func redirectWithAuthorizeError(req api.Context, redirectURI string, err Error) {
	http.Redirect(req.ResponseWriter, req.Request, redirectURI+"?"+err.toQuery().Encode(), http.StatusFound)
}

func redirectWithAuthorizeResponse(req api.Context, oauthAuthRequest v1.OAuthAuthRequest, code, scope string) {
	q := url.Values{
		"code":  {code},
		"state": {oauthAuthRequest.Spec.State},
	}
	if scope != "" {
		q.Set("scope", scope)
	}

	http.Redirect(req.ResponseWriter, req.Request, oauthAuthRequest.Spec.RedirectURI+"?"+q.Encode(), http.StatusFound)
}
