package handlers

import (
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SlackHandler struct {
	gptScript *gptscript.GPTScript
}

func NewSlackHandler(gptScript *gptscript.GPTScript) *SlackHandler {
	return &SlackHandler{
		gptScript: gptScript,
	}
}

func (s *SlackHandler) Configure(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var input v1.SlackReceiverManifest

	if err := req.Read(&input); err != nil {
		return err
	}

	var slackReceiver v1.SlackReceiver

	if req.Method == "POST" {
		var slackReceivers v1.SlackReceiverList
		if err := req.List(&slackReceivers, &client.ListOptions{
			Namespace: req.Namespace(),
		}); err != nil {
			return err
		}

		for _, slackReceiver := range slackReceivers.Items {
			if slackReceiver.Spec.Manifest.AppID == input.AppID {
				return apierrors.NewBadRequest(fmt.Sprintf("Slack app ID %s is already configured for project %s",
					input.AppID, slackReceiver.Name))
			}

			if slackReceiver.Spec.ThreadName == thread.Name {
				return apierrors.NewBadRequest(fmt.Sprintf("Slack receiver for thread %s is already configured", thread.Name))
			}
		}

		slackReceiver = v1.SlackReceiver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.SlackReceiverPrefix + thread.Name,
				Namespace: req.Namespace(),
			},
			Spec: v1.SlackReceiverSpec{
				Manifest: v1.SlackReceiverManifest{
					AppID:    input.AppID,
					ClientID: input.ClientID,
				},
				ThreadName: thread.Name,
			},
		}

		if err := req.Create(&slackReceiver); err != nil {
			return err
		}
	} else if req.Method == "PUT" {
		var slackReceivers v1.SlackReceiverList
		if err := req.List(&slackReceivers, &client.ListOptions{
			Namespace: req.Namespace(),
			FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
				"spec.threadName": thread.Name,
			})),
		}); err != nil {
			return err
		}

		if len(slackReceivers.Items) == 0 {
			return apierrors.NewBadRequest(fmt.Sprintf("Slack receiver for app ID %s not found", input.AppID))
		}

		slackReceiver = slackReceivers.Items[0]
		slackReceiver.Spec.Manifest.ClientID = input.ClientID
		slackReceiver.Spec.Manifest.AppID = input.AppID
		slackReceiver.Spec.Manifest.ClientSecret = ""
		slackReceiver.Spec.Manifest.SigningSecret = ""

		if err := req.Update(&slackReceiver); err != nil {
			return err
		}
	}

	oauthAppName := system.OAuthAppPrefix + thread.Name
	credential := gptscript.Credential{
		// Override the context and the tool name so that oauth app can use the credential directly without having to recreate
		Context:  oauthAppName,
		ToolName: oauthAppName,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"CLIENT_SECRET":  input.ClientSecret,
			"SIGNING_SECRET": input.SigningSecret,
		},
	}

	return req.GPTClient.CreateCredential(req.Context(), credential)
}

func (s *SlackHandler) DeleteConfiguration(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var slackReceivers v1.SlackReceiverList
	if err := req.List(&slackReceivers, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.threadName": thread.Name,
		})),
	}); err != nil {
		return err
	}

	if len(slackReceivers.Items) == 0 {
		return apierrors.NewBadRequest(fmt.Sprintf("Slack receiver for thread %s not found", thread.Name))
	}

	return req.Delete(&slackReceivers.Items[0])
}
