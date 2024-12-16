package smtp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"path"
	"strings"

	"github.com/acorn-io/acorn/logger"
	"github.com/acorn-io/acorn/pkg/alias"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/mhale/smtpd"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Server struct {
	s        smtpd.Server
	c        kclient.Client
	ctx      context.Context
	hostname string
}

func Start(ctx context.Context, c kclient.Client, hostname string) {
	s := Server{
		s: smtpd.Server{
			Addr: ":2525",
		},
		c:        c,
		ctx:      ctx,
		hostname: hostname,
	}
	s.s.Handler = s.handler
	go func() {
		err := s.s.ListenAndServe()
		select {
		case <-ctx.Done():
		default:
			log.Fatalf("smtp server shutdown: %v", err)
		}
	}()
}

func (s *Server) handler(_ net.Addr, from string, to []string, data []byte) error {
	log.Infof("New mail received from %s for %s: length=%d", from, to, len(data))

	message, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	body, err := getBody(message)
	if err != nil {
		return fmt.Errorf("get body: %w", err)
	}

	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("parse from address: %w", err)
	}

	for _, to := range to {
		toAddr, err := mail.ParseAddress(to)
		if err != nil {
			return fmt.Errorf("parse to address: %w", err)
		}

		name, host, ok := strings.Cut(toAddr.Address, "@")
		if !ok {
			return fmt.Errorf("invalid to address: %s", toAddr.Address)
		}

		if host != s.hostname {
			log.Infof("Skipping mail for %s: not for this host", toAddr.Address)
			continue
		}

		ns, name, ok := strings.Cut(name, ".")
		if !ok {
			log.Infof("Skipping mail for %s: no namespace found", toAddr.Address)
		}

		var emailReceiver v1.EmailReceiver
		if err := alias.Get(s.ctx, s.c, &emailReceiver, ns, name); apierror.IsNotFound(err) {
			log.Infof("Skipping mail for %s: no receiver found", toAddr.Address)
			continue
		} else if err != nil {
			return fmt.Errorf("get email receiver: %w", err)
		}

		if !matches(fromAddress.Address, emailReceiver) {
			log.Infof("Skipping mail for %s: sender not allowed", toAddr.Address)
			continue
		}

		if len(emailReceiver.Spec.AllowedSenders) > 0 {
			for _, allowedSender := range emailReceiver.Spec.AllowedSenders {
				if allowedSender == fromAddress.Address {
					break
				}
			}
		}

		if err := s.dispatchEmail(emailReceiver, body, message); err != nil {
			return fmt.Errorf("dispatch email: %w", err)
		}
	}

	return err
}

func getBody(message *mail.Message) (string, error) {
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}

	var html string
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(message.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				if errors.Is(err, io.EOF) {
					// Break and return whatever html is found or an error.
					break
				}
				return "", err
			}
			if strings.HasPrefix(p.Header.Get("Content-Type"), "text/plain") {
				d, err := io.ReadAll(p)
				if err != nil {
					return "", err
				}
				if p.Header.Get("Content-Transfer-Encoding") == "base64" {
					d, err = base64.StdEncoding.DecodeString(string(d))
				}
				return string(d), err
			}
			if strings.HasPrefix(p.Header.Get("Content-Type"), "text/html") {
				d, err := io.ReadAll(p)
				if err != nil {
					return "", err
				}
				html = string(d)
			}
		}
	}

	if html != "" {
		return html, nil
	}

	return "", fmt.Errorf("failed to find text/plain body: %s", mediaType)
}

func (s *Server) dispatchEmail(email v1.EmailReceiver, body string, message *mail.Message) error {
	var input struct {
		Type    string `json:"type"`
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	input.Type = "email"
	input.From = message.Header.Get("From")
	input.To = message.Header.Get("To")
	input.Subject = message.Header.Get("Subject")
	input.Body = body

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	var workflow v1.Workflow
	if err := alias.Get(s.ctx, s.c, &workflow, email.Namespace, email.Spec.Workflow); err != nil {
		return err
	}

	return s.c.Create(s.ctx, &v1.WorkflowExecution{
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
