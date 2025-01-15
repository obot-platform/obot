package sendgrid

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/emailtrigger"
	"github.com/sendgrid/sendgrid-go/helpers/inbound"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type InboundWebhookHandler struct {
	emailTrigger *emailtrigger.EmailHandler
	username     string
	password     string
}

func NewInboundWebhookHandler(c kclient.Client, hostname string, username, password string) *InboundWebhookHandler {
	emailTrigger := emailtrigger.EmailTrigger(c, hostname)
	return &InboundWebhookHandler{emailTrigger: emailTrigger, username: username, password: password}
}

func (h *InboundWebhookHandler) InboundWebhookHandler(req api.Context) error {
	if h.username != "" && h.password != "" {
		username, password, ok := req.Request.BasicAuth()
		if !ok || username != h.username || password != h.password {
			return types.NewErrHttp(http.StatusUnauthorized, "Invalid credentials")
		}
	}

	inboundEmail, err := inbound.Parse(req.Request)
	if err != nil {
		return types.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("Failed to parse inbound email: %v", err))
	}

	subject := inboundEmail.Headers["Subject"]
	if err := h.emailTrigger.Handler(req.Context(), inboundEmail.Envelope.From, inboundEmail.Envelope.To, subject, []byte(inboundEmail.TextBody)); err != nil {
		return types.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("Failed to handle inbound email: %v", err))
	}

	req.WriteHeader(http.StatusOK)
	return nil
}
