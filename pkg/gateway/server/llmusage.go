package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// llmRoute is how the LLM gateway served a request: the provider and model of the attempt
// that answered, the gateway key it used, and whether that attempt was a fallback.
type llmRoute struct {
	Provider string
	Model    string
	Key      string
	Fallback bool
	// Subscription is true when the caller's Claude subscription paid for the request, which
	// was then served with no gateway key.
	Subscription bool
}

// llmUsageRecorder records one LLM request in Obot's token usage when it ends, whether it
// succeeded or not, so request counts, success rates and latency can be reported from token
// usage alone. It is independent of the LLM audit log, which can be turned off.
type llmUsageRecorder struct {
	once   sync.Once
	start  time.Time
	user   kuser.Info
	client *client.Client
	// model is the requested model ("provider/model"), until the model that served the
	// request is known.
	model  string
	status int
	usage  types.TokenUsage
	// route, when set, returns how the gateway served the request. It is read when the request
	// ends.
	route func() llmRoute
}

// llmRecordingResponseBody records a response that Obot doesn't meter when it has been
// delivered: in the LLM audit log, and in token usage as a request without usage.
type llmRecordingResponseBody struct {
	body   io.ReadCloser
	audit  *llmAuditRecorder
	usage  *llmUsageRecorder
	client *client.Client
}

// meteredLLMEndpoint reports whether a client path under the LLM proxy names an endpoint
// whose requests are recorded in token usage: those that generate tokens.
func meteredLLMEndpoint(clientPath string) bool {
	switch strings.TrimPrefix(strings.Trim(clientPath, "/"), "v1/") {
	case "messages", "responses", "chat/completions", "embeddings":
		return true
	default:
		return false
	}
}

func newLLMUsageRecorder(user kuser.Info, c *client.Client, model string, route func() llmRoute) *llmUsageRecorder {
	return &llmUsageRecorder{
		start:  time.Now(),
		user:   user,
		client: c,
		model:  model,
		route:  route,
	}
}

func (r *llmUsageRecorder) recordResponseStatus(status int) {
	if r == nil {
		return
	}
	r.status = status
}

// setTokenUsage records the usage of the request and the model it is counted against.
func (r *llmUsageRecorder) setTokenUsage(model string, usage types.TokenUsage) {
	if r == nil {
		return
	}
	if model != "" {
		r.model = model
	}
	r.usage = usage
}

// finish records the request once, when it ends; err is why it failed, if it did.
func (r *llmUsageRecorder) finish(err error) {
	if r == nil || r.client == nil {
		return
	}
	r.once.Do(func() {
		// Recorded against the owner for the same reason the limit is checked against them:
		// an agent has no user row of its own, and its usage should still land on whoever
		// created it.
		activity := newRunTokenActivity(r.user, r.model, r.usage)
		activity.Outcome, _, _ = llmOutcome(r.status, err)
		activity.DurationMs = time.Since(r.start).Milliseconds()
		var route llmRoute
		if r.route != nil {
			route = r.route()
		}
		activity.Provider = route.Provider
		if activity.Provider == "" {
			activity.Provider, _, _ = strings.Cut(r.model, "/")
		}
		activity.Fallback = route.Fallback
		if err := r.client.InsertTokenUsage(context.Background(), activity); err != nil {
			slog.Warn("failed to save token usage for user", "userID", activity.UserID, "error", err)
		}
	})
}

func newRunTokenActivity(user kuser.Info, model string, usage types.TokenUsage) *types.RunTokenActivity {
	activity := &types.RunTokenActivity{
		UserID: user.GetUID(),
		Model:  model,
		Usage:  usage,
	}
	if attribution, ok := principal.APIKeyAttributionFromUser(user); ok {
		activity.APIKeyID = &attribution.ID
		activity.APIKeyName = attribution.Name
	}
	return activity
}

// wrapRecordingResponse records a response Obot doesn't meter once it has been delivered.
func wrapRecordingResponse(resp *http.Response, audit *llmAuditRecorder, usage *llmUsageRecorder, client *client.Client) {
	if (audit == nil && usage == nil) || resp == nil || resp.Body == nil {
		return
	}
	resp.Body = &llmRecordingResponseBody{
		body:   resp.Body,
		audit:  audit,
		usage:  usage,
		client: client,
	}
}

func (r *llmRecordingResponseBody) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.audit.captureResponseChunk(p[:n])
	}
	return n, err
}

func (r *llmRecordingResponseBody) Close() error {
	err := r.body.Close()
	r.audit.finish(r.client, err)
	r.usage.finish(err)
	return err
}
