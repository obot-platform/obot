package mcpgateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/obot-platform/nanobot/pkg/mcp/auditlogs"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mcpSessionHeader        = "Mcp-Session-Id"
	mcpSessionDeleteRequest = `{"jsonrpc":"2.0","method":"session/delete"}`

	proxyMessageUnknown      proxyMessageKind = 0
	proxyMessageRequest      proxyMessageKind = 1
	proxyMessageNotification proxyMessageKind = 2
)

type proxyAuditCollector interface {
	auditlogs.Collector
	CollectMCPProxyAuditEntry(entry auditlogs.MCPAuditLog, responseReceived bool, proxyExchangeID string)
}

// proxyMessageKind controls whether an HTTP exchange is logged immediately or
// awaits a correlated JSON-RPC response.
type proxyMessageKind int

// proxyAudit only retains state for one proxied HTTP exchange. Protocol
// requests and responses are emitted as separate audit entries; the database
// persistence layer correlates same-exchange entries by proxyExchangeID and
// uses protocol session metadata only for legacy cross-request responses.
type proxyAudit struct {
	collector       proxyAuditCollector
	storage         kclient.Client
	ctx             context.Context
	entry           auditlogs.MCPAuditLog
	method          string
	kind            proxyMessageKind
	initialize      bool
	proxyExchangeID string

	responseHooksByID       map[string]hookResult
	streamRequestHooksByID  map[string]hookResult
	streamNotificationHooks []hookResult
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type auditResponseBody struct {
	io.ReadCloser
	audit      *proxyAudit
	statusCode int
	body       bytes.Buffer
	once       sync.Once
}

type auditSSEBody struct {
	io.ReadCloser
	audit      *proxyAudit
	statusCode int
	buffer     []byte
	event      string
	data       []string
	once       sync.Once
}

func newProxyAudit(req *http.Request, metadata map[string]string, collector proxyAuditCollector, storageClient kclient.Client) (*proxyAudit, error) {
	if collector == nil || metadata[obotmcp.AuditLogIgnore] == "true" {
		return nil, nil
	}

	entry := buildMCPProxyAuditEntry(req, metadata)
	kind := proxyMessageUnknown
	if req.Method == http.MethodDelete {
		entry.RequestBody = json.RawMessage(mcpSessionDeleteRequest)
		kind = proxyMessageNotification
	} else if req.Body != nil {
		body, err := io.ReadAll(io.LimitReader(req.Body, maxMCPProxyHookBodySize+1))
		if err != nil {
			_ = req.Body.Close()
			return nil, err
		}
		if err := req.Body.Close(); err != nil {
			return nil, err
		}

		if len(body) > maxMCPProxyHookBodySize {
			return nil, fmt.Errorf("request body too large")
		}

		req.Body = io.NopCloser(bytes.NewReader(body))
		entry.RequestBody = jsonBody(body)
		kind = populateMCPMessageFields(&entry, body)
	}

	audit := &proxyAudit{
		collector:              collector,
		storage:                storageClient,
		ctx:                    req.Context(),
		entry:                  entry,
		method:                 req.Method,
		kind:                   kind,
		initialize:             entry.CallType == "initialize",
		responseHooksByID:      make(map[string]hookResult),
		streamRequestHooksByID: make(map[string]hookResult),
	}
	if kind == proxyMessageRequest {
		audit.proxyExchangeID = rand.Text()
	}
	virtualSession, err := saveMCPClientSession(audit.ctx, audit.storage, &audit.entry, false)
	if err != nil {
		slog.ErrorContext(req.Context(), "failed to load MCP client session for audit logging", "error", err)
	}

	// If this is a virtual session, then remove the session ID header from the request because the upstream server isn't expecting it.
	if virtualSession {
		req.Header.Del(mcpSessionHeader)
	}

	return audit, nil
}

func (a *proxyAudit) recordRequestHooks(result hookResult) {
	if a == nil {
		return
	}
	a.entry.WebhookStatuses = append(a.entry.WebhookStatuses, auditHookStatuses(result.statuses)...)
	if result.mutated {
		a.entry.MutatedRequestBody = result.mutatedBody
	}
}

func (a *proxyAudit) recordResponseHooks(requestID string, result hookResult) {
	if a == nil || requestID == "" {
		return
	}
	a.responseHooksByID[requestID] = result
}

func (a *proxyAudit) recordStreamRequestHooks(requestID string, result hookResult) {
	if a == nil {
		return
	}
	if requestID == "" {
		a.streamNotificationHooks = append(a.streamNotificationHooks, result)
		return
	}
	a.streamRequestHooksByID[requestID] = result
}

func (a *proxyAudit) takeStreamRequestHooks(requestID string) (hookResult, bool) {
	if a == nil {
		return hookResult{}, false
	}
	if requestID == "" {
		if len(a.streamNotificationHooks) == 0 {
			return hookResult{}, false
		}
		result := a.streamNotificationHooks[0]
		a.streamNotificationHooks = a.streamNotificationHooks[1:]
		return result, true
	}
	result, ok := a.streamRequestHooksByID[requestID]
	delete(a.streamRequestHooksByID, requestID)
	return result, ok
}

func (a *proxyAudit) applyResponseHooks(entry *auditlogs.MCPAuditLog, requestID string) {
	if a == nil || requestID == "" {
		return
	}
	result, ok := a.responseHooksByID[requestID]
	if !ok {
		return
	}

	delete(a.responseHooksByID, requestID)
	entry.WebhookStatuses = append(entry.WebhookStatuses, auditHookStatuses(result.statuses)...)
	if result.responseChanged {
		entry.OriginalResponseBody = result.originalBody
	}
	if result.err != nil {
		entry.Error = result.err.Error()
	}
}

func auditHookStatuses(statuses []hookStatus) []auditlogs.MCPWebhookStatus {
	result := make([]auditlogs.MCPWebhookStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, auditlogs.MCPWebhookStatus{
			Type: status.typeName, Method: status.method, Name: status.name, Tool: status.tool, Status: status.status, Message: status.message,
		})
	}
	return result
}

func (a *proxyAudit) recordBlockedRequest(body []byte, err error) {
	if a == nil {
		return
	}
	if a.kind == proxyMessageNotification {
		a.recordNotification(http.StatusOK, err)
		return
	}
	a.recordResponse(body, http.StatusOK, err, a.entry.RequestID)
}

func buildMCPProxyAuditEntry(req *http.Request, metadata map[string]string) auditlogs.MCPAuditLog {
	headers, _ := json.Marshal(sanitizedMCPHeaders(req.Header))
	clientIP := req.RemoteAddr
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}

	entry := auditlogs.MCPAuditLog{
		Metadata:       maps.Clone(metadata),
		CreatedAt:      time.Now(),
		Subject:        metadata["userID"],
		ClientIP:       strings.TrimSpace(clientIP),
		SessionID:      mcpSessionID(req.Header, req.URL),
		UserAgent:      req.UserAgent(),
		RequestHeaders: headers,
	}

	switch req.Method {
	case http.MethodGet:
		entry.CallType = "sse/stream"
	case http.MethodDelete:
		entry.CallType = "session/delete"
	}

	if auth := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "); auth != "" && strings.Count(auth, ".") != 2 {
		entry.APIKey = auditlogs.RedactAPIKey(auth)
	}
	return entry
}

func populateMCPMessageFields(entry *auditlogs.MCPAuditLog, body []byte) proxyMessageKind {
	var msg obotmcp.Message
	if decodeMCPHookMessage(body, &msg) != nil {
		return proxyMessageUnknown
	}

	entry.RequestID = obotmcp.MessageIDString(msg.ID)
	if msg.Method == "" {
		return proxyMessageUnknown
	}

	entry.CallType = msg.Method
	entry.CallIdentifier = mcpCallIdentifier(msg)
	populateMCPClientInfo(entry, msg)
	if entry.RequestID == "" {
		return proxyMessageNotification
	}
	return proxyMessageRequest
}

func populateMCPClientInfo(entry *auditlogs.MCPAuditLog, msg obotmcp.Message) {
	var params struct {
		ClientInfo clientInfo `json:"clientInfo"`
		Meta       struct {
			ClientInfo clientInfo `json:"io.modelcontextprotocol/clientInfo"`
		} `json:"_meta"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	if params.Meta.ClientInfo.Name != "" || params.Meta.ClientInfo.Version != "" {
		entry.ClientName = params.Meta.ClientInfo.Name
		entry.ClientVersion = params.Meta.ClientInfo.Version
	} else if msg.Method == "initialize" {
		entry.ClientName = params.ClientInfo.Name
		entry.ClientVersion = params.ClientInfo.Version
	}
}

func mcpCallIdentifier(msg obotmcp.Message) string {
	var params struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		return ""
	}
	if msg.Method == "resources/read" {
		return params.URI
	}
	if msg.Method == "tools/call" || msg.Method == "prompts/get" {
		return params.Name
	}
	return ""
}

func (a *proxyAudit) wrapResponse(resp *http.Response) error {
	if a == nil {
		return nil
	}

	var virtual bool
	if sessionID := resp.Header.Get(mcpSessionHeader); sessionID != "" {
		a.entry.SessionID = sessionID
	} else if a.entry.CallType == "initialize" && resp.StatusCode == http.StatusOK {
		a.entry.SessionID = uuid.New().String()
		resp.Header.Set(mcpSessionHeader, a.entry.SessionID)
		virtual = true
	}

	if _, err := saveMCPClientSession(a.ctx, a.storage, &a.entry, virtual); err != nil {
		slog.Error("failed to save MCP client session for audit logging", "error", err)
	}
	responseHeaders, _ := json.Marshal(sanitizedMCPHeaders(resp.Header))
	a.entry.ResponseHeaders = responseHeaders
	a.recordNotification(resp.StatusCode, nil)

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		resp.Body = newAuditSSEBody(resp.Body, a, resp.StatusCode)
		return nil
	}

	// 202 only acknowledges delivery. The JSON-RPC response will arrive on a
	// separate stream and will be persisted as its own audit entry there.
	if a.method == http.MethodPost && resp.StatusCode == http.StatusAccepted {
		return nil
	}

	resp.Body = &auditResponseBody{
		ReadCloser: resp.Body,
		audit:      a,
		statusCode: resp.StatusCode,
	}
	return nil
}

func (a *proxyAudit) recordRequest() {
	if a == nil || a.method != http.MethodPost {
		return
	}
	if a.kind != proxyMessageRequest {
		return
	}
	entry := a.entry
	entry.ResponseHeaders = nil
	a.submit(entry, false)
}

func (a *proxyAudit) recordNotification(statusCode int, responseErr error) {
	if a == nil || a.kind != proxyMessageNotification {
		return
	}
	entry := a.entry
	entry.ResponseStatus = statusCode
	if responseErr != nil {
		entry.Error = responseErr.Error()
	}
	entry.SetProcessingTime()
	a.submit(entry, true)
}

func (a *proxyAudit) recordHTTPResponse(body []byte, statusCode int, readErr error) {
	if a == nil {
		return
	}
	if a.kind == proxyMessageNotification {
		return
	}
	if a.method == http.MethodPost {
		if a.kind != proxyMessageRequest {
			return
		}
		a.recordResponse(body, statusCode, readErr, a.entry.RequestID)
		return
	}

	// DELETE and non-streaming GET requests are HTTP-level audit events and do
	// not have a JSON-RPC request ID to correlate during persistence.
	entry := a.entry
	entry.ResponseBody = jsonBody(body)
	entry.ResponseStatus = statusCode
	if readErr != nil && readErr != io.EOF {
		entry.Error = readErr.Error()
	}
	entry.SetProcessingTime()
	a.submit(entry, true)
}

func (a *proxyAudit) recordTransportError(err error, statusCode int) {
	if a == nil {
		return
	}
	if a.kind == proxyMessageNotification {
		a.recordNotification(statusCode, err)
		return
	}
	if a.kind == proxyMessageRequest {
		a.recordResponse(nil, statusCode, err, a.entry.RequestID)
		return
	}
	if a.method != http.MethodPost {
		entry := a.entry
		entry.ResponseStatus = statusCode
		entry.Error = err.Error()
		entry.SetProcessingTime()
		a.submit(entry, true)
	}
}

func (a *proxyAudit) recordResponse(body []byte, statusCode int, readErr error, fallbackRequestID string) {
	if a == nil {
		return
	}

	var (
		requestID = fallbackRequestID
		msg       obotmcp.Message
	)
	if decodeMCPHookMessage(body, &msg) == nil && msg.Method == "" {
		requestID = obotmcp.MessageIDString(msg.ID)
	}
	if requestID == "" {
		return
	}

	entry := a.newResponseEntry(requestID)
	entry.ResponseBody = jsonBody(body)
	entry.ResponseStatus = statusCode
	a.applyResponseHooks(&entry, requestID)
	if readErr != nil && readErr != io.EOF {
		entry.Error = readErr.Error()
	}
	a.submit(entry, true)
}

func (a *proxyAudit) recordSSEEvent(event string, data []byte, statusCode int) {
	if a == nil {
		return
	}

	if event == "endpoint" {
		if endpoint, err := url.Parse(string(data)); err == nil {
			if sessionID := mcpSessionID(nil, endpoint); sessionID != "" {
				a.entry.SessionID = sessionID
			}
		}
		return
	}

	var msg obotmcp.Message
	if decodeMCPHookMessage(data, &msg) != nil {
		return
	}
	requestID := obotmcp.MessageIDString(msg.ID)
	if hookResult, ok := a.takeStreamRequestHooks(requestID); ok {
		entry := a.entry
		entry.CreatedAt = time.Now()
		entry.RequestBody = hookResult.originalBody
		populateMCPMessageFields(&entry, hookResult.originalBody)
		entry.RequestHeaders = entry.ResponseHeaders
		entry.ResponseHeaders = nil
		entry.ResponseBody = nil
		entry.WebhookStatuses = append(entry.WebhookStatuses, auditHookStatuses(hookResult.statuses)...)
		if hookResult.mutated {
			entry.MutatedRequestBody = hookResult.mutatedBody
		}
		if hookResult.err != nil {
			entry.Error = hookResult.err.Error()
		}
		if requestID == "" || hookResult.err != nil {
			entry.ResponseStatus = statusCode
			entry.SetProcessingTime()
			a.submit(entry, true)
		} else {
			a.submit(entry, false)
		}
		return
	}
	if msg.Method == "" {
		a.recordResponse(data, statusCode, nil, "")
		return
	}

	entry := a.entry
	entry.CreatedAt = time.Now()
	entry.CallType = msg.Method
	entry.CallIdentifier = mcpCallIdentifier(msg)
	entry.RequestID = requestID
	entry.RequestBody = jsonBody(data)
	entry.RequestHeaders = entry.ResponseHeaders
	entry.ResponseHeaders = nil
	entry.ResponseBody = nil
	if requestID == "" {
		entry.ResponseStatus = statusCode
		entry.SetProcessingTime()
		a.submit(entry, true)
	} else {
		a.submit(entry, false)
	}
}

func (a *proxyAudit) newResponseEntry(requestID string) auditlogs.MCPAuditLog {
	if a == nil {
		return auditlogs.MCPAuditLog{}
	}

	return auditlogs.MCPAuditLog{
		Metadata:        maps.Clone(a.entry.Metadata),
		CreatedAt:       time.Now(),
		Subject:         a.entry.Subject,
		ClientName:      a.entry.ClientName,
		ClientVersion:   a.entry.ClientVersion,
		ClientIP:        a.entry.ClientIP,
		CallType:        "response",
		SessionID:       a.entry.SessionID,
		RequestID:       requestID,
		UserAgent:       a.entry.UserAgent,
		ResponseHeaders: bytes.Clone(a.entry.ResponseHeaders),
	}
}

func (a *proxyAudit) submit(entry auditlogs.MCPAuditLog, responseReceived bool) {
	if a == nil || a.collector == nil || entry.CallType == "" {
		return
	}
	a.collector.CollectMCPProxyAuditEntry(entry, responseReceived, a.proxyExchangeID)
}

func (b *auditResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.body.Write(p[:n])
	}
	if err != nil {
		b.finish(err)
	}
	return n, err
}

func (b *auditResponseBody) Close() error {
	b.finish(nil)
	return b.ReadCloser.Close()
}

func (b *auditResponseBody) finish(err error) {
	b.once.Do(func() {
		b.audit.recordHTTPResponse(b.body.Bytes(), b.statusCode, err)
	})
}

func newAuditSSEBody(body io.ReadCloser, auditor *proxyAudit, statusCode int) *auditSSEBody {
	return &auditSSEBody{ReadCloser: body, audit: auditor, statusCode: statusCode}
}

func (b *auditSSEBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.write(p[:n])
	}
	if err != nil {
		b.finish()
	}
	return n, err
}

func (b *auditSSEBody) Close() error {
	b.finish()
	return b.ReadCloser.Close()
}

func (b *auditSSEBody) finish() {
	b.once.Do(func() {
		if len(b.buffer) > 0 {
			b.consumeLine(strings.TrimSuffix(string(b.buffer), "\r"))
			b.buffer = nil
		}
		b.dispatch()
	})
}

func (b *auditSSEBody) write(data []byte) {
	b.buffer = append(b.buffer, data...)
	for {
		index := bytes.IndexByte(b.buffer, '\n')
		if index < 0 {
			return
		}
		line := strings.TrimSuffix(string(b.buffer[:index]), "\r")
		b.buffer = b.buffer[index+1:]
		b.consumeLine(line)
	}
}

func (b *auditSSEBody) consumeLine(line string) {
	if line == "" {
		b.dispatch()
		return
	}
	if value, ok := strings.CutPrefix(line, "event:"); ok {
		b.event = strings.TrimSpace(value)
		return
	}
	if value, ok := strings.CutPrefix(line, "data:"); ok {
		b.data = append(b.data, strings.TrimPrefix(value, " "))
	}
}

func (b *auditSSEBody) dispatch() {
	if len(b.data) > 0 {
		b.audit.recordSSEEvent(b.event, []byte(strings.Join(b.data, "\n")), b.statusCode)
	}
	b.event = ""
	b.data = nil
}

func mcpSessionID(headers http.Header, u *url.URL) string {
	if sessionID := headers.Get(mcpSessionHeader); sessionID != "" {
		return sessionID
	}
	if u == nil {
		return ""
	}
	if sessionID := u.Query().Get("session_id"); sessionID != "" {
		return sessionID
	}
	if sessionID := u.Query().Get("sessionid"); sessionID != "" {
		return sessionID
	}
	if sessionID := u.Query().Get("sessionId"); sessionID != "" {
		return sessionID
	}
	return u.Query().Get("id")
}

func sanitizedMCPHeaders(headers http.Header) http.Header {
	result := make(http.Header, len(headers))
	for key, values := range headers {
		switch http.CanonicalHeaderKey(key) {
		case "Authorization", "Cookie", "Set-Cookie", "X-Api-Key", "X-Auth-Token", "Proxy-Authorization":
			result[key] = []string{"[REDACTED]"}
		default:
			result[key] = slices.Clone(values)
		}
	}
	return result
}

func jsonBody(body []byte) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	if json.Valid(body) {
		return bytes.Clone(body)
	}
	encoded, err := json.Marshal(string(body))
	if err != nil {
		return json.RawMessage(fmt.Sprintf("%q", string(body)))
	}
	return encoded
}
