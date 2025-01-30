package emailtrigger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"path"
	"strings"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type EmailHandler struct {
	c        kclient.Client
	hostname string
}

func EmailTrigger(c kclient.Client, hostname string) *EmailHandler {
	return &EmailHandler{
		c:        c,
		hostname: hostname,
	}
}

func (h *EmailHandler) Handler(ctx context.Context, from string, to []string, subject string, data []byte) error {
	for _, to := range to {
		toAddr, err := mail.ParseAddress(to)
		if err != nil {
			return fmt.Errorf("parse to address: %w", err)
		}

		name, host, ok := strings.Cut(toAddr.Address, "@")
		if !ok {
			return fmt.Errorf("invalid to address: %s", toAddr.Address)
		}

		if host != h.hostname {
			log.Infof("Skipping mail for %s: not for this host", toAddr.Address)
			continue
		}

		name, ns, _ := strings.Cut(name, "+")
		if ns == "" {
			ns = system.DefaultNamespace
		}

		var emailReceiver v1.EmailReceiver
		if err = alias.Get(ctx, h.c, &emailReceiver, ns, name); apierror.IsNotFound(err) {
			log.Infof("Skipping mail for %s: no receiver found", toAddr.Address)
			continue
		} else if err != nil {
			return fmt.Errorf("get email receiver: %w", err)
		}

		if !matches(from, emailReceiver) {
			log.Infof("Skipping mail for %s: sender not allowed", toAddr.Address)
			continue
		}

		if err = h.dispatchEmail(ctx, emailReceiver, string(data), from, to, subject); err != nil {
			return fmt.Errorf("dispatch email: %w", err)
		}
	}

	return nil
}

func (h *EmailHandler) dispatchEmail(ctx context.Context, email v1.EmailReceiver, body string, from, to, subject string) error {
	var input struct {
		Type    string `json:"type"`
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	input.Type = "email"
	input.From = from
	input.To = to
	input.Subject = subject
	input.Body = body

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	var workflow v1.Workflow
	if err = alias.Get(ctx, h.c, &workflow, email.Namespace, email.Spec.Workflow); err != nil {
		return err
	}

	return h.c.Create(ctx, &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Namespace:    workflow.Namespace,
		},
		Spec: v1.WorkflowExecutionSpec{
			WorkflowName:      workflow.Name,
			EmailReceiverName: email.Name,
			ThreadName:        workflow.Spec.ThreadName,
			Input:             string(inputJSON),
		},
	})
}

func matches(address string, email v1.EmailReceiver) bool {
	if len(email.Spec.AllowedSenders) == 0 {
		return true
	}

	for _, allowedSender := range email.Spec.AllowedSenders {
		if allowedSender == address {
			return true
		}
		matched, _ := path.Match(allowedSender, address)
		if matched {
			return true
		}
	}

	return false
}
