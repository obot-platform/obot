package mcpgateway

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Match the MCPClientSession cleanup controller's seven-day idle retention.
	// Enforce it during requests too, since cleanup may not have deleted an expired record yet.
	mcpSessionMaxIdleTime = 7 * 24 * time.Hour
)

// Ownership records use a separate key space from untrusted audit metadata.
type sessionOwnership struct {
	ctx                  context.Context
	storage              kclient.Client
	connection, user, id string
	initialize, virtual  bool
	method               string
	clientInfo           clientInfo
	mcpServerID          string
}

func (s *sessionOwnership) key(id string) kclient.ObjectKey {
	return kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      name.SafeConcatName("mcp-session-owner", hash.String([]string{s.connection, s.user, id})),
	}
}

func suppliedSessionID(req *http.Request) (string, error) {
	// Only the header identifies a Streamable HTTP session. Query parameters are upstream inputs.
	id := mcpSessionID(req.Header, nil)

	for _, value := range req.Header.Values(mcpSessionHeader) {
		if value == "" {
			continue
		}

		if id != value {
			return "", fmt.Errorf("conflicting MCP session identifiers")
		}
	}

	return id, nil
}

func (s *sessionOwnership) prepare(req *http.Request) (int, error) {
	id, err := suppliedSessionID(req)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.id = id
	s.method = req.Method

	if id != "" {
		var record v1.MCPClientSession
		err = s.storage.Get(s.ctx, s.key(id), &record)
		if apierrors.IsNotFound(err) {
			return http.StatusNotFound, fmt.Errorf("MCP session not found")
		}

		if err != nil {
			return http.StatusServiceUnavailable, err
		}

		if record.Spec.ConnectionID != s.connection || record.Spec.UserID != s.user || time.Since(record.Status.LastUsed.Time) > mcpSessionMaxIdleTime {
			return http.StatusNotFound, fmt.Errorf("MCP session not found")
		}

		s.virtual = record.Spec.Virtual
		s.clientInfo = clientInfo{Name: record.Spec.ClientName, Version: record.Spec.ClientVersion}
		s.mcpServerID = record.Spec.MCPServerID
		record.Status.LastUsed = metav1.Now()
		if err = s.storage.Status().Update(s.ctx, &record); err != nil && !apierrors.IsConflict(err) {
			return http.StatusServiceUnavailable, err
		}
	}

	return 0, nil
}

// trackAudit shares client metadata with auditing without creating another session record.
func (s *sessionOwnership) trackAudit(audit *proxyAudit) {
	if audit == nil {
		return
	}

	s.initialize = audit.initialize
	s.mcpServerID = audit.entry.Metadata["mcpID"]
	s.clientInfo.Name = cmp.Or(s.clientInfo.Name, audit.entry.ClientName)
	s.clientInfo.Version = cmp.Or(s.clientInfo.Version, audit.entry.ClientVersion)
	audit.entry.ClientName = s.clientInfo.Name
	audit.entry.ClientVersion = s.clientInfo.Version
}

func (s *sessionOwnership) register(id string, virtual bool) error {
	key := s.key(id)
	record := v1.MCPClientSession{
		Name:      key.Name,
		Namespace: key.Namespace,
		Spec: v1.MCPClientSessionSpec{
			MCPServerID:   s.mcpServerID,
			ClientName:    s.clientInfo.Name,
			ClientVersion: s.clientInfo.Version,
			ConnectionID:  s.connection,
			UserID:        s.user,
			Virtual:       virtual,
		},
	}
	if err := s.storage.Create(s.ctx, &record); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		if err = s.storage.Get(s.ctx, key, &record); err != nil {
			return err
		}

		if record.Spec.ConnectionID != s.connection || record.Spec.UserID != s.user || record.Spec.Virtual != virtual {
			return fmt.Errorf("inconsistent session ownership")
		}
	}

	record.Status.LastUsed = metav1.Now()

	if err := s.storage.Status().Update(s.ctx, &record); err != nil && !apierrors.IsConflict(err) {
		return err
	}

	return nil
}

func (s *sessionOwnership) response(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	if s.method == http.MethodDelete && s.id != "" {
		record := v1.MCPClientSession{
			Name:      s.key(s.id).Name,
			Namespace: system.DefaultNamespace,
		}
		if err := s.storage.Delete(s.ctx, &record); err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		return nil
	}

	id := mcpSessionID(resp.Header, nil)
	if resp.StatusCode == http.StatusOK && (s.initialize || (s.id == "" && id != "")) {
		virtual := id == ""
		if virtual {
			id = uuid.NewString()
		}

		if err := s.register(id, virtual); err != nil {
			return err
		}

		s.virtual = virtual
		resp.Header.Set(mcpSessionHeader, id)
	}

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" && resp.Body != nil {
		body, _, err := decodeMCPHookBody(resp.Body, resp.Header.Get("Content-Encoding"))
		if err != nil {
			return err
		}

		resp.Body = &streamableSSEBody{source: body, reader: bufio.NewReader(body)}
		resp.ContentLength = -1
		clearMCPHookResponseHeaders(resp.Header)
	}

	return nil
}

type streamableSSEBody struct {
	source io.ReadCloser
	reader *bufio.Reader
	output []byte
	err    error
}

func (b *streamableSSEBody) Close() error { return b.source.Close() }

// Read is called as the reverse proxy streams an upstream SSE response to the client.
// It passes Streamable HTTP events through and rejects legacy endpoint events before delivery.
func (b *streamableSSEBody) Read(p []byte) (int, error) {
	for len(b.output) == 0 {
		if b.err != nil {
			return 0, b.err
		}

		raw, lines, err := readMCPHookSSEEvent(b.reader)
		b.err = err

		var event string

		for _, line := range lines {
			if v, ok := strings.CutPrefix(line.value, "event:"); ok {
				event = strings.TrimSpace(v)
			}
		}

		if event == "endpoint" {
			b.err = fmt.Errorf("legacy HTTP+SSE transport is not supported; use Streamable HTTP")
			return 0, b.err
		}

		b.output = raw
	}

	n := copy(p, b.output)
	b.output = b.output[n:]

	return n, nil
}
