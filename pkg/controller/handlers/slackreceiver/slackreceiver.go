package slackreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayTypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript  *gptscript.GPTScript
	subscribed map[string]context.CancelFunc
	storage    storage.Client
}

func NewHandler(gptScript *gptscript.GPTScript, storage storage.Client) *Handler {
	return &Handler{gptScript: gptScript, subscribed: make(map[string]context.CancelFunc), storage: storage}
}

func CreateOAuthApp(req router.Request, _ router.Response) error {
	slackReceiver := req.Object.(*v1.SlackReceiver)

	oauthAppName := system.OAuthAppPrefix + slackReceiver.Spec.ThreadName

	oauthApp := v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: slackReceiver.Namespace,
			Name:      oauthAppName,
		},
		Spec: v1.OAuthAppSpec{
			Manifest: types.OAuthAppManifest{
				ClientID: slackReceiver.Spec.Manifest.ClientID,
				Alias:    string(types.OAuthAppTypeSlack),
				Type:     types.OAuthAppTypeSlack,
			},
			ThreadName:        slackReceiver.Spec.ThreadName,
			SlackReceiverName: slackReceiver.Name,
		},
	}

	if err := req.Get(&oauthApp, slackReceiver.Namespace, oauthApp.Name); apierrors.IsNotFound(err) {
		if err := gatewayTypes.ValidateAndSetDefaultsOAuthAppManifest(&oauthApp.Spec.Manifest, true); err != nil {
			return err
		}
		return req.Client.Create(req.Ctx, &oauthApp)
	} else if err != nil {
		return err
	}

	if oauthApp.Spec.Manifest.ClientID != slackReceiver.Spec.Manifest.ClientID {
		oauthApp.Spec.Manifest.ClientID = slackReceiver.Spec.Manifest.ClientID
		return req.Client.Update(req.Ctx, &oauthApp)
	}

	return nil
}

func (s *Handler) SubscribeToSlackEvents(req router.Request, resp router.Response) error {
	slackReceiver := req.Object.(*v1.SlackReceiver)

	if s.subscribed[slackReceiver.Name] != nil {
		return nil
	}

	cred, err := s.gptScript.RevealCredential(req.Ctx, []string{system.OAuthAppPrefix + slackReceiver.Spec.ThreadName}, string(types.OAuthAppTypeSlack))
	if err != nil {
		return err
	}

	slackAppToken := cred.Env["APP_TOKEN"]

	if slackAppToken == "" {
		return nil
	}

	slackClient := slack.New("", slack.OptionAppLevelToken(slackAppToken))
	client := socketmode.New(slackClient)

	h := &eventHandler{
		ctx:     req.Ctx,
		storage: s.storage,
	}

	socketmodeHandler := socketmode.NewSocketmodeHandler(client)

	socketmodeHandler.Handle(socketmode.EventTypeConnecting, middlewareConnecting)
	socketmodeHandler.Handle(socketmode.EventTypeConnectionError, middlewareConnectionError)
	socketmodeHandler.Handle(socketmode.EventTypeConnected, middlewareConnected)

	// Handle a specific event from EventsAPI
	socketmodeHandler.HandleEvents(slackevents.AppMention, h.middlewareAppMentionEvent)

	ctx, cancel := context.WithCancel(context.Background())
	s.subscribed[slackReceiver.Name] = cancel

	go func() {
		if err := socketmodeHandler.RunEventLoopContext(ctx); err != nil {
			fmt.Printf("error running event loop: %v", err)
		}
		delete(s.subscribed, slackReceiver.Name)
		resp.RetryAfter(time.Second * 10)
	}()

	return nil
}

func (s *Handler) UnsubscribeFromSlackEvents(req router.Request, _ router.Response) error {
	slackReceiver := req.Object.(*v1.SlackReceiver)
	s.subscribed[slackReceiver.Name]()

	delete(s.subscribed, slackReceiver.Name)
	return nil
}

type eventHandler struct {
	ctx     context.Context
	storage storage.Client
}

func (h *eventHandler) middlewareAppMentionEvent(evt *socketmode.Event, slackClient *socketmode.Client) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	slackClient.Ack(*evt.Request)

	ev, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
	if !ok {
		return
	}

	if ev.BotID != "" {
		return
	}

	var slackTriggers v1.SlackTriggerList
	if err := h.storage.List(h.ctx, &slackTriggers, client.MatchingFields{
		"spec.appID":  eventsAPIEvent.APIAppID,
		"spec.teamID": eventsAPIEvent.TeamID,
	}); err != nil {
		fmt.Printf("failed to list slack triggers: %v", err)
		return
	}

	var errs []error
	for _, trigger := range slackTriggers.Items {
		var workflows v1.WorkflowList
		if err := h.storage.List(h.ctx, &workflows, client.MatchingFields{
			"spec.threadName": trigger.Spec.ThreadName,
			"spec.slack":      "true",
		}); err != nil {
			fmt.Printf("failed to list workflows: %v", err)
			return
		}

		var payload = &strings.Builder{}
		if err := json.NewEncoder(payload).Encode(map[string]interface{}{
			"type":  "slack",
			"event": ev,
		}); err != nil {
			fmt.Printf("failed to encode payload: %v", err)
			return
		}

		for _, workflow := range workflows.Items {
			err := h.storage.Create(h.ctx, &v1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.WorkflowExecutionPrefix,
					Namespace:    workflow.Namespace,
				},
				Spec: v1.WorkflowExecutionSpec{
					Input:        payload.String(),
					ThreadName:   workflow.Spec.ThreadName,
					WorkflowName: workflow.Name,
				},
			})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		fmt.Printf("failed to create workflow executions: %v", errors.Join(errs...))
	}
}

func middlewareConnecting(_ *socketmode.Event, _ *socketmode.Client) {
	log.Println("Connecting to Slack with Socket Mode...")
}

func middlewareConnectionError(_ *socketmode.Event, _ *socketmode.Client) {
	log.Println("Connection failed. Retrying later...")
}

func middlewareConnected(_ *socketmode.Event, _ *socketmode.Client) {
	log.Println("Connected to Slack with Socket Mode.")
}
