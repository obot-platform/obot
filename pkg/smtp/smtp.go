package smtp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"

	"github.com/mhale/smtpd"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/emailtrigger"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Server struct {
	s            smtpd.Server
	ctx          context.Context
	emailTrigger *emailtrigger.EmailHandler
}

func Start(ctx context.Context, c kclient.Client, hostname string) {
	emailTrigger := emailtrigger.EmailTrigger(c, hostname)
	s := Server{
		s: smtpd.Server{
			Addr: ":2525",
		},
		ctx:          ctx,
		emailTrigger: emailTrigger,
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

	return s.emailTrigger.Handler(s.ctx, fromAddress.Address, to, message.Header.Get("Subject"), []byte(body))
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
	} else if strings.HasPrefix(mediaType, "text/plain") || strings.HasPrefix(mediaType, "text/html") {
		d, err := io.ReadAll(message.Body)
		if err != nil {
			return "", err
		}
		if message.Header.Get("Content-Transfer-Encoding") == "base64" {
			d, err = base64.StdEncoding.DecodeString(string(d))
			if err != nil {
				return "", err
			}
		}
		html = string(d)
	}

	if html != "" {
		return html, nil
	}

	return "", fmt.Errorf("failed to find text/plain body: %s", mediaType)
}
