package mcpconnect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// protocolTransport supplies the version negotiated by the actual client. The
// SDK's low-level transport cannot learn it from a ClientSession in this bridge.
type protocolTransport struct {
	base    http.RoundTripper
	mu      sync.RWMutex
	version string
}

func (t *protocolTransport) setVersion(version string) {
	t.mu.Lock()
	t.version = version
	t.mu.Unlock()
}
func (t *protocolTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodDelete {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.Clone(ctx)
	} else {
		r = r.Clone(r.Context())
	}
	t.mu.RLock()
	if r.Header.Get("MCP-Protocol-Version") == "" && t.version != "" {
		r.Header.Set("MCP-Protocol-Version", t.version)
	}
	t.mu.RUnlock()
	return t.base.RoundTrip(r)
}

// bridge forwards wire messages without terminating MCP sessions or inventing
// capabilities. The SDK handles HTTP POST/SSE responses and session identifiers.
func bridge(ctx context.Context, local gomcp.Connection, endpoint string, handler auth.OAuthHandler, base http.RoundTripper) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer local.Close()
	transport := &protocolTransport{base: base}
	client := &http.Client{Transport: transport}
	upstream, err := (&gomcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client, OAuthHandler: handler, DisableStandaloneSSE: true}).Connect(ctx)
	if err != nil {
		return err
	}
	defer func() { cancel(); _ = upstream.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = local.Close(); _ = upstream.Close() })
	defer stop()
	done := make(chan error, 1)
	report := func(err error) {
		select {
		case done <- err:
		default:
		}
	}
	messages := make(chan jsonrpc.Message, 32)
	// Read independently of writes so EOF cancels even an outstanding browser flow.
	go func() {
		for {
			msg, err := local.Read(ctx)
			if err != nil {
				report(err)
				cancel()
				return
			}
			select {
			case messages <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	var initMu sync.Mutex
	var initializeID jsonrpc.ID
	var initializing bool
	calls := make(chan struct{}, 32)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-messages:
				if req, ok := msg.(*jsonrpc.Request); ok && req.Method == "initialize" {
					initMu.Lock()
					initializeID, initializing = req.ID, true
					initMu.Unlock()
				}
				if req, ok := msg.(*jsonrpc.Request); ok && req.IsCall() {
					select {
					case calls <- struct{}{}:
					case <-ctx.Done():
						return
					}
					go func() {
						defer func() { <-calls }()
						if err := upstream.Write(ctx, msg); err != nil {
							report(err)
						}
					}()
				} else if err := upstream.Write(ctx, msg); err != nil {
					report(err)
					return
				}
			}
		}
	}()
	var listenOnce sync.Once
	go func() {
		for {
			msg, err := upstream.Read(ctx)
			if err != nil {
				report(err)
				return
			}
			initMu.Lock()
			if resp, ok := msg.(*jsonrpc.Response); ok && initializing && resp.ID == initializeID {
				initializing = false
				if resp.Error == nil {
					var result struct {
						ProtocolVersion string `json:"protocolVersion"`
					}
					if err := json.Unmarshal(resp.Result, &result); err == nil && result.ProtocolVersion != "" {
						transport.setVersion(result.ProtocolVersion)
						if result.ProtocolVersion < "2026-07-28" {
							listenOnce.Do(func() {
								go func() {
									if err := standalone(ctx, client, endpoint, upstream.SessionID(), handler, local); err != nil {
										report(err)
									}
								}()
							})
						}
					}
				}
			}
			initMu.Unlock()
			if err := local.Write(ctx, msg); err != nil {
				report(err)
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		select {
		case err := <-done:
			if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				return err
			}
		default:
		}
		return nil
	case err := <-done:
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}

// The low-level SDK transport does not start a standalone stream without a
// ClientSession. Maintain that optional legacy stream here; newer stateless
// protocol versions carry server messages on request streams instead.
func standalone(ctx context.Context, client *http.Client, endpoint, session string, handler auth.OAuthHandler, local gomcp.Connection) error {
	lastID := ""
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}
		if handler != nil {
			source, err := handler.TokenSource(ctx)
			if err != nil {
				return err
			}
			if source != nil {
				tok, err := source.Token()
				if err != nil {
					return err
				}
				tok.SetAuthHeader(req)
			}
		}
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound && session == "" {
				_ = resp.Body.Close()
				return nil
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return fmt.Errorf("MCP event stream returned HTTP %d", resp.StatusCode)
			}
			if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
				_ = resp.Body.Close()
				return fmt.Errorf("MCP event stream has invalid content type")
			}
			err = readEvents(ctx, resp.Body, local, &lastID)
			_ = resp.Body.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt >= 5 {
			return fmt.Errorf("MCP event stream disconnected: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func readEvents(ctx context.Context, r io.Reader, local gomcp.Connection, lastID *string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 10<<20)
	var data strings.Builder
	eventID, eventType := "", ""
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 && (eventType == "" || eventType == "message") {
				msg, err := jsonrpc.DecodeMessage([]byte(data.String()))
				if err != nil {
					return err
				}
				if err := local.Write(ctx, msg); err != nil {
					return err
				}
			}
			if eventID != "" {
				*lastID = eventID
			}
			data.Reset()
			eventID, eventType = "", ""
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			if data.Len()+len(value) > 10<<20 {
				return fmt.Errorf("MCP event exceeds 10 MiB")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		case "id":
			if !strings.ContainsRune(value, 0) {
				eventID = value
			}
		case "event":
			eventType = value
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}
