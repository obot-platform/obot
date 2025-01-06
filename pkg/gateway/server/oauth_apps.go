package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	kcontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mvl"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// oAuthCleanup is a background task that deletes temporary OAuth-related objects that were created
// more than five minutes ago.
func (s *Server) oAuthCleanup(ctx context.Context) {
	logger := kcontext.GetLogger(ctx)
	t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Delete token responses that are older than five minutes.
			var responses []types.OAuthTokenResponse
			if err := s.db.WithContext(ctx).Find(&responses).Error; err != nil {
				logger.Debug("failed to get responses", "error", err)
			} else {
				for _, response := range responses {
					if time.Since(response.CreatedAt) > 5*time.Minute {
						if err = s.db.WithContext(ctx).Where("state = ?", response.State).Delete(&response).Error; err != nil {
							logger.Debug("failed to delete response", "error", err)
						}
					}
				}
			}

			// Delete token request challenges that are older than five minutes.
			var challenges []types.OAuthTokenRequestChallenge
			if err := s.db.WithContext(ctx).Find(&challenges).Error; err != nil {
				logger.Debug("failed to get challenges", "error", err)
			} else {
				for _, challenge := range challenges {
					if time.Since(challenge.CreatedAt) > 5*time.Minute {
						if err := s.db.WithContext(ctx).Delete(&challenge).Error; err != nil {
							kcontext.GetLogger(ctx).Debug("failed to delete challenge", "error", err)
						}
					}
				}
			}
		}
	}
}

// listOAuthApps lists all the OAuth app registrations in the database.
func (s *Server) listOAuthApps(apiContext api.Context) error {
	var apps v1.OAuthAppList
	if err := apiContext.List(&apps); err != nil {
		return err
	}

	resp := make([]types2.OAuthApp, 0, len(apps.Items))
	for _, app := range apps.Items {
		app.Spec.Manifest.ClientSecret = ""
		resp = append(resp, convertOAuthAppRegistrationToOAuthApp(app, s.baseURL))
	}

	return apiContext.Write(types2.OAuthAppList{Items: resp})
}

// oauthAppByID gets a single OAuth app registration from the database based on its ID.
func (s *Server) oauthAppByID(apiContext api.Context) error {
	var app v1.OAuthApp
	if err := apiContext.Get(&app, apiContext.PathValue("id")); err != nil {
		return err
	}

	return apiContext.Write(convertOAuthAppRegistrationToOAuthApp(app, s.baseURL))
}

// createOAuthApp creates a new OAuth app registration in the database (admin only).
func (s *Server) createOAuthApp(apiContext api.Context) error {
	appManifest := new(types2.OAuthAppManifest)
	if err := apiContext.Read(appManifest); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid OAuth app: %s", err))
	}

	if err := types.ValidateAndSetDefaultsOAuthAppManifest(appManifest, true); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid OAuth app: %s", err))
	}

	// Ensure that the integration is unique.
	var existingApps v1.OAuthAppList
	if err := apiContext.Storage.List(apiContext.Context(), &existingApps, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.manifest.integration": appManifest.Integration,
		})),
		Namespace: apiContext.Namespace(),
	}); err != nil {
		return err
	}

	if len(existingApps.Items) > 0 {
		return types2.NewErrHttp(http.StatusConflict, fmt.Sprintf("OAuth app with integration %s already exists", appManifest.Integration))
	}

	app := v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.OAuthAppPrefix,
			Namespace:    apiContext.Namespace(),
		},
		Spec: v1.OAuthAppSpec{
			Manifest: *appManifest,
		},
	}
	if err := apiContext.Create(&app); err != nil {
		return err
	}

	return apiContext.Write(convertOAuthAppRegistrationToOAuthApp(app, s.baseURL))
}

// updateOAuthApp updates an existing OAuth app registration in the database (admin only).
func (s *Server) updateOAuthApp(apiContext api.Context) error {
	var appManifest types2.OAuthAppManifest
	if err := apiContext.Read(&appManifest); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid OAuth app: %s", err))
	}

	// See if the app exists first, and return a 404 if it doesn't.
	var originalApp v1.OAuthApp
	if err := apiContext.Get(&originalApp, apiContext.PathValue("id")); err != nil {
		return err
	}

	merged := types.MergeOAuthAppManifests(originalApp.Spec.Manifest, appManifest)
	if err := types.ValidateAndSetDefaultsOAuthAppManifest(&merged, false); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid OAuth app: %s", err))
	}

	// Update the app.
	originalApp.Spec.Manifest = merged
	if err := apiContext.Update(&originalApp); err != nil {
		return err
	}

	return apiContext.Write(convertOAuthAppRegistrationToOAuthApp(originalApp, s.baseURL))
}

// deleteOAuthApp deletes an existing OAuth app registration from the database (admin only).
func (s *Server) deleteOAuthApp(apiContext api.Context) error {
	return apiContext.Delete(&v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(apiContext.PathValue("id")),
			Namespace: apiContext.Namespace(),
		},
	})
}

// authorizeOAuthApp starts the OAuth authorization code flow to allow the Acorn Gateway to access
// a third-party API on the user's behalf. The user will go to this route in their browser,
// which will redirect them to the authorization URL for the configured OAuth app registration.
func (s *Server) authorizeOAuthApp(apiContext api.Context) error {
	app, err := getOAuthAppFromName(apiContext)
	if err != nil {
		return err
	}

	// Check for required query parameters: state, scope, and challenge.
	var (
		state     = apiContext.URL.Query().Get("state")
		scope     = apiContext.URL.Query().Get("scope")
		challenge = apiContext.URL.Query().Get("challenge")
	)
	if state == "" {
		return apierrors.NewBadRequest("missing state query parameter")
	} else if len(state) < 64 || len(state) > 256 {
		return apierrors.NewBadRequest("invalid state length - must be between 64 and 256 characters")
	} else if challenge == "" {
		return apierrors.NewBadRequest("missing challenge query parameter")
	}

	c := new(types.OAuthTokenRequestChallenge)
	// Save the challenge to the database. This will be used later when the cred tool requests the token.
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state = ?", state).First(c).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// If the challenge already exists, don't error, just return.
		if !c.CreatedAt.IsZero() {
			return nil
		}

		return tx.Create(&types.OAuthTokenRequestChallenge{
			State:     state,
			Challenge: challenge,
		}).Error
	}); err != nil {
		return err
	}

	// If the challenge already exists, redirect the user to the "complete" page instead of putting them through the normal OAuth flow.
	// This would happen if the user clicked on the "Authorize" link multiple times.
	if !c.CreatedAt.IsZero() {
		http.Redirect(apiContext.ResponseWriter, apiContext.Request, s.authCompleteURL(), http.StatusFound)
		return nil
	}

	// Construct URL to redirect the user to.
	u, err := url.Parse(app.Spec.Manifest.AuthURL)
	if err != nil { // This should never happen unless someone updates the database directly with an invalid URL.
		return fmt.Errorf("failed to parse auth URL %q: %w", app.Spec.Manifest.AuthURL, err)
	}

	q := u.Query()

	q.Set("response_type", "code")
	q.Set("client_id", app.Spec.Manifest.ClientID)
	q.Set("redirect_uri", app.RedirectURL(s.baseURL))
	q.Set("state", state)

	// HubSpot supports setting optional scopes in this query param so that we can support an app that is able to have broad permissions,
	// while at the same time only granting specific stuff.
	if app.Spec.Manifest.Type == types2.OAuthAppTypeHubSpot {
		q.Set("optional_scope", app.Spec.Manifest.OptionalScope)
	}

	// Atlassian requires the audience and prompt parameters to be set.
	// See https://developer.atlassian.com/cloud/jira/platform/oauth-2-3lo-apps/#1--direct-the-user-to-the-authorization-url-to-get-an-authorization-code
	// for details.
	if app.Spec.Manifest.Type == types2.OAuthAppTypeAtlassian {
		q.Set("audience", "api.atlassian.com")
		q.Set("prompt", "consent")
	}

	// For Google: access_type=offline instructs Google to return a refresh token and an access token on the initial authorization.
	// This can be used to refresh the access token when a user is not present at the browser
	// prompt=consent instructs Google to show the consent screen every time the authorization flow happens so that we get a new refresh token.
	if app.Spec.Manifest.Type == types2.OAuthAppTypeGoogle {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	}

	// Slack is annoying and makes us call this query parameter user_scope instead of scope.
	// user_scope is used for delegated user permissions (which is what we want), while just scope is used for bot permissions.
	if app.Spec.Manifest.Type == types2.OAuthAppTypeSlack {
		q.Set("user_scope", scope)
	} else {
		q.Set("scope", scope)
	}

	u.RawQuery = q.Encode()

	// Return a 302 to redirect.
	http.Redirect(apiContext.ResponseWriter, apiContext.Request, u.String(), http.StatusFound)
	return nil
}

// refreshOAuthApp is a route that the cred tool will hit to refresh an OAuth token using a refresh token.
func (s *Server) refreshOAuthApp(apiContext api.Context) error {
	app, err := getOAuthAppFromName(apiContext)
	if err != nil {
		return err
	}

	var (
		scope        = apiContext.URL.Query().Get("scope")
		refreshToken = apiContext.URL.Query().Get("refresh_token")
	)
	if refreshToken == "" {
		return apierrors.NewBadRequest("missing refresh_token query parameter")
	}

	data := url.Values{}
	data.Set("client_id", app.Spec.Manifest.ClientID)
	data.Set("client_secret", app.Spec.Manifest.ClientSecret)
	data.Set("scope", scope)
	data.Set("redirect_uri", app.RedirectURL(s.baseURL))
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", app.Spec.Manifest.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to make token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBuf := new(bytes.Buffer)
		_, _ = bodyBuf.ReadFrom(resp.Body)
		return fmt.Errorf("failed to get tokens: %d %s", resp.StatusCode, bodyBuf.String())
	}

	tokenResp := new(types.OAuthTokenResponse)
	if err := json.NewDecoder(resp.Body).Decode(tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	if app.Spec.Manifest.Type == types2.OAuthAppTypeGoogle {
		tokenResp.RefreshToken = refreshToken
	}

	return apiContext.Write(tokenResp)
}

// callbackOAuthApp is the callback route that the OAuth provider will redirect the user to after they have authorized the app.
// This route will exchange the authorization code for an access token and store it in the database, so that
// the cred tool can request it.
func (s *Server) callbackOAuthApp(apiContext api.Context) error {
	app, err := getOAuthAppFromName(apiContext)
	if err != nil {
		return err
	}

	// Check for the query parameters.
	var (
		code         = apiContext.URL.Query().Get("code")
		state        = apiContext.URL.Query().Get("state")
		e            = apiContext.URL.Query().Get("error")
		eDescription = apiContext.URL.Query().Get("error_description")
	)
	if e != "" {
		return apierrors.NewBadRequest(fmt.Sprintf("error: %s (%s)", e, eDescription))
	}

	if code == "" {
		return apierrors.NewBadRequest("missing code query parameter")
	} else if state == "" {
		return apierrors.NewBadRequest("missing state query parameter")
	}

	// Build and make the request to get the tokens.
	data := url.Values{}
	data.Set("client_id", app.Spec.Manifest.ClientID)
	data.Set("client_secret", app.Spec.Manifest.ClientSecret) // Including the client secret in the body is not strictly required in the OAuth2 RFC, but some providers require it anyway.
	data.Set("code", code)
	data.Set("redirect_uri", app.RedirectURL(s.baseURL))
	data.Set("grant_type", "authorization_code")

	if app.Spec.Manifest.Type == types2.OAuthAppTypeHubSpot {
		data.Set("optional_scope", app.Spec.Manifest.OptionalScope)
	}

	req, err := http.NewRequest("POST", app.Spec.Manifest.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if app.Spec.Manifest.Type != types2.OAuthAppTypeGoogle {
		req.SetBasicAuth(url.QueryEscape(app.Spec.Manifest.ClientID), url.QueryEscape(app.Spec.Manifest.ClientSecret))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBuf := new(bytes.Buffer)
		_, _ = bodyBuf.ReadFrom(resp.Body)
		return fmt.Errorf("failed to get tokens: %d %s", resp.StatusCode, bodyBuf.String())
	}

	// Get the response and save it to the db so that the cred tool can acquire it.
	// Once again, Slack and GitHub are annoying and do their own thing.
	tokenResp := new(types.OAuthTokenResponse)
	switch app.Spec.Manifest.Type {
	case types2.OAuthAppTypeSlack:
		slackTokenResp := new(types.SlackOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(slackTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}

		tokenResp = &types.OAuthTokenResponse{
			State:       state,
			Scope:       slackTokenResp.AuthedUser.Scope,
			AccessToken: slackTokenResp.AuthedUser.AccessToken,
			Ok:          slackTokenResp.Ok,
			Error:       slackTokenResp.Error,
			CreatedAt:   time.Now(),
		}
	case types2.OAuthAppTypeGitHub:
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
		tokenResp = &types.OAuthTokenResponse{
			State:       state,
			TokenType:   values.Get("token_type"),
			Scope:       values.Get("scope"),
			AccessToken: values.Get("access_token"),
			Ok:          true, // Assuming true if no error is present
			CreatedAt:   time.Now(),
		}
	case types2.OAuthAppTypeGoogle:
		googleTokenResp := new(types.GoogleOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(googleTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}

		tokenResp = &types.OAuthTokenResponse{
			State:        state,
			TokenType:    googleTokenResp.TokenType,
			Scope:        googleTokenResp.Scope,
			AccessToken:  googleTokenResp.AccessToken,
			ExpiresIn:    googleTokenResp.ExpiresIn,
			Ok:           true, // Assuming true if no error is present
			CreatedAt:    time.Now(),
			RefreshToken: googleTokenResp.RefreshToken,
		}
	case types2.OAuthAppTypeSalesforce:
		salesforceTokenResp := new(types.SalesforceOAuthTokenResponse)
		if err := json.NewDecoder(resp.Body).Decode(salesforceTokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		issuedAt, err := strconv.ParseInt(salesforceTokenResp.IssuedAt, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		createdAt := time.Unix(issuedAt/1000, (issuedAt%1000)*1000000)

		tokenResp = &types.OAuthTokenResponse{
			State:        state,
			TokenType:    salesforceTokenResp.TokenType,
			Scope:        salesforceTokenResp.Scope,
			AccessToken:  salesforceTokenResp.AccessToken,
			ExpiresIn:    7200, // Relies on Salesforce admin not overriding the default 2 hours
			Ok:           true, // Assuming true if no error is present
			CreatedAt:    createdAt,
			RefreshToken: salesforceTokenResp.RefreshToken,
			Extras: map[string]string{
				"GPTSCRIPT_SALESFORCE_URL": salesforceTokenResp.InstanceURL,
			},
		}
	default:
		if err := json.NewDecoder(resp.Body).Decode(tokenResp); err != nil {
			return fmt.Errorf("failed to parse token response: %w", err)
		}
		tokenResp.State = state
		tokenResp.CreatedAt = time.Now()
	}

	if tokenResp.Error != "" {
		return fmt.Errorf("failed to get tokens: %s", tokenResp.Error)
	}

	if err := s.db.WithContext(apiContext.Context()).Create(tokenResp).Error; err != nil {
		return fmt.Errorf("failed to save token response: %w", err)
	}

	http.Redirect(apiContext.ResponseWriter, apiContext.Request, s.authCompleteURL(), http.StatusFound)
	return nil
}

// getTokenOAuthApp is a route that the cred tool will hit to get the OAuth token response after the user has authorized the app.
// The cred tool must be able to provide the state parameter that it first generated in order to prove that it is the one that
// started the OAuth flow.
func (s *Server) getTokenOAuthApp(apiContext api.Context) error {
	var (
		state    = apiContext.URL.Query().Get("state")
		verifier = apiContext.URL.Query().Get("verifier")
	)
	if state == "" {
		return apierrors.NewBadRequest("missing state query parameter")
	} else if verifier == "" {
		return apierrors.NewBadRequest("missing verifier query parameter")
	}

	// Look up the challenge by the state.
	var challenge types.OAuthTokenRequestChallenge
	if err := s.db.WithContext(apiContext.Context()).First(&challenge, "state = ?", state).Error; err != nil {
		return types2.NewErrNotFound("challenge not found")
	}

	// Verify the verifier by taking the SHA256 hash and checking it against the challenge.
	h := sha256.New()
	h.Write([]byte(verifier))
	hash := hex.EncodeToString(h.Sum(nil))
	if hash != challenge.Challenge {
		// This is an invalid request, possibly an unauthorized attempt to obtain a token.
		// Return a 404 to mask that this matched a real challenge.
		return types2.NewErrHttp(http.StatusNotFound, "challenge not found")
	}

	// Look up the token response by the state.
	var tokenResp types.OAuthTokenResponse
	if err := s.db.WithContext(apiContext.Context()).First(&tokenResp, "state = ?", state).Error; err != nil {
		return types2.NewErrNotFound("token response not found")
	}

	// Delete the challenge and token response from the database.
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&challenge).Error; err != nil {
			return err
		}

		return tx.Where("state = ?", state).Delete(&tokenResp).Error
	}); err != nil {
		logger := mvl.Package()
		logger.Debugf("failed to delete OAuth token request challenge: %v", err)
	}

	return apiContext.Write(tokenResp)
}

func convertOAuthAppRegistrationToOAuthApp(app v1.OAuthApp, baseURL string) types2.OAuthApp {
	appManifest := app.Spec.Manifest
	appManifest.ClientSecret = ""
	links := make([]string, 0, 6)
	if redirectURL := app.RedirectURL(baseURL); redirectURL != "" {
		links = append(links, "redirectURL", redirectURL)
	}
	if authorizeURL := app.AuthorizeURL(baseURL); authorizeURL != "" {
		links = append(links, "authorizeURL", authorizeURL)
	}
	if refreshURL := app.RefreshURL(baseURL); refreshURL != "" {
		links = append(links, "refreshURL", refreshURL)
	}
	appManifest.Metadata = handlers.MetadataFrom(&app, links...)
	return types2.OAuthApp{
		OAuthAppManifest: appManifest,
	}
}

func getOAuthAppFromName(apiContext api.Context) (*v1.OAuthApp, error) {
	var oauthApp v1.OAuthApp
	if err := alias.Get(apiContext.Context(), apiContext.Storage, &oauthApp, apiContext.Namespace(), apiContext.PathValue("id")); err != nil {
		return nil, err
	}
	return &oauthApp, nil
}
