package mcpgateway

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/nanobot/pkg/mcp/auditlogs"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type recordingProxyAuditCollector struct {
	mu               sync.Mutex
	entries          []auditlogs.MCPAuditLog
	received         []bool
	proxyExchangeIDs []string
}

func (c *recordingProxyAuditCollector) CollectMCPAuditEntry(entry auditlogs.MCPAuditLog) {
	c.CollectMCPProxyAuditEntry(entry, false, "")
}

func (c *recordingProxyAuditCollector) CollectMCPProxyAuditEntry(entry auditlogs.MCPAuditLog, responseReceived bool, proxyExchangeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
	c.received = append(c.received, responseReceived)
	c.proxyExchangeIDs = append(c.proxyExchangeIDs, proxyExchangeID)
}

func (*recordingProxyAuditCollector) Close() {}

func newMCPProxyTestStorage(objects ...kclient.Object) storage.Client {
	return storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPClientSession{}).
		WithObjects(objects...).
		Build())
}

func TestMCPProxyAuditCollectsRequestAndResponseSeparately(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set(mcpSessionHeader, "session-1")
	req.Header.Set("Authorization", "Bearer secret-api-key-that-must-be-redacted")

	auditor, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	if len(collector.entries) != 1 || collector.received[0] {
		t.Fatalf("request was not collected immediately: count=%d received=%v", len(collector.entries), collector.received)
	}
	if collector.proxyExchangeIDs[0] == "" {
		t.Fatal("request audit entry did not receive a proxy exchange ID")
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":7,"result":{"content":[]}}`)),
	}
	consumeAuditResponse(t, auditor, resp)

	if len(collector.entries) != 2 {
		t.Fatalf("expected separate request and response entries, got %d", len(collector.entries))
	}
	if collector.proxyExchangeIDs[1] != collector.proxyExchangeIDs[0] {
		t.Fatalf("request and response exchange IDs differ: %#v", collector.proxyExchangeIDs)
	}
	requestEntry := collector.entries[0]
	if collector.received[0] {
		t.Fatal("request entry must not claim that the response was received")
	}
	if requestEntry.CallType != "tools/call" || requestEntry.CallIdentifier != "search" {
		t.Fatalf("unexpected request metadata: type=%q identifier=%q", requestEntry.CallType, requestEntry.CallIdentifier)
	}
	if requestEntry.RequestID != "7" || requestEntry.SessionID != "session-1" {
		t.Fatalf("unexpected request correlation: request=%q session=%q", requestEntry.RequestID, requestEntry.SessionID)
	}
	if string(requestEntry.RequestBody) != `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search"}}` {
		t.Fatalf("unexpected request body: %s", requestEntry.RequestBody)
	}
	if strings.Contains(string(requestEntry.RequestHeaders), "secret-api-key") || !strings.Contains(string(requestEntry.RequestHeaders), "[REDACTED]") {
		t.Fatalf("authorization header was not redacted: %s", requestEntry.RequestHeaders)
	}

	responseEntry := collector.entries[1]
	if !collector.received[1] || responseEntry.CallType != "response" {
		t.Fatalf("expected response-only entry, received=%v type=%q", collector.received[1], responseEntry.CallType)
	}
	if responseEntry.RequestID != "7" || responseEntry.SessionID != "session-1" {
		t.Fatalf("unexpected response correlation: request=%q session=%q", responseEntry.RequestID, responseEntry.SessionID)
	}
	if len(responseEntry.RequestBody) != 0 {
		t.Fatalf("response entry unexpectedly contains request data: %s", responseEntry.RequestBody)
	}
	if string(responseEntry.ResponseBody) != `{"jsonrpc":"2.0","id":7,"result":{"content":[]}}` {
		t.Fatalf("unexpected response body: %s", responseEntry.ResponseBody)
	}
}

func TestMCPProxyAuditPreservesLargeNumericRequestIDForResponseHooks(t *testing.T) {
	const requestID = "9007199254740993"

	collector := new(recordingProxyAuditCollector)
	req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":`+requestID+`,"method":"tools/call","params":{"name":"search"}}`))
	if err != nil {
		t.Fatal(err)
	}
	auditor, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	auditor.recordResponseHooks(requestID, hookResult{
		responseChanged: true,
		originalBody:    []byte(`{"jsonrpc":"2.0","id":` + requestID + `,"result":{"value":"original"}}`),
		statuses: []hookStatus{{
			typeName: "response",
			tool:     "policy/check",
			status:   "mutated",
		}},
	})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"jsonrpc":"2.0","id":` + requestID + `,"result":{"value":"mutated"}}`)),
	}
	consumeAuditResponse(t, auditor, resp)

	if len(collector.entries) != 2 {
		t.Fatalf("expected request and response audit entries, got %d", len(collector.entries))
	}
	if got := collector.entries[0].RequestID; got != requestID {
		t.Fatalf("request ID lost precision: got %q, want %q", got, requestID)
	}
	responseEntry := collector.entries[1]
	if responseEntry.RequestID != requestID {
		t.Fatalf("response ID lost precision: got %q, want %q", responseEntry.RequestID, requestID)
	}
	if len(responseEntry.WebhookStatuses) != 1 || responseEntry.WebhookStatuses[0].Status != "mutated" {
		t.Fatalf("large request ID did not match response hook state: %#v", responseEntry.WebhookStatuses)
	}
	if len(responseEntry.OriginalResponseBody) == 0 {
		t.Fatal("large request ID did not apply the original response body from hook state")
	}
}

func TestMCPProxyAuditPersistsAndLoadsClientInfoBySession(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	storageClient := newMCPProxyTestStorage()
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Desktop","version":"1.2.3"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	auditor, err := newProxyAudit(req, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			mcpSessionHeader: []string{"session-1"},
		},
		Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`)),
	}
	consumeAuditResponse(t, auditor, resp)

	if len(collector.entries) != 2 {
		t.Fatalf("expected initialize request and response, got %d", len(collector.entries))
	}
	if collector.entries[0].SessionID != "" || collector.entries[1].SessionID != "session-1" {
		t.Fatalf("expected persistence to join empty request session to assigned response session: request=%q response=%q", collector.entries[0].SessionID, collector.entries[1].SessionID)
	}
	for i, entry := range collector.entries {
		if entry.ClientName != "Claude Desktop" || entry.ClientVersion != "1.2.3" {
			t.Fatalf("entry %d lost same-exchange client info: name=%q version=%q", i, entry.ClientName, entry.ClientVersion)
		}
	}

	key := kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      mcpClientSessionName(metadata, "session-1"),
	}
	var session v1.MCPClientSession
	if err := storageClient.Get(t.Context(), key, &session); err != nil {
		t.Fatalf("client session was not persisted: %v", err)
	}
	if session.Spec.MCPServerID != "mcp-1" || session.Spec.UserID != "user-1" || session.Spec.ClientName != "Claude Desktop" || session.Spec.ClientVersion != "1.2.3" {
		t.Fatalf("unexpected persisted client session: %#v", session.Spec)
	}
	if session.Spec.Virtual {
		t.Fatal("upstream-provided session was marked virtual")
	}
	if session.Status.LastUsed.IsZero() {
		t.Fatal("new client session did not record its initial last-used time")
	}
	oldLastUsed := metav1.NewTime(time.Unix(1, 0))
	session.Status.LastUsed = oldLastUsed
	if err := storageClient.Status().Update(t.Context(), &session); err != nil {
		t.Fatal(err)
	}

	followup, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	followup.Header.Set(mcpSessionHeader, "session-1")
	followupAuditor, err := newProxyAudit(followup, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	if followupAuditor.entry.ClientName != "Claude Desktop" || followupAuditor.entry.ClientVersion != "1.2.3" {
		t.Fatalf("client identity was not loaded from storage: name=%q version=%q", followupAuditor.entry.ClientName, followupAuditor.entry.ClientVersion)
	}
	if got := followup.Header.Get(mcpSessionHeader); got != "session-1" {
		t.Fatalf("upstream-provided session header was changed: got %q", got)
	}
	if err := storageClient.Get(t.Context(), key, &session); err != nil {
		t.Fatal(err)
	}
	if !session.Status.LastUsed.After(oldLastUsed.Time) {
		t.Fatalf("last-used time was not updated: %v", session.Status.LastUsed)
	}
}

func TestMCPProxyAuditCreatesAndReusesVirtualSession(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	storageClient := newMCPProxyTestStorage()
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	initialize, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Stateless Client","version":"1.0.0"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	auditor, err := newProxyAudit(initialize, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`)),
	}
	consumeAuditResponse(t, auditor, resp)

	virtualSessionID := resp.Header.Get(mcpSessionHeader)
	if virtualSessionID == "" {
		t.Fatal("initialize response without an upstream session did not receive a virtual session ID")
	}
	if len(collector.entries) != 2 || collector.entries[1].SessionID != virtualSessionID {
		t.Fatalf("virtual session was not recorded on the initialize response: %#v", collector.entries)
	}

	key := kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      mcpClientSessionName(metadata, virtualSessionID),
	}
	var session v1.MCPClientSession
	if err := storageClient.Get(t.Context(), key, &session); err != nil {
		t.Fatalf("virtual client session was not persisted: %v", err)
	}
	if !session.Spec.Virtual {
		t.Fatalf("generated session was not marked virtual: %#v", session.Spec)
	}
	if session.Spec.ClientName != "Stateless Client" || session.Spec.ClientVersion != "1.0.0" {
		t.Fatalf("virtual session lost client identity: %#v", session.Spec)
	}

	followup, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	followup.Header.Set(mcpSessionHeader, virtualSessionID)
	followupAuditor, err := newProxyAudit(followup, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	if got := followup.Header.Get(mcpSessionHeader); got != "" {
		t.Fatalf("virtual session header was forwarded upstream: %q", got)
	}
	if followupAuditor.entry.SessionID != virtualSessionID {
		t.Fatalf("removing the upstream header also removed audit correlation: got %q, want %q", followupAuditor.entry.SessionID, virtualSessionID)
	}
	if followupAuditor.entry.ClientName != "Stateless Client" || followupAuditor.entry.ClientVersion != "1.0.0" {
		t.Fatalf("virtual session did not restore client identity: name=%q version=%q", followupAuditor.entry.ClientName, followupAuditor.entry.ClientVersion)
	}
}

func TestMCPProxyAuditWaitsForNotificationResponseStatus(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		t.Fatal(err)
	}
	auditor, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	if len(collector.entries) != 0 {
		t.Fatalf("notification was collected before its response status arrived: %d", len(collector.entries))
	}

	resp := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"X-Upstream": []string{"accepted"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}
	consumeAuditResponse(t, auditor, resp)

	if len(collector.entries) != 1 {
		t.Fatalf("expected one completed notification entry, got %d", len(collector.entries))
	}
	entry := collector.entries[0]
	if !collector.received[0] || entry.ResponseStatus != http.StatusAccepted {
		t.Fatalf("notification response status was not captured: received=%v status=%d", collector.received[0], entry.ResponseStatus)
	}
	if !strings.Contains(string(entry.ResponseHeaders), "X-Upstream") {
		t.Fatalf("notification response headers were not captured: %s", entry.ResponseHeaders)
	}
}

func TestMCPProxyAuditLogsSessionDeleteAsNotification(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	req, err := http.NewRequest(http.MethodDelete, "http://obot.example/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(mcpSessionHeader, "session-1")
	auditor, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	if len(collector.entries) != 0 {
		t.Fatalf("session delete was collected before its response status arrived: %d", len(collector.entries))
	}

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{"X-Upstream": []string{"deleted"}},
		Body:       http.NoBody,
	}
	consumeAuditResponse(t, auditor, resp)

	if len(collector.entries) != 1 {
		t.Fatalf("expected one session delete notification, got %d", len(collector.entries))
	}
	entry := collector.entries[0]
	if !collector.received[0] || entry.ResponseStatus != http.StatusNoContent {
		t.Fatalf("session delete status was not captured: received=%v status=%d", collector.received[0], entry.ResponseStatus)
	}
	if entry.CallType != "session/delete" || entry.RequestID != "" {
		t.Fatalf("session delete was not recorded as a notification: method=%q requestID=%q", entry.CallType, entry.RequestID)
	}
	if string(entry.RequestBody) != mcpSessionDeleteRequest {
		t.Fatalf("unexpected session delete notification body: %s", entry.RequestBody)
	}
	if entry.SessionID != "session-1" {
		t.Fatalf("session delete lost its session ID: %q", entry.SessionID)
	}
}

func TestMCPProxyAuditCorrelatesLegacySSEThroughPersistenceFields(t *testing.T) {
	collector := new(recordingProxyAuditCollector)
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	post, err := http.NewRequest(http.MethodPost, "http://obot.example/messages?sessionid=legacy-session", strings.NewReader(`{"jsonrpc":"2.0","id":"request-1","method":"resources/read","params":{"uri":"file:///readme"}}`))
	if err != nil {
		t.Fatal(err)
	}
	storageClient := newMCPProxyTestStorage()
	postAuditor, err := newProxyAudit(post, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	postAuditor.recordRequest()
	accepted := &http.Response{StatusCode: http.StatusAccepted, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
	consumeAuditResponse(t, postAuditor, accepted)

	get, err := http.NewRequest(http.MethodGet, "http://obot.example/sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	getAuditor, err := newProxyAudit(get, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	getAuditor.recordRequest()
	sse := "event: endpoint\n" +
		"data: /messages?sessionid=legacy-session\n\n" +
		"event: message\n" +
		"data: {\"jsonrpc\":\"2.0\",\"id\":\"request-1\",\"result\":{\"contents\":[]}}\n\n" +
		"event: message\n" +
		"data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/resources/updated\",\"params\":{\"uri\":\"file:///readme\"}}\n\n"
	stream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body: &chunkReadCloser{chunks: []string{
			sse[:17], sse[17:53], sse[53:81], sse[81:],
		}},
	}
	consumeAuditResponse(t, getAuditor, stream)

	if len(collector.entries) != 3 {
		t.Fatalf("expected request, streamed response, and notification, got %d", len(collector.entries))
	}
	requestEntry := collector.entries[0]
	if collector.received[0] || requestEntry.CallType != "resources/read" || requestEntry.SessionID != "legacy-session" || requestEntry.RequestID != "request-1" {
		t.Fatalf("unexpected request entry: %#v received=%v", requestEntry, collector.received[0])
	}
	responseEntry := collector.entries[1]
	if !collector.received[1] || responseEntry.CallType != "response" || responseEntry.SessionID != "legacy-session" || responseEntry.RequestID != "request-1" {
		t.Fatalf("unexpected response entry: %#v received=%v", responseEntry, collector.received[1])
	}
	if len(responseEntry.RequestBody) != 0 || string(responseEntry.ResponseBody) != `{"jsonrpc":"2.0","id":"request-1","result":{"contents":[]}}` {
		t.Fatalf("response was not response-only: request=%s response=%s", responseEntry.RequestBody, responseEntry.ResponseBody)
	}
	notificationEntry := collector.entries[2]
	if notificationEntry.CallType != "notifications/resources/updated" || !collector.received[2] || notificationEntry.ResponseStatus != http.StatusOK {
		t.Fatalf("server notification was not audited with its response status: type=%q received=%v status=%d", notificationEntry.CallType, collector.received[2], notificationEntry.ResponseStatus)
	}
}

func consumeAuditResponse(t *testing.T, auditor *proxyAudit, resp *http.Response) {
	t.Helper()
	if err := auditor.wrapResponse(resp); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

type chunkReadCloser struct {
	chunks []string
}

func (r *chunkReadCloser) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]
	return copy(p, chunk), nil
}

func (*chunkReadCloser) Close() error { return nil }
