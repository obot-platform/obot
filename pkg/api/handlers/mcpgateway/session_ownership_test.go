package mcpgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSessionOwnership(t *testing.T) {
	storage := newMCPProxyTestStorage()
	owner := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}
	request := httptest.NewRequest(http.MethodPost, "http://obot/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	status, err := owner.prepare(request)
	require.NoError(t, err)
	require.Zero(t, status)

	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session"}}}
	require.NoError(t, owner.response(response))

	for _, test := range []struct {
		name       string
		user       string
		connection string
		method     string
		want       int
	}{
		{
			name:       "owner POST",
			user:       "alice",
			connection: "connection",
			method:     http.MethodPost,
		},
		{
			name:       "owner GET",
			user:       "alice",
			connection: "connection",
			method:     http.MethodGet,
		},
		{
			name:       "other user POST",
			user:       "bob",
			connection: "connection",
			method:     http.MethodPost,
			want:       http.StatusNotFound,
		},
		{
			name:       "other user GET",
			user:       "bob",
			connection: "connection",
			method:     http.MethodGet,
			want:       http.StatusNotFound,
		},
		{
			name:       "other user DELETE",
			user:       "bob",
			connection: "connection",
			method:     http.MethodDelete,
			want:       http.StatusNotFound,
		},
		{
			name:       "other connection",
			user:       "alice",
			connection: "other",
			method:     http.MethodGet,
			want:       http.StatusNotFound,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A new guard represents a request on another replica, without an audit collector.
			guard := &sessionOwnership{ctx: t.Context(), storage: storage, connection: test.connection, user: test.user}
			req := httptest.NewRequest(test.method, "http://obot/mcp", nil)
			req.Header.Set(mcpSessionHeader, "session")
			req.Header.Set("Authorization", "Bearer changed-credential")

			status, err := guard.prepare(req)
			require.Equal(t, test.want, status)

			if test.want == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}

	owner.method = http.MethodDelete
	owner.id = "session"
	require.NoError(t, owner.response(&http.Response{StatusCode: http.StatusNoContent}))

	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp", nil)
	req.Header.Set(mcpSessionHeader, "session")

	status, err = owner.prepare(req)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, status)
}

func TestSessionOwnershipVirtual(t *testing.T) {
	storage := newMCPProxyTestStorage()
	owner := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice", initialize: true}
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	require.NoError(t, owner.response(resp))

	id := resp.Header.Get(mcpSessionHeader)
	require.NotEmpty(t, id)

	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp", nil)
	req.Header.Set(mcpSessionHeader, id)

	_, err := owner.prepare(req)
	require.NoError(t, err)
	require.True(t, owner.virtual)
}

func TestIncomingSessionCannotClaimOwnership(t *testing.T) {

	guard := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "bob"}

	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"attacker","version":"1"}}}`))
		req.Header.Set(mcpSessionHeader, "stolen")

		status, err := guard.prepare(req)
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, status)
	}
}

func TestSessionOwnershipHTTPBoundary(t *testing.T) {
	storage := newMCPProxyTestStorage()
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set(mcpSessionHeader, "upstream-session")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := &sessionOwnership{ctx: r.Context(), storage: storage, connection: "shared", user: r.Header.Get("Test-User")}
		if status, err := owner.prepare(r); err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		out, err := http.NewRequestWithContext(r.Context(), r.Method, upstream.URL, r.Body)
		require.NoError(t, err)

		out.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(out)
		require.NoError(t, err)

		defer resp.Body.Close()
		if err = owner.response(resp); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer gateway.Close()
	send := func(user, method, id, body string) int {
		req, err := http.NewRequest(method, gateway.URL, strings.NewReader(body))
		require.NoError(t, err)

		req.Header.Set("Test-User", user)
		if id != "" {
			req.Header.Set(mcpSessionHeader, id)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)

		return resp.StatusCode
	}
	require.Equal(t, http.StatusOK, send("alice", http.MethodPost, "", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`))

	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		require.Equal(t, http.StatusNotFound, send("bob", method, "upstream-session", `{"jsonrpc":"2.0","id":2,"method":"tools/call"}`))
	}
	require.Equal(t, 1, calls, "another user's session must never reach upstream")
	require.Equal(t, http.StatusOK, send("alice", http.MethodPost, "upstream-session", `{"jsonrpc":"2.0","id":2,"method":"tools/call"}`))
}

func TestSessionOwnershipRejectsUnverifiedAndExpired(t *testing.T) {
	storage := newMCPProxyTestStorage()

	guard := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}
	require.NoError(t, guard.register("session", false))

	var record v1.MCPClientSession
	require.NoError(t, storage.Get(t.Context(), guard.key("session"), &record))

	record.Spec.OwnershipVerified = false
	require.NoError(t, storage.Update(t.Context(), &record))

	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp", nil)
	req.Header.Set(mcpSessionHeader, "session")

	status, err := guard.prepare(req)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, status)

	record.Spec.OwnershipVerified = true
	require.NoError(t, storage.Update(t.Context(), &record))

	record.Status.LastUsed = metav1.NewTime(time.Now().Add(-8 * 24 * time.Hour))
	require.NoError(t, storage.Status().Update(t.Context(), &record))

	status, err = guard.prepare(req)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, status)
}

type failingOwnershipStorage struct{ kclient.Client }

func (failingOwnershipStorage) Get(context.Context, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
	return errors.New("storage unavailable")
}

func TestSessionOwnershipFailsClosed(t *testing.T) {

	guard := &sessionOwnership{ctx: t.Context(), storage: failingOwnershipStorage{Client: newMCPProxyTestStorage()}, connection: "connection", user: "alice"}
	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp", nil)
	req.Header.Set(mcpSessionHeader, "session")

	status, err := guard.prepare(req)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, status)
}

func TestSessionOwnershipConcurrentRegistration(t *testing.T) {
	storage := newMCPProxyTestStorage()
	results := make(chan error, 8)

	for range 8 {
		go func() {

			guard := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}
			results <- guard.register("session", false)
		}()
	}

	for range 8 {
		err := <-results
		require.NoError(t, err)
	}

	guard := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}

	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp", nil)
	req.Header.Set(mcpSessionHeader, "session")

	status, err := guard.prepare(req)
	require.NoError(t, err)
	require.Zero(t, status)
}

func TestSessionOwnershipPreservesOrdinaryQuery(t *testing.T) {
	guard := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "alice"}
	req := httptest.NewRequest(http.MethodGet, "http://obot/mcp?id=123&session_id=filter", nil)

	status, err := guard.prepare(req)
	require.NoError(t, err)
	require.Zero(t, status)
	require.Equal(t, "id=123&session_id=filter", req.URL.RawQuery)
}

func TestSessionOwnershipDoesNotInspectBodies(t *testing.T) {
	guard := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "alice"}
	require.NoError(t, guard.register("session", false))

	req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", nil)
	req.Body = unreadableOwnershipBody{}
	req.Header.Set(mcpSessionHeader, "session")
	req.Header.Set("Content-Encoding", "br")

	status, err := guard.prepare(req)
	require.NoError(t, err)
	require.Zero(t, status)
}

type unreadableOwnershipBody struct{}

func (unreadableOwnershipBody) Read([]byte) (int, error) {
	return 0, errors.New("unexpected body read")
}
func (unreadableOwnershipBody) Close() error { return nil }

func TestSessionOwnershipDoesNotInspectSessionlessBodies(t *testing.T) {
	guard := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "alice"}
	req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", nil)
	req.Body = unreadableOwnershipBody{}
	req.Header.Set("Content-Encoding", "br")

	status, err := guard.prepare(req)
	require.NoError(t, err)
	require.Zero(t, status)
}

func TestVirtualSessionUsesExistingAuditInspection(t *testing.T) {
	for _, auditEnabled := range []bool{false, true} {
		storage := newMCPProxyTestStorage()
		owner := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}
		req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"client","version":"1"}}}`))

		status, err := owner.prepare(req)
		require.NoError(t, err)
		require.Zero(t, status)

		var collector proxyAuditCollector
		if auditEnabled {
			collector = new(recordingProxyAuditCollector)
		}

		audit, err := newProxyAudit(req, map[string]string{"mcpID": "connection", "userID": "alice"}, collector, storage)
		require.NoError(t, err)
		owner.initialize = audit != nil && audit.initialize

		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		require.NoError(t, owner.response(resp))

		id := resp.Header.Get(mcpSessionHeader)
		if auditEnabled {
			require.NotEmpty(t, id)
			req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", nil)
			req.Header.Set(mcpSessionHeader, id)
			_, err := owner.prepare(req)
			require.NoError(t, err)
			require.True(t, owner.virtual)
		} else {
			require.Empty(t, id)
		}
	}
}

type conflictingOwnershipStatus struct{ kclient.SubResourceWriter }

func (conflictingOwnershipStatus) Update(context.Context, kclient.Object, ...kclient.SubResourceUpdateOption) error {
	return apierrors.NewConflict(schema.GroupResource{Resource: "mcpclientsessions"}, "session", errors.New("concurrent activity update"))
}

type conflictingOwnershipStorage struct{ kclient.Client }

func (s conflictingOwnershipStorage) Status() kclient.SubResourceWriter {
	return conflictingOwnershipStatus{SubResourceWriter: s.Client.Status()}
}

func TestSessionRegistrationIgnoresActivityConflicts(t *testing.T) {
	storage := newMCPProxyTestStorage()
	guard := &sessionOwnership{ctx: t.Context(), storage: storage, connection: "connection", user: "alice"}
	require.NoError(t, guard.register("session", false))

	guard.storage = conflictingOwnershipStorage{Client: storage}
	require.NoError(t, guard.register("session", false))
}

func TestSessionOwnershipRejectsLegacyEndpointEvents(t *testing.T) {
	owner := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "alice"}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: endpoint\ndata: /messages?session_id=legacy\n\n")),
	}
	require.NoError(t, owner.response(resp))

	body, err := io.ReadAll(resp.Body)
	require.ErrorContains(t, err, "legacy HTTP+SSE transport is not supported")
	require.Empty(t, body)

	_, err = resp.Body.Read(make([]byte, 1))
	require.ErrorContains(t, err, "legacy HTTP+SSE transport is not supported")
}

func TestSessionOwnershipPreservesStreamableSSE(t *testing.T) {
	owner := &sessionOwnership{ctx: t.Context(), storage: newMCPProxyTestStorage(), connection: "connection", user: "alice"}
	stream := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}
	require.NoError(t, owner.response(resp))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, stream, string(body))
}
