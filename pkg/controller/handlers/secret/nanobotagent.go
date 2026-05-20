package secret

import (
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	corev1 "k8s.io/api/core/v1"
)

type Handler struct {
	mcpDeploymentNamespace string
	gatewayClient          *gateway.Client
}

func New(mcpNamespace string, gatewayClient *gateway.Client) *Handler {
	return &Handler{
		mcpDeploymentNamespace: mcpNamespace,
		gatewayClient:          gatewayClient,
	}
}

func (h *Handler) UpdateNanobotAgentCreds(req router.Request, _ router.Response) error {
	secret := req.Object.(*corev1.Secret)
	mcpServerID, ok := strings.CutSuffix(secret.Name, "-mcp-files")
	if !ok {
		return nil
	}

	userID, ok := secret.Annotations["mcp-user-id"]
	if !ok || userID == "" {
		return nil
	}

	cred, err := h.gatewayClient.RevealCredential(req.Ctx, []string{fmt.Sprintf("%s-%s", userID, mcpServerID)}, mcpServerID)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil
		}
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	update := len(secret.Data) != len(cred.Secrets)
	for key, val := range cred.Secrets {
		if string(secret.Data[fmt.Sprintf("%s-%s", mcpServerID, key)]) != val {
			update = true
			if secret.Data == nil {
				secret.Data = make(map[string][]byte, len(cred.Secrets))
			}
			secret.Data[fmt.Sprintf("%s-%s", mcpServerID, key)] = []byte(val)
		}
	}

	if update {
		if err = req.Client.Update(req.Ctx, secret); err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	return nil
}
