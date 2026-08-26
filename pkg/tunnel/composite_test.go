package tunnel

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rancher/remotedialer"
)

func TestCompositeSessionKey(t *testing.T) {
	first := CompositeSessionKey("mcp-server-one")
	if first != CompositeSessionKey("mcp-server-one") {
		t.Fatal("composite key is not deterministic")
	}
	if !strings.HasPrefix(first, compositeSessionKeyPrefix) || !validCompositeSessionKey(first) {
		t.Fatalf("composite key %q is not a valid namespaced key", first)
	}
	if first == CompositeSessionKey("mcp-server-two") {
		t.Fatal("different MCP server names shared a composite key")
	}
}

func TestCompositeConnectAuthorizationIsExact(t *testing.T) {
	if !compositeConnectAuthorized(compositeNetwork, compositeAddress) {
		t.Fatal("expected composite network/address to be authorized")
	}
	for _, pair := range [][2]string{
		{forwardNetwork, forwardAddress},
		{compositeNetwork, "other"},
		{"other", compositeAddress},
		{disconnectNetwork, disconnectAddress},
	} {
		if compositeConnectAuthorized(pair[0], pair[1]) {
			t.Fatalf("unexpected authorization for %s/%s", pair[0], pair[1])
		}
	}
}

func TestCompositeRegistrationRequiresCapability(t *testing.T) {
	manager, server := newManagerTestServer(t.Context(), t)
	defer server.Close()
	defer manager.Close()

	key := CompositeSessionKey("server")
	response, err := http.Get(server.URL + compositeRegisterPath + key)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unauthenticated registration status = %d, want 404", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+compositeRegisterPath+"not-a-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, value := manager.BridgeAuthorization()
	request.Header.Set(name, value)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid-key registration status = %d, want 404", response.StatusCode)
	}
}

func TestCompositeClaimCoalescesAndTearsDown(t *testing.T) {
	manager, server := newManagerTestServer(t.Context(), t)
	defer server.Close()
	defer manager.Close()

	lifetime, cancelLifetime := context.WithCancel(context.Background())
	key := CompositeSessionKey("coalesced")
	errors := make(chan error, 20)
	for range 20 {
		go func() {
			errors <- manager.ClaimCompositeSession(t.Context(), lifetime, key)
		}()
	}
	for range 20 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}

	manager.compositeMu.Lock()
	registrations := len(manager.compositeRegistrations)
	manager.compositeMu.Unlock()
	if registrations != 1 {
		t.Fatalf("local registrations = %d, want 1", registrations)
	}
	if !manager.HasCompositeSession(key) {
		t.Fatal("claimed composite session is not advertised")
	}

	cancelLifetime()
	manager.CloseCompositeSessions()
	waitForTestCondition(t, 2*time.Second, func() bool {
		return !manager.HasCompositeSession(key)
	}, "composite session remained advertised after teardown")
}

func TestCompositeRegistrationReconnects(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(t.Context(), "http://"+listener.Addr().String(), allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	defer manager.Close()

	accepted := make(chan net.Conn, 2)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tunnel/composite/register/{key}", func(w http.ResponseWriter, r *http.Request) {
		manager.ServeCompositeRegistration(&hijackTrackingResponseWriter{
			ResponseWriter: w,
			onHijack: func(connection net.Conn) bool {
				accepted <- connection
				return true
			},
		}, r)
	})
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

	key := CompositeSessionKey("reconnect")
	if err := manager.ClaimCompositeSession(t.Context(), t.Context(), key); err != nil {
		t.Fatal(err)
	}
	var first net.Conn
	select {
	case first = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("initial composite registration was not accepted")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("composite registration did not reconnect")
	}
	waitForTestCondition(t, 2*time.Second, func() bool {
		return manager.HasCompositeSession(key)
	}, "reconnected composite session was not advertised")
}

func TestCompositeOwnerMarkerAndControlHeaders(t *testing.T) {
	manager, err := NewManager(t.Context(), "http://127.0.0.1:8080", allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	request := httptest.NewRequest(http.MethodPost, "/mcp-connect-composite/server", nil)
	request.Header.Set(compositeOwnerHeader, "spoofed")
	request.Header.Set(bridgeAuthorizationHeader, "spoofed")
	request.Header.Set(forwardTargetHeader, "spoofed")
	request.Header.Set(forwardErrorHeader, "spoofed")
	request.Header.Set(tunnelNameHeader, "spoofed")
	request.Header.Set(remotedialer.ID, "spoofed")
	request.Header.Set(remotedialer.Token, "spoofed")
	present, valid := manager.ConsumeCompositeOwnerRequest(request)
	if !present || valid {
		t.Fatalf("spoofed marker result = present %v valid %v", present, valid)
	}
	for _, name := range []string{
		compositeOwnerHeader, bridgeAuthorizationHeader, forwardTargetHeader,
		forwardErrorHeader, tunnelNameHeader, remotedialer.ID, remotedialer.Token,
	} {
		if value := request.Header.Get(name); value != "" {
			t.Fatalf("control header %s was not stripped: %q", name, value)
		}
	}

	request.Header.Set(compositeOwnerHeader, manager.bridgeAuthorization)
	request.Header.Set("Authorization", "Bearer original-user")
	if _, ok, err := manager.AuthenticateRequest(request); err != nil || ok {
		t.Fatalf("owner marker replaced bearer authentication: ok %v error %v", ok, err)
	}
	present, valid = manager.ConsumeCompositeOwnerRequest(request)
	if !present || !valid {
		t.Fatalf("authentic marker result = present %v valid %v", present, valid)
	}
}

func TestCompositeForwardsAcrossReplicaWithStateStreamingCancellationAndTeardown(t *testing.T) {
	type state struct {
		sync.Mutex
		initialized bool
		requests    int
	}
	ownerState := &state{}
	enteredBlock := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	ownerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(compositeOwnerHeader, "downstream-control")
		w.Header().Set(forwardTargetHeader, "downstream-control")
		w.Header().Set(remotedialer.ID, "downstream-control")
		if r.Header.Get("Authorization") != "Bearer original-user" {
			http.Error(w, "authorization identity was not preserved", http.StatusUnauthorized)
			return
		}
		for _, name := range []string{
			compositeOwnerHeader, bridgeAuthorizationHeader, forwardTargetHeader,
			forwardErrorHeader, tunnelNameHeader, remotedialer.ID, remotedialer.Token,
		} {
			if r.Header.Get(name) != "" {
				http.Error(w, "internal control header reached owner handler", http.StatusBadRequest)
				return
			}
		}
		if r.URL.Query().Get("block") == "true" {
			enteredBlock <- struct{}{}
			<-r.Context().Done()
			canceled <- struct{}{}
			return
		}

		ownerState.Lock()
		defer ownerState.Unlock()
		ownerState.requests++
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if string(body) == "initialize" {
				ownerState.initialized = true
				w.Header().Set("Mcp-Session-Id", "stateful-session")
				w.WriteHeader(http.StatusCreated)
				return
			}
			if !ownerState.initialized || r.Header.Get("Mcp-Session-Id") != "stateful-session" {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, "post-on-owner")
		case http.MethodGet:
			if !ownerState.initialized || r.Header.Get("Mcp-Session-Id") != "stateful-session" {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message\ndata: on-owner\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case http.MethodDelete:
			if !ownerState.initialized || r.Header.Get("Mcp-Session-Id") != "stateful-session" {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			ownerState.initialized = false
			w.WriteHeader(http.StatusNoContent)
		}
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, firstServer := newCompositeManagerTestServer(ctx, t, ownerHandler)
	defer firstServer.Close()
	defer first.Close()
	second, secondServer := newCompositeManagerTestServer(ctx, t, ownerHandler)
	defer secondServer.Close()
	defer second.Close()

	const peerToken = "composite-peer-token"
	configureTestPeer(first, "replica-a", peerToken)
	configureTestPeer(second, "replica-b", peerToken)
	first.ReconcilePeers(map[string]string{
		"replica-b": strings.Replace(secondServer.URL, "http://", "ws://", 1) + PeerConnectPath,
	})
	second.ReconcilePeers(map[string]string{
		"replica-a": strings.Replace(firstServer.URL, "http://", "ws://", 1) + PeerConnectPath,
	})

	key := CompositeSessionKey("resolved-server")
	if err := first.ClaimCompositeSession(t.Context(), ctx, key); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 10*time.Second, func() bool {
		return second.HasCompositeSession(key)
	}, "second replica did not learn composite owner")

	forward := func(manager *Manager, method, path, body string, session bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer original-user")
		request.Header.Set(forwardTargetHeader, "external-spoof")
		if session {
			request.Header.Set("Mcp-Session-Id", "stateful-session")
		}
		recorder := httptest.NewRecorder()
		if err := manager.ForwardComposite(recorder, request, key, "resolved-server"); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{compositeOwnerHeader, forwardTargetHeader, remotedialer.ID} {
			if value := recorder.Header().Get(name); value != "" {
				t.Fatalf("downstream control header %s was not stripped: %q", name, value)
			}
		}
		return recorder
	}

	initialized := forward(first, http.MethodPost, "http://replica-a/mcp-connect-composite/alias-a", "initialize", false)
	if initialized.Code != http.StatusCreated || initialized.Header().Get("Mcp-Session-Id") != "stateful-session" {
		t.Fatalf("initialize response = %d %#v", initialized.Code, initialized.Header())
	}
	posted := forward(second, http.MethodPost, "http://replica-b/mcp-connect-composite/alias-b", "stateful", true)
	if posted.Code != http.StatusOK || posted.Body.String() != "post-on-owner" {
		t.Fatalf("stateful POST response = %d %q", posted.Code, posted.Body.String())
	}
	streamed := forward(second, http.MethodGet, "http://replica-b/mcp-connect-composite/another-alias", "", true)
	if streamed.Code != http.StatusOK || streamed.Body.String() != "event: message\ndata: on-owner\n\n" {
		t.Fatalf("SSE response = %d %q", streamed.Code, streamed.Body.String())
	}

	cancelCtx, cancelRequest := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "http://replica-b/mcp-connect-composite/alias?block=true", nil).WithContext(cancelCtx)
	request.Header.Set("Authorization", "Bearer original-user")
	request.Header.Set("Mcp-Session-Id", "stateful-session")
	forwardDone := make(chan error, 1)
	go func() {
		forwardDone <- second.ForwardComposite(httptest.NewRecorder(), request, key, "resolved-server")
	}()
	select {
	case <-enteredBlock:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request did not reach composite owner")
	}
	cancelRequest()
	select {
	case <-forwardDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled composite request did not return")
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("owner did not observe composite request cancellation")
	}

	deleted := forward(second, http.MethodDelete, "http://replica-b/mcp-connect-composite/alias-b", "", true)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE response = %d %q", deleted.Code, deleted.Body.String())
	}

	first.CloseCompositeSessions()
	waitForTestCondition(t, 5*time.Second, func() bool {
		return !first.HasCompositeSession(key) && !second.HasCompositeSession(key)
	}, "composite owner remained advertised after owner teardown")
	unavailable := httptest.NewRequest(http.MethodPost, "http://replica-b/mcp-connect-composite/alias-b", strings.NewReader("stateful"))
	if err := second.ForwardComposite(httptest.NewRecorder(), unavailable, key, "resolved-server"); err == nil {
		t.Fatal("request unexpectedly succeeded after owner failure; client must reinitialize")
	}
}

func newCompositeManagerTestServer(ctx context.Context, t *testing.T, owner http.Handler) (*Manager, *httptest.Server) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(ctx, "http://"+listener.Addr().String(), allowAllTunnelReader{}, PeerConfig{})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PeerConnectPath, manager.ServePeer)
	mux.HandleFunc("GET /tunnel/composite/register/{key}", manager.ServeCompositeRegistration)
	mux.HandleFunc("/mcp-connect-composite/{mcp_id}", func(w http.ResponseWriter, r *http.Request) {
		present, valid := manager.ConsumeCompositeOwnerRequest(r)
		if !present || !valid {
			http.Error(w, "invalid owner marker", http.StatusForbidden)
			return
		}
		owner.ServeHTTP(w, r)
	})
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	return manager, server
}
