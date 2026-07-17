// Package hostedagent contains placeholder orchestration for hosted agents.
//
// Nothing here deploys anything. The handlers mark agents and instances ready
// and hand back a synthetic URL so that the surrounding feature — access rules,
// the API, and the UI — can be exercised end to end. Real orchestration is
// expected to replace these handlers wholesale.
package hostedagent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	serverURL string
}

func New(serverURL string) *Handler {
	return &Handler{serverURL: serverURL}
}

func randomSlug() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate slug: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (h *Handler) fakeURL(name string) (string, error) {
	slug, err := randomSlug()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/hosted/%s-%s", h.serverURL, name, slug), nil
}

// Orchestrate assigns a URL to shared agents. Per-user agents are served by
// their instances instead, so the agent itself never carries a URL.
func (h *Handler) Orchestrate(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.HostedAgent)

	if agent.Spec.Manifest.PerUser {
		if agent.Status.State == "" && agent.Status.URL == "" && agent.Status.Error == "" {
			return nil
		}

		// The agent was switched from shared to per-user; drop the stale URL.
		agent.Status.State = ""
		agent.Status.URL = ""
		agent.Status.Error = ""
		return req.Client.Status().Update(req.Ctx, agent)
	}

	// Without this guard every status update would retrigger this handler and
	// mint a new URL forever.
	if agent.Status.State == types.HostedAgentStateReady && agent.Status.URL != "" {
		return nil
	}

	url, err := h.fakeURL(agent.Name)
	if err != nil {
		return err
	}

	agent.Status.State = types.HostedAgentStateReady
	agent.Status.URL = url
	agent.Status.Error = ""

	return req.Client.Status().Update(req.Ctx, agent)
}

// OrchestrateInstance assigns a URL to a per-user instance.
func (h *Handler) OrchestrateInstance(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.HostedAgentInstance)

	if instance.Status.State == types.HostedAgentStateReady && instance.Status.URL != "" {
		return nil
	}

	url, err := h.fakeURL(instance.Name)
	if err != nil {
		return err
	}

	instance.Status.State = types.HostedAgentStateReady
	instance.Status.URL = url
	instance.Status.Error = ""

	return req.Client.Status().Update(req.Ctx, instance)
}
