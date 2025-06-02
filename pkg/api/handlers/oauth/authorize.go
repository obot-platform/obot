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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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

	scope := req.FormValue("scope")
	state := req.FormValue("state")
	codeChallenge := req.FormValue("code_challenge")
	codeChallengeMethod := req.FormValue("code_challenge_method")
	// TODO: Check that the code_challenge_method is supported by the server

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

	var oauthClient v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &oauthClient); err != nil {
		return err
	}

	if !slices.Contains(oauthClient.Spec.Manifest.RedirectURIs, redirectURI) || oauthClient.Spec.Manifest.RedirectURI != "" && oauthClient.Spec.Manifest.RedirectURI != redirectURI {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "redirect_uri is invalid for this client",
			State:       state,
		})
	}

	// TODO: According to the OAuth Spec, we should also check that the response_type is allowed for the server.
	if !slices.Contains(oauthClient.Spec.Manifest.ResponseTypes, responseType) {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrUnsupportedResponseType,
			Description: "response_type is not allowed for this client",
			State:       state,
		})
		return nil
	}

	if scope != "" && !slices.Contains(strings.Split(oauthClient.Spec.Manifest.Scope, " "), scope) {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrInvalidScope,
			Description: "scope is not allowed for this client",
			State:       state,
		})
		return nil
	}

	// TODO: This is where we need to OAuth with the auth provider.

	code := rand.Text() + rand.Text()

	if err := req.Create(&v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
			Namespace: oauthClient.Namespace,
		},
		Spec: v1.OAuthAuthRequestSpec{
			ClientID:            oauthClient.Name,
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			GrantType:           "authorization_code",
			Scope:               scope,
		},
	}); err != nil {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       state,
		})
		return nil
	}

	redirectWithAuthorizeResponse(req, redirectURI, code, state)
	return nil
}

func redirectWithAuthorizeError(req api.Context, redirectURI string, err Error) {
	http.Redirect(req.ResponseWriter, req.Request, redirectURI+"?"+err.toQuery().Encode(), http.StatusFound)
}

func redirectWithAuthorizeResponse(req api.Context, redirectURI, code, state string) {
	q := url.Values{
		"code":  {code},
		"state": {state},
	}

	http.Redirect(req.ResponseWriter, req.Request, redirectURI+"?"+q.Encode(), http.StatusFound)
}
