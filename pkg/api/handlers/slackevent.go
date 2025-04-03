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
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SlackEventHandler struct {
	gptscript *gptscript.GPTScript
}

func NewSlackEventHandler(gptscript *gptscript.GPTScript) *SlackEventHandler {
	return &SlackEventHandler{gptscript: gptscript}
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

func (h *SlackEventHandler) validateRequest(req api.Context, event SlackEvent, body []byte) error {
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

	timestamp := req.Request.Header.Get("X-Slack-Request-Timestamp")
	signature := req.Request.Header.Get("X-Slack-Signature")

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return types.NewErrBadRequest("invalid timestamp")
	}

	for _, receiver := range slackReceivers.Items {
		var (
			oauthApp      v1.OAuthApp
			signingSecret string
		)

		var oauthApps v1.OAuthAppList
		if err := req.List(&oauthApps, &client.ListOptions{
			Namespace: req.Namespace(),
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.slackReceiverName": receiver.Name,
			}),
		}); err != nil {
			return err
		}

		if len(oauthApps.Items) != 1 {
			continue
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

		return nil
	}

	return types.NewErrBadRequest("no oauth app found for this event")
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

	if event.APIAppID == "" {
		return types.NewErrBadRequest("missing api_app_id")
	}

	if event.TeamID == "" {
		return types.NewErrBadRequest("missing team_id")
	}

	if err := h.validateRequest(req, event, body); err != nil {
		return err
	}

	var slackTriggers v1.SlackTriggerList

	if err := req.List(&slackTriggers, client.MatchingFields{
		"spec.appID":  event.APIAppID,
		"spec.teamID": event.TeamID,
	}); err != nil {
		return err
	}

	var errs []error
	for _, trigger := range slackTriggers.Items {
		var workflows v1.WorkflowList
		if err := req.List(&workflows, client.MatchingFields{
			"spec.threadName": trigger.Spec.ThreadName,
			"spec.slack":      "true",
		}); err != nil {
			return err
		}

		var payload = &strings.Builder{}
		if err := json.NewEncoder(payload).Encode(map[string]interface{}{
			"type":  "slack",
			"event": json.RawMessage(body),
		}); err != nil {
			return err
		}

		for _, workflow := range workflows.Items {
			err := req.Create(&v1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.WorkflowExecutionPrefix,
					Namespace:    req.Namespace(),
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
		return types.NewErrBadRequest("failed to create workflow execution: %v", errs)
	}

	return req.Write("ok")
}
