package oauth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
	var apps v1.OAuthAppList
	if err := req.List(&apps); err != nil {
		return err
	}

	if len(apps.Items) == 0 {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "no oauth apps found",
		})
	}
	if len(apps.Items) != 1 {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "not able to determine oauth app",
		})
	}

	oauthApp := apps.Items[0]

	if err := req.ParseForm(); err != nil {
		return err
	}

	resource := strings.TrimPrefix(req.FormValue("resource"), "http://")
	if resource == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "resource is required",
		})
	}

	state := req.FormValue("state")
	codeChallenge := req.FormValue("code_challenge")
	codeChallengeMethod := req.FormValue("code_challenge_method")
	if codeChallenge != "" && (codeChallengeMethod == "" || !slices.Contains([]string{"S256", "plain"}, codeChallengeMethod)) {
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

	scope := req.FormValue("scope")
	if scope == "" {
		scope = oauthClient.Spec.Manifest.Scope
	} else {
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

	if scope == "" {
		scope = oauthApp.Spec.Manifest.DefaultScope
	}

	oauthAppAuthRequest := v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.OAuthAppPrefix,
			Namespace:    oauthClient.Namespace,
		},
		Spec: v1.OAuthAuthRequestSpec{
			ProviderName:        oauthApp.Spec.Manifest.Alias,
			Resource:            resource,
			ClientID:            oauthClient.Name,
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			GrantType:           "authorization_code",
			Scope:               scope,
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

	clientState := strings.ToLower(rand.Text())

	if err := req.Create(&v1.OAuthAppAuth{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(clientState))),
			Namespace: clientNamespace,
		},
		Spec: v1.OAuthAppAuthSpec{
			AuthRequestName:   oauthAppAuthRequest.Name,
			OAuthAppNamespace: oauthApp.Namespace,
			OAuthAppName:      oauthApp.Name,
		},
	}); err != nil {
		redirectWithAuthorizeError(req, redirectURI, Error{
			Code:        ErrServerError,
			Description: err.Error(),
			State:       state,
		})
		return nil
	}

	// Construct URL to redirect the user to.
	u, err := url.Parse(oauthApp.Spec.Manifest.AuthURL)
	if err != nil { // This should never happen unless someone updates the database directly with an invalid URL.
		return fmt.Errorf("failed to parse auth URL %q: %w", oauthApp.Spec.Manifest.AuthURL, err)
	}

	q := u.Query()

	q.Set("response_type", "code")
	q.Set("client_id", oauthApp.Spec.Manifest.ClientID)
	q.Set("redirect_uri", fmt.Sprintf("%s/oauth/callback", h.baseURL))
	q.Set("state", clientState)

	// HubSpot supports setting optional scopes in this query param so that we can support an app that is able to have broad permissions,
	// while at the same time only granting specific stuff.
	if oauthApp.Spec.Manifest.Type == types.OAuthAppTypeHubSpot {
		q.Set("optional_scope", oauthApp.Spec.Manifest.OptionalScope)
	}

	// Atlassian requires the audience and prompt parameters to be set.
	// See https://developer.atlassian.com/cloud/jira/platform/oauth-2-3lo-apps/#1--direct-the-user-to-the-authorization-url-to-get-an-authorization-code
	// for details.
	if oauthApp.Spec.Manifest.Type == types.OAuthAppTypeAtlassian {
		q.Set("audience", "api.atlassian.com")
		q.Set("prompt", "consent")
	}

	// For Google: access_type=offline instructs Google to return a refresh token and an access token on the initial authorization.
	// This can be used to refresh the access token when a user is not present at the browser
	// prompt=consent instructs Google to show the consent screen every time the authorization flow happens so that we get a new refresh token.
	if oauthApp.Spec.Manifest.Type == types.OAuthAppTypeGoogle {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	}

	// Slack is annoying and makes us call this query parameter user_scope instead of scope.
	// user_scope is used for delegated user permissions (which is what we want), while just scope is used for bot permissions.
	if oauthApp.Spec.Manifest.Type == types.OAuthAppTypeSlack {
		if scope != "" {
			q.Set("scope", scope)
		}
		userScope := req.URL.Query().Get("user_scope")
		if userScope != "" {
			q.Set("user_scope", userScope)
		}
	} else {
		q.Set("scope", scope)
	}

	u.RawQuery = q.Encode()

	// Return a 302 to redirect.
	http.Redirect(req.ResponseWriter, req.Request, u.String(), http.StatusFound)
	return nil
}

func (h *handler) callback(req api.Context) error {
	// Check for the query parameters.
	var (
		code         = req.URL.Query().Get("code")
		state        = req.URL.Query().Get("state")
		e            = req.URL.Query().Get("error")
		eDescription = req.URL.Query().Get("error_description")
	)
	if e != "" {
		return apierrors.NewBadRequest(fmt.Sprintf("error: %s (%s)", e, eDescription))
	}

	if code == "" {
		return apierrors.NewBadRequest("missing code query parameter")
	} else if state == "" {
		return apierrors.NewBadRequest("missing state query parameter")
	}

	var oauthAppAuth v1.OAuthAppAuth
	if err := req.Get(&oauthAppAuth, fmt.Sprintf("%x", sha256.Sum256([]byte(state)))); err != nil {
		return err
	}

	var app v1.OAuthApp
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthAppAuth.Spec.OAuthAppNamespace, Name: oauthAppAuth.Spec.OAuthAppName}, &app); err != nil {
		return err
	}

	var oauthAuthRequest v1.OAuthAuthRequest
	if err := req.Get(&oauthAuthRequest, oauthAppAuth.Spec.AuthRequestName); err != nil {
		return err
	}

	var clientSecret string

	// Reveal the credential to get the client secret.
	cred, err := h.gptClient.RevealCredential(req.Context(), []string{oauthAppAuth.Spec.OAuthAppName}, app.Spec.Manifest.Alias)
	if err != nil {
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	clientSecret = cred.Env["CLIENT_SECRET"]

	// Build and make the request to get the tokens.
	data := url.Values{}
	data.Set("client_id", app.Spec.Manifest.ClientID)
	data.Set("client_secret", clientSecret) // Including the client secret in the body is not strictly required in the OAuth2 RFC, but some providers require it anyway.
	data.Set("code", code)
	data.Set("redirect_uri", fmt.Sprintf("%s/oauth/callback", h.baseURL))
	data.Set("grant_type", "authorization_code")

	if app.Spec.Manifest.Type == types.OAuthAppTypeHubSpot {
		data.Set("optional_scope", app.Spec.Manifest.OptionalScope)
	}

	r, err := http.NewRequest("POST", app.Spec.Manifest.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if app.Spec.Manifest.Type != types.OAuthAppTypeGoogle &&
		app.Spec.Manifest.Type != types.OAuthAppTypePagerDuty {
		req.SetBasicAuth(url.QueryEscape(app.Spec.Manifest.ClientID), url.QueryEscape(clientSecret))
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return fmt.Errorf("failed to make token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBuf := new(bytes.Buffer)
		_, _ = bodyBuf.ReadFrom(resp.Body)
		return fmt.Errorf("failed to get tokens: %d %s", resp.StatusCode, bodyBuf.String())
	}

	// Get the response and save it. Once again, Slack and GitHub are annoying and do their own thing.
	var status v1.OAuthAuthRequestStatus
	switch app.Spec.Manifest.Type {
	case types.OAuthAppTypeSlack:
		slackTokenResp := new(slackOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(slackTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}

		status = v1.OAuthAuthRequestStatus{
			Ok:                     slackTokenResp.Ok,
			Error:                  slackTokenResp.Error,
			ProviderTokenCreatedAt: metav1.Now(),
			Data: map[string]string{
				"slack_app_id":    slackTokenResp.AppID,
				"slack_team_id":   slackTokenResp.Team.ID,
				"slack_team_name": slackTokenResp.Team.Name,
			},
		}

		if slackTokenResp.AuthedUser.AccessToken != "" {
			status.ProviderAccessToken = slackTokenResp.AuthedUser.AccessToken
			status.Scope = slackTokenResp.AuthedUser.Scope
		} else if slackTokenResp.AccessToken != "" {
			status.ProviderAccessToken = slackTokenResp.AccessToken
			status.Scope = slackTokenResp.Scope
		}
	case types.OAuthAppTypeGitHub:
		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Parse the URL-encoded body
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}

		// Map the parsed values to the struct
		status = v1.OAuthAuthRequestStatus{
			Scope:                  values.Get("scope"),
			ProviderAccessToken:    values.Get("access_token"),
			Ok:                     true, // Assuming true if no error is present
			ProviderTokenCreatedAt: metav1.Now(),
		}
	case types.OAuthAppTypeGoogle:
		googleTokenResp := new(googleOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(googleTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}

		status = v1.OAuthAuthRequestStatus{
			ProviderTokenType:      googleTokenResp.TokenType,
			Scope:                  googleTokenResp.Scope,
			ProviderAccessToken:    googleTokenResp.AccessToken,
			ExpiresAt:              metav1.Time{Time: time.Now().Add(time.Second * time.Duration(googleTokenResp.ExpiresIn))},
			Ok:                     true, // Assuming true if no error is present
			ProviderTokenCreatedAt: metav1.Now(),
			ProviderRefreshToken:   googleTokenResp.RefreshToken,
		}
	case types.OAuthAppTypeSalesforce:
		salesforceTokenResp := new(salesforceOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(salesforceTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		issuedAt, err := strconv.ParseInt(salesforceTokenResp.IssuedAt, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		createdAt := time.Unix(issuedAt/1000, (issuedAt%1000)*1000000)

		status = v1.OAuthAuthRequestStatus{
			ProviderTokenType:      salesforceTokenResp.TokenType,
			Scope:                  salesforceTokenResp.Scope,
			ProviderAccessToken:    salesforceTokenResp.AccessToken,
			ExpiresAt:              metav1.Time{Time: time.Now().Add(7200 * time.Second)}, // Relies on Salesforce admin not overriding the default 2 hours
			Ok:                     true,                                                  // Assuming true if no error is present
			ProviderTokenCreatedAt: metav1.NewTime(createdAt),
			ProviderRefreshToken:   salesforceTokenResp.RefreshToken,
			Data: map[string]string{
				"salesforce_instance_url": salesforceTokenResp.InstanceURL,
			},
		}
	case types.OAuthAppTypeGitLab:
		var tokenResp tokenResponse
		// For GitLab, decode the standard token response and then add the base URL to extras
		if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		status.ProviderTokenCreatedAt = metav1.Now()

		// Add GitLab base URL to extras if it's a custom instance
		if app.Spec.Manifest.GitLabBaseURL != "" {
			if status.Data == nil {
				status.Data = make(map[string]string, 1)
			}
			status.Data["gitlab_base_url"] = app.Spec.Manifest.GitLabBaseURL
		}
	default:
		var tokenResp tokenResponse
		if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		status.ProviderTokenCreatedAt = metav1.Now()
	}

	code = strings.ToLower(rand.Text() + rand.Text())

	status.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	status.OAuthAppNamespace = app.Namespace
	status.OAuthAppName = app.Name
	oauthAuthRequest.Status = status

	if err = req.Storage.Status().Update(req.Context(), &oauthAuthRequest); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	if status.Error != "" {
		return fmt.Errorf("failed to get provider token: %s", status.Error)
	}

	redirectWithAuthorizeResponse(req, oauthAuthRequest, code)
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
