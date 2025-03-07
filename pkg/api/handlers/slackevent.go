package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SlackEventHandler struct {
	invoker   *invoke.Invoker
	gptscript *gptscript.GPTScript
}

func NewSlackEventHandler(invoker *invoke.Invoker, gptscript *gptscript.GPTScript) *SlackEventHandler {
	return &SlackEventHandler{invoker: invoker, gptscript: gptscript}
}

type SlackEvent struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	TeamID    string `json:"team_id"`
	APIAppID  string `json:"api_app_id"`
	Event     struct {
		Type        string `json:"type"`
		User        string `json:"user"`
		Text        string `json:"text"`
		ThreadTS    string `json:"thread_ts"`
		ChannelType string `json:"channel_type"`
		Channel     string `json:"channel"`
		EventTS     string `json:"event_ts"`
		TS          string `json:"ts"`
	} `json:"event"`
}

func (h *SlackEventHandler) HandleEvent(req api.Context) error {
	body, err := io.ReadAll(req.Request.Body)
	if err != nil {
		return types.NewErrBadRequest("failed to read request body: %v", err)
	}
	req.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var event SlackEvent
	if err := json.NewDecoder(bytes.NewBuffer(body)).Decode(&event); err != nil {
		return types.NewErrBadRequest("failed to decode event: %v", err)
	}

	if event.Type == "url_verification" {
		return req.Write(map[string]string{"challenge": event.Challenge})
	}

	if event.Event.Type != "app_mention" {
		return req.Write(map[string]string{"status": "ignored"})
	}

	var slackReceivers v1.SlackReceiverList
	if err := req.List(&slackReceivers, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.manifest.appID": event.APIAppID,
		}),
	}); err != nil {
		return err
	}

	if len(slackReceivers.Items) == 0 {
		return types.NewErrBadRequest("no slack receiver found for app ID")
	}

	slackReceiver := slackReceivers.Items[0]

	var projectThread v1.Thread
	if err := req.Get(&projectThread, slackReceiver.Spec.ThreadName); err != nil {
		return err
	}

	var triggerList v1.SlackTriggerList
	if err := req.List(&triggerList, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": projectThread.Name,
		}),
	}); err != nil {
		return err
	}

	if len(triggerList.Items) == 0 {
		return types.NewErrBadRequest("no trigger found for thread")
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, triggerList.Items[0].Spec.WorkflowName); err != nil {
		return err
	}

	timestamp := req.Request.Header.Get("X-Slack-Request-Timestamp")
	signature := req.Request.Header.Get("X-Slack-Signature")

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return types.NewErrBadRequest("invalid timestamp")
	}

	var (
		oauthApp      v1.OAuthApp
		signingSecret string
	)

	var oauthApps v1.OAuthAppList
	if err := req.List(&oauthApps, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": projectThread.Name,
		}),
	}); err != nil {
		return err
	}

	if len(oauthApps.Items) == 0 {
		return types.NewErrBadRequest("no oauth app found for thread")
	}

	oauthApp = oauthApps.Items[0]

	cred, err := h.gptscript.RevealCredential(req.Context(), []string{oauthApp.Name}, oauthApp.Spec.Manifest.Alias)
	if err != nil {
		return err
	}

	signingSecret = cred.Env["SIGNING_SECRET"]
	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(sigBase))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return types.NewErrBadRequest("invalid signature")
	}

	threadID := event.Event.ThreadTS
	if event.Event.ThreadTS == "" {
		threadID = event.Event.TS
	}
	input := fmt.Sprintf(`{"CHANNEL_ID":%q,"THREAD_ID":%q,"USER_MESSAGE":%q,"USER_ID":%q}`, event.Event.Channel, threadID, event.Event.Text, event.Event.User)

	_, err = h.invoker.Workflow(req.Context(), req.Storage, &workflow, input, invoke.WorkflowOptions{
		StepID: "*",
	})
	if err != nil {
		return err
	}

	return req.Write("ok")
}
