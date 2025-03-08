package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	slackOAuthURL = "https://slack.com/oauth/v2/authorize"
)

func (p *ProjectsHandler) Configure(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var (
		input struct {
			ClientID      string `json:"clientID"`
			ClientSecret  string `json:"clientSecret"`
			SigningSecret string `json:"signingSecret"`
		}
	)

	if err := req.Read(&input); err != nil {
		return err
	}

	// Create OAuth app manifest for Slack
	appManifest := &types.OAuthAppManifest{
		Type:     types.OAuthAppTypeSlack,
		ClientID: input.ClientID,
		Alias:    getOauthAppFromThreadName(thread.Name),
	}

	// Create the OAuth app
	app := v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getOauthAppFromThreadName(thread.Name),
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppSpec{
			Manifest:   *appManifest,
			ThreadName: thread.Name,
		},
	}

	if err := req.Create(&app); err != nil {
		if apierrors.IsAlreadyExists(err) {
			var existing v1.OAuthApp
			if err := req.Get(&existing, app.Name); err != nil {
				return err
			}
			existing.Spec.Manifest = *appManifest
			if err := req.Update(&app); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Store client secret as credential
	credential := gptscript.Credential{
		Context:  app.Name,
		ToolName: appManifest.Alias,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"CLIENT_SECRET":  input.ClientSecret,
			"SIGNING_SECRET": input.SigningSecret,
		},
	}

	if err := req.GPTClient.CreateCredential(req.Context(), credential); err != nil {
		return err
	}

	r := types.OAuthApp{
		OAuthAppManifest: types.OAuthAppManifest{Name: app.Name},
	}

	return req.Write(r)
}

func (p *ProjectsHandler) DeleteConfiguration(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Delete(&v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "slack-" + thread.Name,
			Namespace: req.Namespace(),
		},
	}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	thread.Status.SlackConfiguration = nil

	if err := req.Storage.Status().Update(req.Context(), thread); err != nil {
		return err
	}

	return req.Write(struct{}{})
}

func (p *ProjectsHandler) SlackAuthorize(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var app v1.OAuthApp
	if err := req.Get(&app, getOauthAppFromThreadName(thread.Name)); err != nil {
		return err
	}

	scopes := []string{
		"app_mentions:read",
		"channels:history",
		"channels:read",
		"chat:write",
		"files:read",
		"groups:history",
		"groups:read",
		"groups:write",
		"im:history",
		"im:read",
		"im:write",
		"mpim:history",
		"mpim:write",
		"team:read",
		"users:read",
		"assistant:write",
	}
	userScopes := []string{
		"channels:history",
		"groups:history",
		"im:history",
		"mpim:history",
		"channels:read",
		"files:read",
		"im:read",
		"search:read",
		"team:read",
		"users:read",
		"groups:read",
		"chat:write",
		"groups:write",
		"mpim:write",
		"im:write",
	}

	// Construct the Slack OAuth URL
	redirectURL := fmt.Sprintf("%s?client_id=%s&scope=%s&user_scope=%s&redirect_uri=%s",
		slackOAuthURL,
		app.Spec.Manifest.ClientID,
		url.QueryEscape(strings.Join(scopes, ",")),
		url.QueryEscape(strings.Join(userScopes, ",")),
		url.QueryEscape(fmt.Sprintf("https://%s/api/slack/oauth/callback/%s", req.Host, app.Name)))

	http.Redirect(req.ResponseWriter, req.Request, redirectURL, http.StatusFound)

	return nil
}

func (p *ProjectsHandler) SlackCallback(req api.Context) error {
	oauthAppID := req.PathValue("id")

	var app v1.OAuthApp
	if err := req.Get(&app, oauthAppID); err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, app.Spec.ThreadName); err != nil {
		return err
	}

	code := req.Request.URL.Query().Get("code")
	if code == "" {
		return types.NewErrBadRequest("missing code parameter")
	}

	// Get client secret from credentials
	cred, err := p.gptScript.RevealCredential(req.Context(), []string{app.Name}, app.Spec.Manifest.Alias)
	if err != nil {
		return types.NewErrBadRequest("failed to reveal credential: %s", err)
	}

	clientSecret := cred.Env["CLIENT_SECRET"]

	// Exchange code for access token
	data := url.Values{}
	data.Set("client_id", app.Spec.Manifest.ClientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", fmt.Sprintf("https://%s/api/slack/oauth/callback/%s", req.Host, app.Name))

	resp, err := http.PostForm("https://slack.com/api/oauth.v2.access", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Ok          bool   `json:"ok"`
		AccessToken string `json:"access_token"`
		AppID       string `json:"app_id"`
		Team        struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"team"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Ok {
		return fmt.Errorf("slack oauth error: %s", result.Error)
	}

	// Update thread with Slack configuration
	thread.Status.SlackConfiguration = &v1.SlackConfiguration{
		Teams: v1.SlackTeam{
			ID:   result.Team.ID,
			Name: result.Team.Name,
		},
		AppID: result.AppID,
	}

	if err := req.Storage.Status().Update(req.Context(), &thread); err != nil {
		return err
	}

	http.Redirect(req.ResponseWriter, req.Request, fmt.Sprintf("https://%s/login_complete", req.Host), http.StatusTemporaryRedirect)
	return nil
}

func getOauthAppFromThreadName(name string) string {
	return "slack-" + name
}
