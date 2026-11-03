package mcpgateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type failingOwnershipStorage struct{ kclient.Client }

type conflictingOwnershipStatus struct{ kclient.SubResourceWriter }

type conflictingOwnershipStorage struct{ kclient.Client }

func TestMCPClientSessionOwnership(t *testing.T) {
	storage := newMCPProxyTestStorage()
	spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session"}}}
	require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, false))

	for _, test := range []struct {
		name       string
		connection string
		user       string
		allowed    bool
	}{
		{
			name:       "owner with a changed credential",
			connection: "connection",
			user:       "alice",
			allowed:    true,
		},
		{
			name:       "another user",
			connection: "connection",
			user:       "bob",
		},
		{
			name:       "another connection",
			connection: "other",
			user:       "alice",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A new load uses shared storage, as a request on another replica would.
			session, err := loadMCPClientSession(t.Context(), storage, v1.MCPClientSessionSpec{MCPServerID: spec.MCPServerID, ConnectionID: test.connection, UserID: test.user}, "session")
			if test.allowed {
				require.NoError(t, err)
				require.NotNil(t, session)
			} else {
				require.ErrorIs(t, err, errMCPSessionNotFound)
				require.Nil(t, session)
			}
		})
	}
}

func TestSaveMCPClientSessionSuccessfulResponses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted} {
		storage := newMCPProxyTestStorage()
		spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
		resp := &http.Response{StatusCode: status, Header: http.Header{mcpSessionHeader: {"session"}}}

		// Server-issued IDs are registered even when auditing is disabled.
		require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, false))

		session, err := loadMCPClientSession(t.Context(), storage, spec, "session")
		require.NoError(t, err, "successful status %d", status)
		require.False(t, session.Spec.Virtual)
	}
}

func TestSaveMCPClientSessionDoesNotInventSessions(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusUnauthorized} {
		storage := newMCPProxyTestStorage()
		spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
		resp := &http.Response{StatusCode: status, Header: make(http.Header)}
		initialize := status != http.StatusOK
		if status == http.StatusUnauthorized {
			resp.Header.Set(mcpSessionHeader, "untrusted")
		}

		require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, initialize))

		var sessions v1.MCPClientSessionList
		require.NoError(t, storage.List(t.Context(), &sessions))
		require.Empty(t, sessions.Items)
	}
}

func TestMCPClientSessionAndAuditShareOneRecord(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		storage := newMCPProxyTestStorage()
		collector := new(recordingProxyAuditCollector)
		metadata := map[string]string{"mcpID": "vmcp-parent", "userID": "alice"}
		req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Desktop","version":"1.2.3"}}}`))
		audit, err := newProxyAudit(req, metadata, collector)
		require.NoError(t, err)

		var sessions v1.MCPClientSessionList
		require.NoError(t, storage.List(t.Context(), &sessions))
		require.Empty(t, sessions.Items, "requests must not create session records")

		spec := v1.MCPClientSessionSpec{
			ConnectionID:  "vmcp-instance",
			MCPServerID:   metadata["mcpID"],
			UserID:        "alice",
			ClientName:    audit.entry.ClientName,
			ClientVersion: audit.entry.ClientVersion,
		}

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{}}`)),
		}

		if !virtual {
			resp.Header.Set(mcpSessionHeader, "session")
		}

		audit.recordRequest()
		require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, audit.initialize))

		consumeAuditResponse(t, audit, resp)

		require.NoError(t, storage.List(t.Context(), &sessions))
		require.Len(t, sessions.Items, 1)

		record := sessions.Items[0]
		require.Equal(t, spec.ConnectionID, record.Spec.ConnectionID)
		require.Equal(t, spec.MCPServerID, record.Spec.MCPServerID)
		require.Equal(t, virtual, record.Spec.Virtual)

		record.Status.LastUsed = metav1.NewTime(time.Now().Add(-time.Hour))
		require.NoError(t, storage.Status().Update(t.Context(), &record))

		// Resume on another replica and restore the audit's client metadata from the same record.
		id := mcpSessionID(resp.Header, nil)
		session, err := loadMCPClientSession(t.Context(), storage, spec, id)
		require.NoError(t, err)

		followup := httptest.NewRequest(http.MethodPost, "http://obot/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
		followup.Header.Set(mcpSessionHeader, id)
		nextAudit, err := newProxyAudit(followup, metadata, collector)
		require.NoError(t, err)

		nextAudit.setClientSession(session)
		require.Equal(t, "Claude Desktop", nextAudit.entry.ClientName)
		require.Equal(t, "1.2.3", nextAudit.entry.ClientVersion)
		require.True(t, session.Status.LastUsed.After(record.Status.LastUsed.Time))
		require.NoError(t, storage.List(t.Context(), &sessions))
		require.Len(t, sessions.Items, 1)
	}
}

func TestLoadMCPClientSessionRejectsInconsistentOwnership(t *testing.T) {
	for _, field := range []string{"user", "connection"} {
		storage := newMCPProxyTestStorage()
		spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session"}}}
		require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, false))

		var record v1.MCPClientSession
		require.NoError(t, storage.Get(t.Context(), mcpClientSessionKey(spec, "session"), &record))

		switch field {
		case "user":
			record.Spec.UserID = "bob"
			require.NoError(t, storage.Update(t.Context(), &record))

		case "connection":
			record.Spec.ConnectionID = "other"
			require.NoError(t, storage.Update(t.Context(), &record))
		}

		session, err := loadMCPClientSession(t.Context(), storage, spec, "session")
		require.ErrorIs(t, err, errMCPSessionNotFound, "invalid field %s", field)
		require.Nil(t, session)
	}
}

func TestLoadMCPClientSessionKeepsLegacySessions(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		storage := newMCPProxyTestStorage()
		spec := v1.MCPClientSessionSpec{MCPServerID: "vmcp-parent", ConnectionID: "vmcp-instance", UserID: "alice"}
		// This is the exact key format used before this change, with no ConnectionID.
		record := v1.MCPClientSession{
			Name:      name.SafeConcatName("mcp", "client", "session", hash.String([]string{"vmcp-parent", "alice", "session"})),
			Namespace: "default",
			Spec: v1.MCPClientSessionSpec{
				MCPServerID:   "vmcp-parent",
				UserID:        "alice",
				ClientName:    "existing-client",
				ClientVersion: "1",
				Virtual:       virtual,
			},
			Status: v1.MCPClientSessionStatus{LastUsed: metav1.Now()},
		}
		require.NoError(t, storage.Create(t.Context(), &record))

		session, err := loadMCPClientSession(t.Context(), storage, spec, "session")
		require.NoError(t, err)
		require.Equal(t, "existing-client", session.Spec.ClientName)
		require.Equal(t, virtual, session.Spec.Virtual)

		spec.UserID = "bob"
		_, err = loadMCPClientSession(t.Context(), storage, spec, "session")
		require.ErrorIs(t, err, errMCPSessionNotFound)
		spec.UserID = "alice"
		_, err = loadMCPClientSession(t.Context(), storage, spec, "unknown")
		require.ErrorIs(t, err, errMCPSessionNotFound)
	}
}

func TestSuppliedMCPSessionID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://obot/mcp?id=ordinary&session_id=filter", nil)
	id, err := suppliedSessionID(req)
	require.NoError(t, err)
	require.Empty(t, id)
	require.Equal(t, "id=ordinary&session_id=filter", req.URL.RawQuery)

	req.Header.Add(mcpSessionHeader, "session")
	req.Header.Add(mcpSessionHeader, "session")
	id, err = suppliedSessionID(req)
	require.NoError(t, err)
	require.Equal(t, "session", id)

	req.Header.Add(mcpSessionHeader, "conflict")
	_, err = suppliedSessionID(req)
	require.ErrorContains(t, err, "conflicting MCP session identifiers")
}

func (failingOwnershipStorage) Get(context.Context, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
	return errors.New("storage unavailable")
}

func TestLoadMCPClientSessionFailsClosed(t *testing.T) {
	storage := failingOwnershipStorage{Client: newMCPProxyTestStorage()}
	spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
	session, err := loadMCPClientSession(t.Context(), storage, spec, "session")
	require.ErrorContains(t, err, "storage unavailable")
	require.Nil(t, session)

	session, err = loadMCPClientSession(t.Context(), storage, spec, "")
	require.NoError(t, err)
	require.Nil(t, session)
}

func (conflictingOwnershipStatus) Update(context.Context, kclient.Object, ...kclient.SubResourceUpdateOption) error {
	return apierrors.NewConflict(schema.GroupResource{Resource: "mcpclientsessions"}, "session", errors.New("concurrent activity update"))
}

func (s conflictingOwnershipStorage) Status() kclient.SubResourceWriter {
	return conflictingOwnershipStatus{SubResourceWriter: s.Client.Status()}
}

func TestSaveMCPClientSessionIgnoresActivityConflicts(t *testing.T) {
	storage := newMCPProxyTestStorage()
	spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session"}}}
	require.NoError(t, saveMCPClientSession(t.Context(), storage, resp, spec, false))
	require.NoError(t, saveMCPClientSession(t.Context(), conflictingOwnershipStorage{Client: storage}, resp, spec, false))
}

func TestSaveMCPClientSessionConcurrentRegistration(t *testing.T) {
	storage := newMCPProxyTestStorage()
	results := make(chan error, 8)

	for range 8 {
		go func() {
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session"}}}
			spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
			results <- saveMCPClientSession(t.Context(), storage, resp, spec, false)
		}()
	}

	for range 8 {
		require.NoError(t, <-results)
	}

	spec := v1.MCPClientSessionSpec{ConnectionID: "connection", UserID: "alice"}
	_, err := loadMCPClientSession(t.Context(), storage, spec, "session")
	require.NoError(t, err)
}

func TestInitializeInspectionPreservesWireBody(t *testing.T) {
	for _, method := range []string{"initialize", "tools/call"} {
		for _, encoding := range []string{"", "gzip"} {
			body := []byte(`{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`)
			if encoding == "gzip" {
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				_, err := writer.Write(body)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body = compressed.Bytes()
			}

			req := httptest.NewRequest(http.MethodPost, "http://obot/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Encoding", encoding)
			req.Header.Set(mcpSessionHeader, "stale")
			require.Equal(t, method == "initialize", isMCPInitializeRequest(req))

			forwarded, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, forwarded)
			require.Equal(t, encoding, req.Header.Get("Content-Encoding"))
		}
	}
}

func TestLoadMCPClientSessionRefreshesIdleRecords(t *testing.T) {
	for _, missingActivity := range []bool{false, true} {
		storage := newMCPProxyTestStorage()
		spec := v1.MCPClientSessionSpec{MCPServerID: "server", ConnectionID: "connection", UserID: "alice"}
		key := mcpClientSessionKey(spec, "session")
		record := v1.MCPClientSession{
			Name:              key.Name,
			Namespace:         key.Namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-8 * 24 * time.Hour)),
			Spec:              spec,
		}
		if !missingActivity {
			record.Status.LastUsed = record.CreationTimestamp
		}
		require.NoError(t, storage.Create(t.Context(), &record))

		// Existing records remain usable until the cleanup controller deletes them.
		session, err := loadMCPClientSession(t.Context(), storage, spec, "session")
		require.NoError(t, err)
		require.True(t, session.Status.LastUsed.After(record.CreationTimestamp.Time))
	}
}
