package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
)

type lifecycleTokenService struct {
	next atomic.Int64
}

func (s *lifecycleTokenService) NewToken(_ context.Context, claims persistent.TokenContext) (*jwt.Token, string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	return token, fmt.Sprintf("test-token-%d", s.next.Add(1)), nil
}

type lifecycleMCPServer struct {
	server *httptest.Server

	initializeStarted chan struct{}
	releaseInitialize <-chan struct{}

	nextSession atomic.Int64

	mu      sync.Mutex
	deletes map[string]int
}

func newLifecycleMCPServer(t *testing.T, initializeStarted chan struct{}, releaseInitialize <-chan struct{}) *lifecycleMCPServer {
	t.Helper()

	s := &lifecycleMCPServer{
		initializeStarted: initializeStarted,
		releaseInitialize: releaseInitialize,
		deletes:           map[string]int{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(s.server.Close)
	return s
}

func (s *lifecycleMCPServer) serveHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		var msg struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if msg.Method != "initialize" {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		sessionID := fmt.Sprintf("session-%d", s.nextSession.Add(1))
		if s.initializeStarted != nil {
			s.initializeStarted <- struct{}{}
		}
		if s.releaseInitialize != nil {
			<-s.releaseInitialize
		}

		w.Header().Set(nmcp.SessionIDHeader, sessionID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      msg.ID,
			"result":  map[string]any{},
		})
	case http.MethodDelete:
		sessionID := req.Header.Get(nmcp.SessionIDHeader)
		s.mu.Lock()
		s.deletes[sessionID]++
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, req)
	}
}

func (s *lifecycleMCPServer) deleteCount(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletes[sessionID]
}

func newLifecycleSessionManager(t *testing.T, serverURL string) (*SessionManager, ServerConfig) {
	t.Helper()

	sm := &SessionManager{
		tokenService:          &lifecycleTokenService{},
		clientRetirementDelay: 0,
	}
	t.Cleanup(sm.Close)

	return sm, ServerConfig{
		URL:                  serverURL,
		MCPServerName:        "lifecycle-test",
		MCPServerDisplayName: "Lifecycle Test",
		UserID:               "user-1",
		Audiences:            []string{"lifecycle-test"},
	}
}

func loadLifecycleSession(t *testing.T, sm *SessionManager, server ServerConfig) *Client {
	t.Helper()

	client, err := sm.loadSession(t.Context(), server, oauthCheckClientScope, nmcp.ClientOption{})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func expireClient(client *Client) {
	client.jwt.Claims = jwt.MapClaims{
		"exp": float64(time.Now().Add(-time.Hour).Unix()),
	}
}

func TestRapidTokenReplacementBoundsRemoteSessions(t *testing.T) {
	remote := newLifecycleMCPServer(t, nil, nil)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)
	sm.clientRetirementDelay = time.Hour

	current := loadLifecycleSession(t, sm, server)

	const replacements = 10
	for range replacements {
		expireClient(current)
		current = loadLifecycleSession(t, sm, server)
	}

	var liveSessions int
	for i := 1; i <= replacements+1; i++ {
		count := remote.deleteCount(fmt.Sprintf("session-%d", i))
		if count > 1 {
			t.Fatalf("session-%d DELETE count = %d, want at most 1", i, count)
		}
		if count == 0 {
			liveSessions++
		}
	}
	if liveSessions != 2 {
		t.Fatalf("live remote sessions after replacement burst = %d, want 2", liveSessions)
	}

	sm.closeClients(server.MCPServerName)
	for i := 1; i <= replacements+1; i++ {
		if got := remote.deleteCount(fmt.Sprintf("session-%d", i)); got != 1 {
			t.Fatalf("session-%d DELETE count after shutdown = %d, want 1", i, got)
		}
	}
}

func TestDeferredRetirementHasHardDeadline(t *testing.T) {
	remote := newLifecycleMCPServer(t, nil, nil)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)
	sm.clientRetirementDelay = 10 * time.Millisecond

	oldClient := loadLifecycleSession(t, sm, server)
	oldSessionID := oldClient.Session.ID()
	expireClient(oldClient)
	replacement := loadLifecycleSession(t, sm, server)

	deadline := time.Now().Add(time.Second)
	for remote.deleteCount(oldSessionID) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := remote.deleteCount(oldSessionID); got != 1 {
		t.Fatalf("expired remote session DELETE count after deadline = %d, want 1", got)
	}
	if got := remote.deleteCount(replacement.Session.ID()); got != 0 {
		t.Fatalf("replacement remote session DELETE count = %d, want 0", got)
	}
}

func TestLoadSessionRaceDeletesLosingRemoteSessionOnce(t *testing.T) {
	initializeStarted := make(chan struct{}, 2)
	releaseInitialize := make(chan struct{})
	remote := newLifecycleMCPServer(t, initializeStarted, releaseInitialize)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)

	results := make(chan *Client, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			client, err := sm.loadSession(t.Context(), server, oauthCheckClientScope, nmcp.ClientOption{})
			results <- client
			errs <- err
		}()
	}

	for range 2 {
		<-initializeStarted
	}
	close(releaseInitialize)

	first, second := <-results, <-results
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("concurrent callers returned different cached clients")
	}

	var totalDeletes int
	for i := 1; i <= 2; i++ {
		count := remote.deleteCount(fmt.Sprintf("session-%d", i))
		if count > 1 {
			t.Fatalf("session-%d DELETE count = %d, want at most 1", i, count)
		}
		totalDeletes += count
	}
	if totalDeletes != 1 {
		t.Fatalf("losing remote session DELETE count = %d, want 1", totalDeletes)
	}
}

func TestCloseClientFlushesRetiredAndActiveSessionsOnce(t *testing.T) {
	remote := newLifecycleMCPServer(t, nil, nil)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)
	sm.clientRetirementDelay = time.Hour

	oldClient := loadLifecycleSession(t, sm, server)
	oldSessionID := oldClient.Session.ID()
	expireClient(oldClient)
	activeClient := loadLifecycleSession(t, sm, server)
	activeSessionID := activeClient.Session.ID()

	sm.CloseClient(server, oauthCheckClientScope)
	sm.CloseClient(server, oauthCheckClientScope)

	if got := remote.deleteCount(oldSessionID); got != 1 {
		t.Fatalf("retired remote session DELETE count = %d, want 1", got)
	}
	if got := remote.deleteCount(activeSessionID); got != 1 {
		t.Fatalf("active remote session DELETE count = %d, want 1", got)
	}
}

func TestClientRetireIsIdempotent(t *testing.T) {
	remote := newLifecycleMCPServer(t, nil, nil)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)

	client := loadLifecycleSession(t, sm, server)
	sessionID := client.Session.ID()

	client.retire()
	client.retire()

	if got := remote.deleteCount(sessionID); got != 1 {
		t.Fatalf("remote session DELETE count = %d, want 1", got)
	}
}

func TestSessionManagerClosePreservesActiveAndDeletesRetiredSession(t *testing.T) {
	remote := newLifecycleMCPServer(t, nil, nil)
	sm, server := newLifecycleSessionManager(t, remote.server.URL)
	sm.clientRetirementDelay = time.Hour

	oldClient := loadLifecycleSession(t, sm, server)
	oldSessionID := oldClient.Session.ID()
	expireClient(oldClient)
	activeClient := loadLifecycleSession(t, sm, server)
	activeSessionID := activeClient.Session.ID()

	sm.Close()

	if got := remote.deleteCount(oldSessionID); got != 1 {
		t.Fatalf("retired remote session DELETE count = %d, want 1", got)
	}
	if got := remote.deleteCount(activeSessionID); got != 0 {
		t.Fatalf("active remote session DELETE count = %d, want 0", got)
	}
}
