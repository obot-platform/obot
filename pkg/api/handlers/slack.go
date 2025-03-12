package handlers

import (
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayTypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			AppID         string `json:"appID"`
			SigningSecret string `json:"signingSecret"`
			ClientID      string `json:"clientID"`
			ClientSecret  string `json:"clientSecret"`
		}
	)

	if err := req.Read(&input); err != nil {
		return err
	}

	var threads v1.ThreadList
	if err := req.List(&threads, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.slackConfiguration.appID": input.AppID,
		}),
	}); err != nil {
		return err
	}

	for _, existingThread := range threads.Items {
		if existingThread.Name != thread.Name {
			return apierrors.NewBadRequest(fmt.Sprintf("Slack app ID %s is already configured for project %s",
				input.AppID, existingThread.Name))
		}
	}

	oauthAppName := getSlackOauthAppFromThreadName(thread.Name)

	// Create OAuth app manifest for Slack
	appManifest := &types.OAuthAppManifest{
		Type:     types.OAuthAppTypeSlack,
		ClientID: input.ClientID,
		Alias:    oauthAppName,
	}

	if err := gatewayTypes.ValidateAndSetDefaultsOAuthAppManifest(appManifest, true); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("invalid OAuth app: %s", err))
	}

	if req.Method == "POST" {
		// Create the OAuth app
		app := v1.OAuthApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oauthAppName,
				Namespace: req.Namespace(),
			},
			Spec: v1.OAuthAppSpec{
				Manifest: *appManifest,
			},
		}

		if err := req.Create(&app); err != nil {
			return err
		}
	} else if req.Method == "PUT" {
		var app v1.OAuthApp
		if err := req.Get(&app, appManifest.Alias); err != nil {
			return err
		}

		app.Spec.Manifest = *appManifest
		if err := req.Update(&app); err != nil {
			return err
		}
	}
	thread.Spec.Manifest.OauthApps = append(thread.Spec.Manifest.OauthApps, oauthAppName)

	// Store client secret as credential
	credential := gptscript.Credential{
		Context:  oauthAppName,
		ToolName: oauthAppName,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"CLIENT_SECRET":  input.ClientSecret,
			"SIGNING_SECRET": input.SigningSecret,
		},
	}

	if err := req.GPTClient.CreateCredential(req.Context(), credential); err != nil {
		return err
	}

	thread.Spec.SlackConfiguration = &v1.SlackConfiguration{
		AppID: input.AppID,
	}

	if err := req.Storage.Update(req.Context(), thread); err != nil {
		return err
	}

	r := types.OAuthApp{
		OAuthAppManifest: types.OAuthAppManifest{Name: oauthAppName},
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
			Name:      getSlackOauthAppFromThreadName(thread.Name),
			Namespace: req.Namespace(),
		},
	}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	thread.Spec.SlackConfiguration = nil

	if err := req.Storage.Update(req.Context(), thread); err != nil {
		return err
	}

	return req.Write(struct{}{})
}

func getSlackOauthAppFromThreadName(name string) string {
	return "slack-" + name
}
