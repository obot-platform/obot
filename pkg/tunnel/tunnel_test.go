package tunnel

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestConnectURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "http://obot.example", want: "ws://obot.example/tunnel/connect"},
		{base: "https://obot.example/api/", want: "wss://obot.example/tunnel/connect"},
		{base: "https://obot.example/prefix/api", want: "wss://obot.example/prefix/tunnel/connect"},
	}
	for _, test := range tests {
		got, err := ConnectURL(test.base)
		if err != nil {
			t.Fatalf("ConnectURL(%q) error = %v", test.base, err)
		}
		if got != test.want {
			t.Fatalf("ConnectURL(%q) = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestTunnelClosesWebSocketNormallyOnContextCancellation(t *testing.T) {
	accepted := make(chan struct{})
	serverError := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		}).Upgrade(w, r, nil)
		if err != nil {
			serverError <- fmt.Errorf("upgrading websocket: %w", err)
			return
		}
		defer connection.Close()
		close(accepted)

		_, _, err = connection.ReadMessage()
		serverError <- err
	}))
	defer server.Close()

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	closeAcknowledged := make(chan int, 1)
	connection.SetCloseHandler(func(code int, _ string) error {
		closeAcknowledged <- code
		return nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	clientError := make(chan error, 1)
	go func() {
		clientError <- serveConnection(ctx, connection, "test")
	}()

	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket connection")
	}
	cancel()

	select {
	case err := <-serverError:
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			t.Fatalf("server read error = %v, want normal websocket closure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket close frame")
	}

	select {
	case err := <-clientError:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("serveConnection error = %v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for tunnel shutdown")
	}

	select {
	case code := <-closeAcknowledged:
		if code != websocket.CloseNormalClosure {
			t.Fatalf("server close acknowledgement = %d, want %d", code, websocket.CloseNormalClosure)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket close acknowledgement")
	}
}

func TestTunnelAcknowledgesServerWebSocketClose(t *testing.T) {
	accepted := make(chan struct{})
	closeAcknowledged := make(chan int, 1)
	serverError := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		}).Upgrade(w, r, nil)
		if err != nil {
			serverError <- fmt.Errorf("upgrading websocket: %w", err)
			return
		}
		defer connection.Close()
		connection.SetCloseHandler(func(code int, _ string) error {
			closeAcknowledged <- code
			return nil
		})
		close(accepted)

		if err := connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		); err != nil {
			serverError <- fmt.Errorf("sending websocket close: %w", err)
			return
		}
		_, _, err = connection.ReadMessage()
		serverError <- err
	}))
	defer server.Close()

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket connection")
	}

	clientError := make(chan error, 1)
	go func() {
		clientError <- serveConnection(t.Context(), connection, "test")
	}()

	select {
	case code := <-closeAcknowledged:
		if code != websocket.CloseNormalClosure {
			t.Fatalf("client close acknowledgement = %d, want %d", code, websocket.CloseNormalClosure)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client close acknowledgement")
	}

	select {
	case err := <-serverError:
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			t.Fatalf("server read error = %v, want normal websocket closure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server websocket shutdown")
	}

	select {
	case err := <-clientError:
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			t.Fatalf("serveConnection error = %v, want normal websocket closure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for tunnel shutdown")
	}
}

func TestTunnelForwardsStreamingHTTP(t *testing.T) {
	requestBody := bytes.Repeat([]byte("request-data-"), 10_000)
	responseBody := bytes.Repeat([]byte("response-data-"), 10_000)
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
			t.Errorf("target request = %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query()["base"]; len(got) != 1 || got[0] != "one" {
			t.Errorf("base query = %#v", got)
		}
		if got := r.URL.Query().Get("extra"); got != "two" {
			t.Errorf("extra query = %q", got)
		}
		if got := r.Header.Get("X-Tunnel-Test"); got != "present" {
			t.Errorf("X-Tunnel-Test = %q", got)
		}
		if got := r.Header.Get(bridgeAuthorizationHeader); got != "" {
			t.Errorf("internal bridge authorization header was forwarded: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading target body: %v", err)
		}
		if !bytes.Equal(body, requestBody) {
			t.Errorf("target body length = %d, want %d", len(body), len(requestBody))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "session-123")
		w.WriteHeader(http.StatusCreated)
		for offset := 0; offset < len(responseBody); offset += 4096 {
			end := min(offset+4096, len(responseBody))
			_, _ = w.Write(responseBody[offset:end])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	bridgeURL, err := manager.BridgeURL("office", target.URL+"/mcp?base=one")
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, bridgeURL+"?extra=two", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Tunnel-Test", "present")
	response, err := bridgeClient.Do(request)
	if err != nil {
		t.Fatalf("bridge request error = %v", err)
	}
	defer response.Body.Close()
	gotBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading bridge response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	if response.Header.Get("Mcp-Session-Id") != "session-123" {
		t.Fatalf("Mcp-Session-Id = %q", response.Header.Get("Mcp-Session-Id"))
	}
	if !bytes.Equal(gotBody, responseBody) {
		t.Fatalf("response body length = %d, want %d", len(gotBody), len(responseBody))
	}
}

func TestTunnelMultiplexesConcurrentRequests(t *testing.T) {
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		time.Sleep(time.Duration(len(id)%3) * 5 * time.Millisecond)
		_, _ = io.WriteString(w, id)
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Go(func() {
			id := fmt.Sprintf("request-%d", i)
			bridgeURL, err := manager.BridgeURL("office", target.URL+"/?id="+url.QueryEscape(id))
			if err != nil {
				errs <- err
				return
			}
			response, err := bridgeClient.Get(bridgeURL)
			if err != nil {
				errs <- err
				return
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				errs <- readErr
			} else if string(body) != id {
				errs <- fmt.Errorf("body = %q, want %q", body, id)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestTunnelLogsCorrelatedRequestAndResponse(t *testing.T) {
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	testLogger, hook := logrustest.NewNullLogger()
	testLogger.SetLevel(logrus.InfoLevel)
	previousTunnelLog := tunnelLog
	tunnelLog = logger.NewWithLogger(testLogger, nil)
	defer func() {
		tunnelLog = previousTunnelLog
	}()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	bridgeURL, err := manager.BridgeURL("office", target.URL+"/mcp?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	response, err := bridgeClient.Get(bridgeURL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()

	var requestEntry, responseEntry *logrus.Entry
	for _, entry := range hook.AllEntries() {
		switch entry.Message {
		case "Tunnel request received":
			requestEntry = entry
		case "Tunnel response received":
			responseEntry = entry
		}
	}
	if requestEntry == nil || responseEntry == nil {
		t.Fatalf("request and response log entries = %#v, %#v", requestEntry, responseEntry)
	}
	if requestEntry.Data["request_id"] != responseEntry.Data["request_id"] {
		t.Fatalf("request IDs = %v and %v", requestEntry.Data["request_id"], responseEntry.Data["request_id"])
	}
	if requestEntry.Data["tunnel"] != "office" || requestEntry.Data["method"] != http.MethodGet {
		t.Fatalf("request log fields = %#v", requestEntry.Data)
	}
	if requestEntry.Data["url"] != target.URL+"/mcp" || requestEntry.Data["has_query"] != true {
		t.Fatalf("request URL fields = %#v", requestEntry.Data)
	}
	if responseEntry.Data["status"] != http.StatusAccepted || responseEntry.Data["response_content_length"] != int64(2) {
		t.Fatalf("response log fields = %#v", responseEntry.Data)
	}
	if _, ok := responseEntry.Data["duration"]; !ok {
		t.Fatalf("response log has no duration: %#v", responseEntry.Data)
	}
}

func TestTunnelRewritesRedirectAndLegacySSEEndpoint(t *testing.T) {
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
		case "/cross-origin-redirect":
			w.Header().Set("Location", "https://attacker.example/collect")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "/final":
			_, _ = io.WriteString(w, "redirected")
		case "/sse":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: endpoint\ndata: /messages?session=abc\n\n")
		case "/messages":
			if r.URL.Query().Get("session") != "abc" {
				t.Errorf("session query = %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, "posted")
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	redirectBridge, _ := manager.BridgeURL("office", target.URL+"/redirect")
	response, err := bridgeClient.Get(redirectBridge)
	if err != nil {
		t.Fatalf("redirect request error = %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "redirected" {
		t.Fatalf("redirect body = %q", body)
	}
	if !strings.HasPrefix(response.Request.URL.String(), manager.bridgeBaseURL+bridgePathPrefix) {
		t.Fatalf("redirect escaped bridge: %s", response.Request.URL)
	}

	crossOriginBridge, _ := manager.BridgeURL("office", target.URL+"/cross-origin-redirect")
	crossOriginResponse, err := bridgeClient.Get(crossOriginBridge)
	if err != nil {
		t.Fatalf("cross-origin redirect request error = %v", err)
	}
	crossOriginResponse.Body.Close()
	if location := crossOriginResponse.Header.Get("Location"); location != "" {
		t.Fatalf("cross-origin Location was forwarded: %q", location)
	}

	sseBridge, _ := manager.BridgeURL("office", target.URL+"/sse")
	sseResponse, err := bridgeClient.Get(sseBridge)
	if err != nil {
		t.Fatalf("SSE request error = %v", err)
	}
	sseBody, _ := io.ReadAll(sseResponse.Body)
	sseResponse.Body.Close()
	dataLine := ""
	for line := range strings.SplitSeq(string(sseBody), "\n") {
		if value, ok := strings.CutPrefix(line, "data: "); ok {
			dataLine = value
		}
	}
	if !strings.HasPrefix(dataLine, manager.bridgeBaseURL+bridgePathPrefix) {
		t.Fatalf("rewritten endpoint = %q; SSE body = %q", dataLine, sseBody)
	}
	postResponse, err := bridgeClient.Post(dataLine, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("posting to rewritten endpoint: %v", err)
	}
	postBody, _ := io.ReadAll(postResponse.Body)
	postResponse.Body.Close()
	if string(postBody) != "posted" {
		t.Fatalf("endpoint response = %q", postBody)
	}
}

func TestTunnelAcceptsMultipleConnections(t *testing.T) {
	manager, _, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()
	serverURL := manager.bridgeBaseURL
	connection, err := Dial(t.Context(), serverURL, testTunnelToken("office"))
	if err != nil {
		t.Fatalf("second tunnel connection failed: %v", err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- serveConnection(t.Context(), connection, "office") }()
	waitForTestCondition(t, 2*time.Second, func() bool {
		return localTunnelConnectionCount(manager, "office") == 2
	}, "both tunnel connections were not registered")

	_ = connection.Close()
	select {
	case <-serveErrors:
	case <-time.After(time.Second):
		t.Fatal("second tunnel client did not stop")
	}
}

func TestManagerDisconnectsAllTunnelConnections(t *testing.T) {
	manager, _, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()
	connection, err := Dial(t.Context(), manager.bridgeBaseURL, testTunnelToken("office"))
	if err != nil {
		t.Fatal(err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- serveConnection(t.Context(), connection, "office") }()
	waitForTestCondition(t, 2*time.Second, func() bool {
		return localTunnelConnectionCount(manager, "office") == 2
	}, "both tunnel connections were not registered")

	credentialID, err := CredentialID(testTunnelCredential("office"))
	if err != nil {
		t.Fatal(err)
	}
	manager.DisconnectCredential("office", credentialID)

	waitForTestCondition(t, time.Second, func() bool {
		return localTunnelConnectionCount(manager, "office") == 0
	}, "tunnel connections remained registered")
	select {
	case <-serveErrors:
	case <-time.After(time.Second):
		t.Fatal("second tunnel client remained connected")
	}
}

func TestTunnelRoutesRequestsByName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	manager, server := newManagerTestServer(ctx, t)
	errorsByClient := make(chan error, 2)
	sessionKeys := make(map[string]string, 2)

	for name, responseBody := range map[string]string{"office": "from-office", "lab": "from-lab"} {
		token := testTunnelToken(name)
		matcher, ok := newCredentialMatcher(token)
		if !ok {
			t.Fatalf("test tunnel token for %q is invalid", name)
		}
		sessionKeys[name] = tunnelSessionKey(name, matcher.credentialID)
		connection, _, err := dial(ctx, server.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": []string{"text/plain"}},
				Body:          io.NopCloser(strings.NewReader(responseBody)),
				ContentLength: int64(len(responseBody)),
			}, nil
		})}
		go func() { errorsByClient <- serveConnectionWithClient(ctx, connection, name, client) }()
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		connected := manager.remoteDialer.HasSession(sessionKeys["office"]) &&
			manager.remoteDialer.HasSession(sessionKeys["lab"])
		if connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("named tunnel sessions did not become ready")
		}
		time.Sleep(time.Millisecond)
	}

	for name, want := range map[string]string{"office": "from-office", "lab": "from-lab"} {
		bridgeURL, err := manager.BridgeURL(name, "http://target.invalid/mcp")
		if err != nil {
			t.Fatal(err)
		}
		response, err := newBridgeClient(manager).Get(bridgeURL)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(body) != want {
			t.Fatalf("tunnel %q response = %q, want %q", name, body, want)
		}
	}

	cancel()
	manager.Close()
	server.Close()
	for range 2 {
		select {
		case <-errorsByClient:
		case <-time.After(time.Second):
			t.Fatal("named tunnel client did not stop")
		}
	}
}

func TestHTTPClientRoutesRequestsAndRedirectsThroughBridge(t *testing.T) {
	const (
		tunnelName    = "office"
		initialTarget = "https://oauth.internal.test/start"
		finalTarget   = "https://oauth.internal.test/finish"
	)

	var (
		manager     *Manager
		seenTargets []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		name, value := manager.BridgeAuthorization()
		if request.Header.Get(name) != value {
			t.Errorf("bridge authorization header = %q, want %q", request.Header.Get(name), value)
		}
		if request.Header.Get("X-Test") != "preserved" {
			t.Errorf("original request header was not preserved: %#v", request.Header)
		}
		if request.Header.Get("X-Injected") != "configured" {
			t.Errorf("injected header = %q, want configured", request.Header.Get("X-Injected"))
		}
		if request.Header.Get("X-Override") != "configured" {
			t.Errorf("overridden header = %q, want configured", request.Header.Get("X-Override"))
		}
		if !strings.HasPrefix(request.URL.Path, bridgePathPrefix) {
			t.Errorf("request path = %q, want bridge path", request.URL.Path)
			http.Error(w, "not a bridge request", http.StatusBadRequest)
			return
		}

		payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(request.URL.Path, bridgePathPrefix))
		if err != nil {
			t.Errorf("decode bridge target: %v", err)
			http.Error(w, "invalid bridge target", http.StatusBadRequest)
			return
		}
		var target bridgeTarget
		if err := json.Unmarshal(payload, &target); err != nil {
			t.Errorf("unmarshal bridge target: %v", err)
			http.Error(w, "invalid bridge target", http.StatusBadRequest)
			return
		}
		if target.TunnelName != tunnelName {
			t.Errorf("tunnel name = %q, want %q", target.TunnelName, tunnelName)
		}
		seenTargets = append(seenTargets, target.URL)

		switch target.URL {
		case initialTarget:
			redirectURL, err := manager.BridgeURL(tunnelName, finalTarget)
			if err != nil {
				t.Errorf("build redirect bridge URL: %v", err)
				http.Error(w, "failed to redirect", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, request, redirectURL, http.StatusFound)
		case finalTarget:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected target", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	var err error
	manager, err = NewManager(t.Context(), server.URL, allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	httpClient, err := manager.HTTPClient(tunnelName, http.Header{
		"X-Injected": {"configured"},
		"X-Override": {"configured"},
	}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if httpClient.Timeout != 3*time.Second {
		t.Fatalf("client timeout = %v, want %v", httpClient.Timeout, 3*time.Second)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, initialTarget, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test", "preserved")
	request.Header.Set("X-Override", "request")
	response, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
	if !slices.Equal(seenTargets, []string{initialTarget, finalTarget}) {
		t.Fatalf("bridge targets = %#v, want initial and final targets", seenTargets)
	}
	if request.URL.String() != initialTarget {
		t.Fatalf("original request URL mutated to %q", request.URL)
	}
	name, _ := manager.BridgeAuthorization()
	if request.Header.Get(name) != "" {
		t.Fatal("bridge authorization was added to original request")
	}
	if request.Header.Get("X-Injected") != "" {
		t.Fatal("configured header was added to original request")
	}
	if request.Header.Get("X-Override") != "request" {
		t.Fatalf("original request header = %q, want request", request.Header.Get("X-Override"))
	}
}

func TestHTTPClientRejectsInvalidConfiguration(t *testing.T) {
	var manager *Manager
	if _, err := manager.HTTPClient("office", nil, time.Second); err == nil {
		t.Fatal("nil manager returned no error")
	}

	validManager, err := NewManager(t.Context(), "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer validManager.Close()
	if _, err := validManager.HTTPClient("Office", nil, time.Second); err == nil {
		t.Fatal("invalid tunnel name returned no error")
	}
}

func TestTunnelRoutesThroughPeerReplicaAndDisconnectsRemotely(t *testing.T) {
	const (
		tunnelName = "office"
		peerToken  = "shared-peer-token"
	)
	configuredTunnel := testConfiguredTunnel(tunnelName)
	reader := &staticTunnelTestReader{tunnels: map[string]v1.MCPTunnel{
		tunnelName: configuredTunnel,
	}}

	ctx := t.Context()
	first, firstServer := newManagerTestServer(ctx, t, reader)
	defer firstServer.Close()
	defer first.Close()
	second, secondServer := newManagerTestServer(ctx, t, reader)
	defer secondServer.Close()
	defer second.Close()

	configureTestPeer(second, "replica-b", peerToken)
	second.ReconcilePeers(map[string]string{
		"replica-a": strings.Replace(firstServer.URL, "http://", "ws://", 1) + PeerConnectPath,
	})
	configureTestPeer(first, "replica-a", peerToken)
	first.ReconcilePeers(map[string]string{
		"replica-b": strings.Replace(secondServer.URL, "http://", "ws://", 1) + PeerConnectPath,
	})

	connection, _, err := dial(ctx, firstServer.URL, testTunnelToken(tunnelName))
	if err != nil {
		t.Fatal(err)
	}
	serveErrors := make(chan error, 2)
	go func() { serveErrors <- serveConnection(ctx, connection, tunnelName) }()

	key, err := configuredTunnelSessionKey(&configuredTunnel)
	if err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 10*time.Second, func() bool {
		return second.remoteDialer.HasSession(key)
	}, "second replica did not learn the tunnel session")
	secondConnection, _, err := dial(ctx, secondServer.URL, testTunnelToken(tunnelName))
	if err != nil {
		t.Fatalf("second replica rejected an additional tunnel session: %v", err)
	}
	go func() { serveErrors <- serveConnection(ctx, secondConnection, tunnelName) }()
	waitForTestCondition(t, 5*time.Second, func() bool {
		return localTunnelConnectionCount(first, tunnelName) == 1 &&
			localTunnelConnectionCount(second, tunnelName) == 1
	}, "tunnel sessions were not connected to both replicas")

	connections, err := second.ConnectionsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].Name != tunnelName {
		t.Fatalf("second replica connections = %#v", connections)
	}

	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "through-peer")
	}))
	defer target.Close()
	bridgeURL, err := second.BridgeURL(tunnelName, target.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	response, err := newBridgeClient(second).Get(bridgeURL)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != "through-peer" {
		t.Fatalf("peer response = %d %q", response.StatusCode, body)
	}

	second.DisconnectCredential(tunnelName, configuredTunnel.Spec.CredentialID)
	waitForTestCondition(t, 5*time.Second, func() bool {
		return !first.remoteDialer.HasSession(key) && !second.remoteDialer.HasSession(key)
	}, "remote disconnect did not remove the tunnel session")
	for range 2 {
		select {
		case <-serveErrors:
		case <-time.After(5 * time.Second):
			t.Fatal("remote disconnect did not stop every tunnel client session")
		}
	}
}

func TestBridgeEnforcesCurrentAllowedURLs(t *testing.T) {
	const tunnelName = "mt1office"
	configuredTunnel := testConfiguredTunnel(tunnelName)
	configuredTunnel.Spec.Manifest.AllowedURLs = []string{
		"http://example.test/mcp?allowed=true",
		"http://example.test/mcp?b=2&a=1",
	}
	reader := &staticTunnelTestReader{
		tunnels: map[string]v1.MCPTunnel{tunnelName: configuredTunnel},
	}

	ctx := t.Context()
	manager, server := newManagerTestServer(ctx, t, reader)
	defer manager.Close()
	defer server.Close()

	bridgeURL, err := manager.BridgeURL(tunnelName, "http://example.test/mcp")
	if err != nil {
		t.Fatal(err)
	}

	response, err := newBridgeClient(manager).Get(bridgeURL + "?allowed=true")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("allowed target status = %d, want %d for disconnected tunnel", response.StatusCode, http.StatusServiceUnavailable)
	}

	orderedBridgeURL, err := manager.BridgeURL(tunnelName, "http://example.test/mcp?b=2&a=1")
	if err != nil {
		t.Fatal(err)
	}
	response, err = newBridgeClient(manager).Get(orderedBridgeURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("exact query target status = %d, want %d for disconnected tunnel", response.StatusCode, http.StatusServiceUnavailable)
	}

	response, err = newBridgeClient(manager).Get(bridgeURL + "?allowed=false")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed target status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestBridgeRequiresCapability(t *testing.T) {
	ctx := t.Context()
	manager, server := newManagerTestServer(ctx, t)
	defer manager.Close()
	defer server.Close()

	bridgeURL, err := manager.BridgeURL("office", "http://example.test/mcp")
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(bridgeURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, bridgeURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := manager.BridgeAuthorization()
	request.Header.Set(name, "wrong")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong capability status = %d, want 404", response.StatusCode)
	}

	response, err = newBridgeClient(manager).Get(bridgeURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("authorized status = %d, want 503 for disconnected tunnel", response.StatusCode)
	}
}

func TestManagerServeConnectRechecksCredential(t *testing.T) {
	const tunnelName = "mt1office"
	_, currentCredential, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	staleToken, _, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	configuredTunnel := v1.MCPTunnel{}
	configuredTunnel.Name = tunnelName
	configuredTunnel.Spec.Credential = currentCredential
	reader := &staticTunnelTestReader{
		tunnels: map[string]v1.MCPTunnel{tunnelName: configuredTunnel},
	}

	manager, err := NewManager(t.Context(), "http://127.0.0.1:8080", reader, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	request := httptest.NewRequest(http.MethodGet, "/tunnel/connect", nil)
	request.Header.Set("Authorization", "Bearer "+staleToken)
	recorder := httptest.NewRecorder()
	manager.ServeConnect(recorder, request, tunnelName)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if localTunnelConnectionCount(manager, tunnelName) != 0 {
		t.Fatal("failed credential left a tracked tunnel connection")
	}
}

func TestBridgeURLRejectsInvalidInput(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager, err := NewManager(ctx, "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	for _, test := range []struct {
		name       string
		tunnelName string
		targetURL  string
	}{
		{name: "invalid tunnel name", tunnelName: "Office", targetURL: "https://example.test/mcp"},
		{name: "special tunnel scheme", tunnelName: "office", targetURL: "https+tunnel://office@example.test/mcp"},
		{name: "unsupported target scheme", tunnelName: "office", targetURL: "ftp://example.test/mcp"},
		{name: "missing target hostname", tunnelName: "office", targetURL: "https:///mcp"},
		{name: "target user information", tunnelName: "office", targetURL: "https://user@example.test/mcp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := manager.BridgeURL(test.tunnelName, test.targetURL); err == nil {
				t.Fatal("BridgeURL() returned no error")
			}
		})
	}
}

func TestBridgeCapabilityAuthenticator(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager, err := NewManager(ctx, "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	request := httptest.NewRequest(http.MethodPost, "/tunnel/bridge/target", nil)
	request.Header.Set("Authorization", "Bearer remote-mcp-token")
	name, value := manager.BridgeAuthorization()
	request.Header.Set(name, value)

	response, ok, err := manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatalf("AuthenticateRequest() = (%#v, %v), want bridge principal", response, ok)
	}
	if response.User.GetName() != bridgePrincipalName ||
		!slices.Contains(response.User.GetGroups(), apitypes.GroupTunnelBridge) {
		t.Fatalf("authenticated user = %#v", response.User)
	}

	request.Header.Set(name, "wrong")
	response, ok, err = manager.AuthenticateRequest(request)
	if err != nil || ok || response != nil {
		t.Fatalf("wrong capability AuthenticateRequest() = (%#v, %v, %v)", response, ok, err)
	}
}

func TestPeerAuthenticator(t *testing.T) {
	manager, err := NewManager(t.Context(), "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.Token = "shared-peer-token"

	request := httptest.NewRequest(http.MethodGet, PeerConnectPath, nil)
	request.Header.Set(remotedialer.ID, "10.0.0.2")
	request.Header.Set(remotedialer.Token, manager.Token)
	response, ok, err := manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil ||
		response.User.GetUID() != "10.0.0.2" ||
		!slices.Contains(response.User.GetGroups(), apitypes.GroupTunnelPeer) {
		t.Fatalf("AuthenticateRequest() = (%#v, %v), want peer principal", response, ok)
	}

	request.Header.Set(remotedialer.Token, "wrong")
	response, ok, err = manager.AuthenticateRequest(request)
	if err != nil || ok || response != nil {
		t.Fatalf("wrong peer token AuthenticateRequest() = (%#v, %v, %v)", response, ok, err)
	}
}

func TestReconcilePeersClosesRemovedIncomingConnection(t *testing.T) {
	manager, err := NewManager(t.Context(), "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	serverConnection, peerConnection := net.Pipe()
	defer peerConnection.Close()
	manager.peerMu.Lock()
	manager.peers["departed-peer"] = struct{}{}
	manager.peerConnections[serverConnection] = "departed-peer"
	manager.peerMu.Unlock()

	manager.ReconcilePeers(map[string]string{})
	_ = peerConnection.SetReadDeadline(time.Now().Add(time.Second))
	var value [1]byte
	if _, err := peerConnection.Read(value[:]); err == nil {
		t.Fatal("removed incoming peer connection remained open")
	}
	manager.peerMu.Lock()
	defer manager.peerMu.Unlock()
	if len(manager.peers) != 0 || len(manager.peerConnections) != 0 {
		t.Fatalf("removed peer state remains: peers=%v connections=%v", manager.peers, manager.peerConnections)
	}
}

func TestBridgeURLIsStableAcrossManagerRestarts(t *testing.T) {
	ctx := t.Context()

	first, err := NewManager(ctx, "http://127.0.0.1:8080/", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewManager(ctx, "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	target := "https://example.test/mcp?one=two"
	firstURL, err := first.BridgeURL("office", target)
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := second.BridgeURL("office", target)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(bridgeTarget{TunnelName: "office", URL: target})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	want := "http://127.0.0.1:8080" + bridgePathPrefix + encoded
	if firstURL != want || secondURL != want {
		t.Fatalf("bridge URLs = %q and %q, want %q", firstURL, secondURL, want)
	}
	if first.BridgeHost() != "127.0.0.1:8080" || second.BridgeHost() != "127.0.0.1:8080" {
		t.Fatalf("bridge hosts = %q and %q", first.BridgeHost(), second.BridgeHost())
	}
}

func TestManagerConnectionsSnapshot(t *testing.T) {
	manager, _, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()
	manager.tunnels = &staticTunnelTestReader{tunnels: map[string]v1.MCPTunnel{
		"disconnected": testConfiguredTunnel("disconnected"),
		"office":       testConfiguredTunnel("office"),
	}}
	connections, err := manager.ConnectionsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 {
		t.Fatalf("Connections length = %d, want 1", len(connections))
	}
	if connections[0].Name != "office" {
		t.Fatalf("Connections[0].Name = %q, want office", connections[0].Name)
	}
	localConnections := manager.Connections()
	if len(localConnections) != 1 || localConnections[0].Name != "office" {
		t.Fatalf("process-local Connections() = %#v, want office", localConnections)
	}
}

func TestNilManagerConnectionsIsEmpty(t *testing.T) {
	var manager *Manager
	connections, err := manager.ConnectionsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if connections == nil || len(connections) != 0 {
		t.Fatalf("Connections = %#v, want an empty non-nil slice", connections)
	}
}

func TestRewriteTargetURLRejectsCrossOrigin(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager, err := NewManager(ctx, "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	base, err := url.Parse("https://private.example/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.rewriteTargetURL("office", base, "https://attacker.example/collect"); err == nil {
		t.Fatal("cross-origin target was rewritten through the bridge")
	}
	if _, err := manager.rewriteTargetURL("office", base, "/messages"); err != nil {
		t.Fatalf("same-origin target was rejected: %v", err)
	}
}

func TestClientForwarderRejectsMissingTarget(t *testing.T) {
	clientConnection, handlerConnection := net.Pipe()
	defer clientConnection.Close()
	forwarder := &clientForwarder{client: newForwardHTTPClient(), name: "office"}
	done := make(chan error, 1)
	go func() { done <- forwarder.serve(t.Context(), handlerConnection) }()

	request, err := http.NewRequest(http.MethodGet, "http://obot-tunnel-forward/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Write(clientConnection); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(clientConnection), request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadGateway || response.Header.Get(forwardErrorHeader) == "" {
		t.Fatalf("response = %d, headers %#v", response.StatusCode, response.Header)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newConnectedTestTunnel(t *testing.T, name string) (*Manager, *http.Client, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	manager, server := newManagerTestServer(ctx, t)
	token := testTunnelToken(name)
	matcher, ok := newCredentialMatcher(token)
	if !ok {
		server.Close()
		manager.Close()
		cancel()
		t.Fatalf("test tunnel token is invalid")
	}
	sessionKey := tunnelSessionKey(name, matcher.credentialID)
	connection, _, err := dial(ctx, server.URL, token)
	if err != nil {
		server.Close()
		manager.Close()
		cancel()
		t.Fatalf("Dial() error = %v", err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- serveConnection(ctx, connection, name) }()

	deadline := time.Now().Add(2 * time.Second)
	for !manager.remoteDialer.HasSession(sessionKey) {
		if time.Now().After(deadline) {
			t.Fatal("tunnel session did not become ready")
		}
		time.Sleep(time.Millisecond)
	}

	cleanup := func() {
		cancel()
		manager.Close()
		_ = connection.Close()
		server.Close()
		select {
		case <-serveErrors:
		case <-time.After(time.Second):
			t.Errorf("tunnel client did not stop")
		}
	}
	return manager, newBridgeClient(manager), cleanup
}

func newManagerTestServer(ctx context.Context, t *testing.T, readers ...kclient.Reader) (*Manager, *httptest.Server) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reader := kclient.Reader(allowAllTunnelReader{})
	if len(readers) > 0 {
		reader = readers[0]
	}
	manager, err := NewManager(ctx, "http://"+listener.Addr().String(), reader, PeerConfig{})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tunnel/connect", func(w http.ResponseWriter, r *http.Request) {
		name, ok := testTunnelNameFromRequest(r)
		if !ok {
			http.Error(w, "invalid tunnel credential", http.StatusUnauthorized)
			return
		}
		manager.ServeConnect(w, r, name)
	})
	mux.HandleFunc("GET "+PeerConnectPath, manager.ServePeer)
	mux.HandleFunc("/tunnel/bridge/{target}", manager.ServeBridge)
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	return manager, server
}

func configureTestPeer(manager *Manager, id, token string) {
	manager.ID = id
	manager.Token = token
	manager.remoteDialer.PeerID = id
	manager.remoteDialer.PeerToken = token
}

func waitForTestCondition(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func localTunnelConnectionCount(manager *Manager, name string) int {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	count := 0
	for connection := range manager.connections[name] {
		if connection.conn != nil {
			count++
		}
	}
	return count
}

func newBridgeClient(manager *Manager) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			request = request.Clone(request.Context())
			request.Header = request.Header.Clone()
			name, value := manager.BridgeAuthorization()
			request.Header.Set(name, value)
			return http.DefaultTransport.RoundTrip(request)
		}),
	}
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type allowAllTunnelReader struct{}

func (allowAllTunnelReader) Get(_ context.Context, key kclient.ObjectKey, obj kclient.Object, _ ...kclient.GetOption) error {
	tunnel := obj.(*v1.MCPTunnel)
	tunnel.Name = key.Name
	tunnel.Namespace = key.Namespace
	tunnel.Spec.Manifest.AllowedURLs = []string{"*"}
	tunnel.Spec.Credential = testTunnelCredential(key.Name)
	return nil
}

func (allowAllTunnelReader) List(context.Context, kclient.ObjectList, ...kclient.ListOption) error {
	return nil
}

type staticTunnelTestReader struct {
	tunnels map[string]v1.MCPTunnel
}

func (s *staticTunnelTestReader) Get(_ context.Context, key kclient.ObjectKey, obj kclient.Object, _ ...kclient.GetOption) error {
	tunnel, ok := s.tunnels[key.Name]
	if !ok {
		return fmt.Errorf("tunnel %q not found", key.Name)
	}
	*obj.(*v1.MCPTunnel) = tunnel
	return nil
}

func (s *staticTunnelTestReader) List(_ context.Context, obj kclient.ObjectList, _ ...kclient.ListOption) error {
	list := obj.(*v1.MCPTunnelList)
	for _, tunnel := range s.tunnels {
		list.Items = append(list.Items, tunnel)
	}
	return nil
}

func testTunnelToken(name string) string {
	rawToken := make([]byte, credentialTokenSize)
	copy(rawToken, name)
	return base64.RawURLEncoding.EncodeToString(rawToken)
}

func testTunnelCredential(name string) string {
	token := testTunnelToken(name)
	rawToken, _ := base64.RawURLEncoding.DecodeString(token)
	digest := sha256.Sum256([]byte(token))
	encoded, _ := json.Marshal(credentialRecord{
		Version: credentialVersion,
		Digest:  base64.RawURLEncoding.EncodeToString(digest[:]),
		Preview: base64.RawURLEncoding.EncodeToString(rawToken[:credentialPreviewSize]),
	})
	return string(encoded)
}

func testConfiguredTunnel(name string) v1.MCPTunnel {
	credential := testTunnelCredential(name)
	credentialID, _ := CredentialID(credential)
	tunnel := v1.MCPTunnel{}
	tunnel.Name = name
	tunnel.Namespace = system.DefaultNamespace
	tunnel.Spec.Credential = credential
	tunnel.Spec.CredentialID = credentialID
	tunnel.Spec.Manifest.AllowedURLs = []string{"*"}
	return tunnel
}

func testTunnelNameFromRequest(request *http.Request) (string, bool) {
	token, ok := tunnelBearerToken(request)
	if !ok {
		return "", false
	}
	rawToken, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(rawToken) != credentialTokenSize {
		return "", false
	}
	name := string(bytes.TrimRight(rawToken, "\x00"))
	return name, apitypes.ValidateTunnelName(name) == nil
}
