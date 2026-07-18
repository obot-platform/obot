// Package hostedagent contains placeholder orchestration for hosted agent
// instances.
//
// Nothing here deploys anything. The handler marks instances ready and hands
// back a synthetic URL so that the surrounding feature — access rules, the
// API, and the UI — can be exercised end to end. Real orchestration is
// expected to replace this handler wholesale.
//
// Agents themselves are templates and are not reconciled at all; every agent
// is used through per-user instances, and only instances carry state.
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

// OrchestrateInstance assigns a URL to an instance.
func (h *Handler) OrchestrateInstance(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.HostedAgentInstance)

	// Without this guard every status update would retrigger this handler and
	// mint a new URL forever.
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
