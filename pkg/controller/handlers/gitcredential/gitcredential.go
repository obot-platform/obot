package gitcredential

import (
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	credentialstore "github.com/obot-platform/obot/pkg/gitcredential"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gatewayClient *gclient.Client
}

func New(gatewayClient *gclient.Client) *Handler {
	return &Handler{gatewayClient: gatewayClient}
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	credential := req.Object.(*v1.GitCredential)
	references, err := credentialstore.References(req.Ctx, req.Client, credential.Namespace, credential.Name)
	if err != nil {
		return err
	}
	if len(references) > 0 {
		return fmt.Errorf("git credential is still used by %s", strings.Join(references, ", "))
	}
	return credentialstore.Delete(req.Ctx, h.gatewayClient, credential.Name)
}
