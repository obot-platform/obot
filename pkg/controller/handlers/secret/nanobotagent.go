package secret

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	corev1 "k8s.io/api/core/v1"
)

type Handler struct {
	mcpDeploymentNamespace string
	gptClient              *gptscript.GPTScript
}

func New(mcpNamespace string, gptClient *gptscript.GPTScript) *Handler {
	return &Handler{
		mcpDeploymentNamespace: mcpNamespace,
		gptClient:              gptClient,
	}
}

func (h *Handler) UpdateNanobotAgentCreds(req router.Request, _ router.Response) error {
	secret := req.Object.(*corev1.Secret)
	mcpServerID, ok := strings.CutSuffix(secret.Name, "-files")
	if !ok {
		return nil
	}

	cred, err := h.gptClient.RevealCredential(req.Ctx, []string{fmt.Sprintf("%s-%s", secret.Annotations["mcp-user-id"], mcpServerID)}, mcpServerID)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil
		}
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	update := len(secret.Data) != len(cred.Env)
	for key, val := range cred.Env {
		if string(secret.Data[fmt.Sprintf("%s-%s", mcpServerID, key)]) != val {
			update = true
			if secret.Data == nil {
				secret.Data = make(map[string][]byte, len(cred.Env))
			}
			secret.Data[fmt.Sprintf("%s-%s", mcpServerID, key)] = []byte(val)
		}
	}

	if update {
		err := req.Client.Update(req.Ctx, secret)
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	return nil
}
