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
	// 1. Read and verify the request
	body, err := io.ReadAll(req.Request.Body)
	if err != nil {
		return types.NewErrBadRequest("failed to read request body: %v", err)
	}
	req.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var event SlackEvent
	if err := json.NewDecoder(bytes.NewBuffer(body)).Decode(&event); err != nil {
		return types.NewErrBadRequest("failed to decode event: %v", err)
	}

	// Handle Slack URL verification challenge
	if event.Type == "url_verification" {
		return req.Write(map[string]string{"challenge": event.Challenge})
	}

	// Only handle app_mention events
	if event.Event.Type != "app_mention" {
		return req.Write(map[string]string{"status": "ignored"})
	}

	// 2. Find triggers matching the team ID
	var triggerList v1.SlackTriggerList
	if err := req.List(&triggerList); err != nil {
		return err
	}

	var matchingTrigger *v1.SlackTrigger
	for _, trigger := range triggerList.Items {
		if trigger.Spec.TeamID == event.TeamID && trigger.Spec.AppID == event.APIAppID {
			matchingTrigger = &trigger
			break
		}
	}

	if matchingTrigger == nil {
		return types.NewErrBadRequest("no trigger found for team ID")
	}

	// 3. Get workflow and thread
	var workflow v1.Workflow
	if err := req.Get(&workflow, matchingTrigger.Spec.WorkflowName); err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, workflow.Spec.ThreadName); err != nil {
		return err
	}

	// Verify request signature
	timestamp := req.Request.Header.Get("X-Slack-Request-Timestamp")
	signature := req.Request.Header.Get("X-Slack-Signature")

	// Reject requests older than 5 minutes
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return types.NewErrBadRequest("invalid timestamp")
	}

	// Get OAuth app for signing secret
	var (
		oauthApp      v1.OAuthApp
		signingSecret string
	)
	if err := req.Get(&oauthApp, getOauthAppFromThreadName(thread.Name)); err != nil {
		return err
	}
	cred, err := h.gptscript.RevealCredential(req.Context(), []string{oauthApp.Name}, oauthApp.Spec.Manifest.Alias)
	if err != nil {
		return err
	}

	// Verify signature
	signingSecret = cred.Env["SIGNING_SECRET"]
	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(sigBase))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return types.NewErrBadRequest("invalid signature")
	}

	// 4. Trigger workflow with parameters
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
