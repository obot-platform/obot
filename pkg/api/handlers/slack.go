package handlers

import (
	"errors"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SlackHandler struct {
	gptScript *gptscript.GPTScript
}

func NewSlackHandler(gptScript *gptscript.GPTScript) *SlackHandler {
	return &SlackHandler{
		gptScript: gptScript,
	}
}

func (s *SlackHandler) Create(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var input types.SlackReceiver

	if err := req.Read(&input); err != nil {
		return err
	}

	if err := validateSlackInput(input, false); err != nil {
		return err
	}

	slackReceiver := v1.SlackReceiver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.SlackReceiverPrefix + thread.Name,
			Namespace: req.Namespace(),
		},
		Spec: v1.SlackReceiverSpec{
			Manifest:   input.SlackReceiverManifest,
			ThreadName: thread.Name,
		},
	}

	if err := req.Create(&slackReceiver); err != nil {
		return err
	}

	if err := req.GPTClient.CreateCredential(req.Context(), newSlackCred(thread.Name, input.ClientSecret, input.SigningSecret, input.AppToken)); err != nil {
		return err
	}

	return req.WriteCreated(convertSlackReceiver(slackReceiver))
}

func newSlackCred(threadName, clientSecret, signingSecret, appToken string) gptscript.Credential {
	return gptscript.Credential{
		Context:  system.OAuthAppPrefix + threadName,
		ToolName: string(types.OAuthAppTypeSlack),
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"CLIENT_SECRET":  clientSecret,
			"SIGNING_SECRET": signingSecret,
			"APP_TOKEN":      appToken,
		},
	}
}

func convertSlackReceiver(slackReceiver v1.SlackReceiver) types.SlackReceiver {
	return types.SlackReceiver{
		Metadata:              MetadataFrom(&slackReceiver),
		SlackReceiverManifest: slackReceiver.Spec.Manifest,
	}
}

func (s *SlackHandler) Update(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var input types.SlackReceiver

	if err := req.Read(&input); err != nil {
		return err
	}

	if err := validateSlackInput(input, true); err != nil {
		return err
	}

	var slackReceiver v1.SlackReceiver
	if err := req.Get(&slackReceiver, system.SlackReceiverPrefix+thread.Name); err != nil {
		return err
	}

	slackReceiver.Spec.Manifest = input.SlackReceiverManifest
	if err := req.Update(&slackReceiver); err != nil {
		return err
	}

	if input.ClientSecret != "" || input.SigningSecret != "" || input.AppToken != "" {
		if err := req.GPTClient.CreateCredential(req.Context(), newSlackCred(slackReceiver.Spec.ThreadName, input.ClientSecret, input.SigningSecret, input.AppToken)); err != nil {
			return err
		}
	}

	return req.Write(convertSlackReceiver(slackReceiver))
}

func (s *SlackHandler) Get(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var slackReceiver v1.SlackReceiver
	if err := req.Get(&slackReceiver, system.SlackReceiverPrefix+thread.Name); err != nil {
		return err
	}

	return req.Write(convertSlackReceiver(slackReceiver))
}

func (s *SlackHandler) Delete(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	return req.Delete(&v1.SlackReceiver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.SlackReceiverPrefix + thread.Name,
			Namespace: req.Namespace(),
		},
	})
}

func validateSlackInput(input types.SlackReceiver, update bool) error {
	if input.AppID == "" {
		return errors.New("appID is required")
	}
	if input.ClientID == "" {
		return errors.New("clientID is required")
	}
	if input.ClientSecret == "" && !update {
		return errors.New("clientSecret is required")
	}
	if input.SigningSecret == "" && !update {
		return errors.New("signingSecret is required")
	}
	return nil
}
