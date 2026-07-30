package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
)

func TestServeConnectClosesServerHalfAfterClientCloseWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	manager, server := newManagerTestServer(ctx, t)
	t.Cleanup(func() {
		cancel()
		manager.Close()
		server.Close()
	})

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testTunnelToken("office"))
	assertServerClosesAfterClientCloseWrite(t, server.URL+"/tunnel/connect", header)
}

func TestServePeerClosesServerHalfAfterClientCloseWrite(t *testing.T) {
	const (
		peerID    = "replica-b"
		peerToken = "shared-peer-token"
	)

	ctx, cancel := context.WithCancel(t.Context())
	manager, server := newManagerTestServer(ctx, t)
	configureTestPeer(manager, "replica-a", peerToken)
	manager.ReconcilePeers(map[string]string{
		peerID: "ws" + strings.TrimPrefix(server.URL, "http") + "/not-a-peer",
	})
	t.Cleanup(func() {
		cancel()
		manager.Close()
		server.Close()
	})

	header := make(http.Header)
	header.Set(remotedialer.ID, peerID)
	header.Set(remotedialer.Token, peerToken)
	assertServerClosesAfterClientCloseWrite(t, server.URL+PeerConnectPath, header)
}

func TestTunnelClientClosesAfterServerCloseWrite(t *testing.T) {
	accepted := make(chan struct{})
	serverRead := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		}).Upgrade(w, r, nil)
		if err != nil {
			serverRead <- err
			return
		}
		defer connection.Close()

		tcpConnection, ok := connection.UnderlyingConn().(*net.TCPConn)
		if !ok {
			serverRead <- errors.New("websocket server did not use a TCP connection")
			return
		}
		close(accepted)
		if err := tcpConnection.CloseWrite(); err != nil {
			serverRead <- err
			return
		}
		if err := tcpConnection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			serverRead <- err
			return
		}

		var data [1]byte
		count, err := tcpConnection.Read(data[:])
		if count != 0 {
			serverRead <- errors.New("server read data after closing its write half")
			return
		}
		serverRead <- err
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
	case err := <-clientError:
		if !websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
			t.Fatalf("client error after server CloseWrite = %v, want abnormal websocket closure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tunnel client did not close after the server closed its write half")
	}

	select {
	case err := <-serverRead:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("reading client half after server CloseWrite = %v, want EOF", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe the client closing its remaining half")
	}
}

func assertServerClosesAfterClientCloseWrite(t *testing.T, endpoint string, header http.Header) {
	t.Helper()

	var tcpConnection *net.TCPConn
	dialer := websocket.Dialer{
		HandshakeTimeout: 3 * time.Second,
		NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			tcpConnection, _ = connection.(*net.TCPConn)
			return connection, nil
		},
	}
	websocketConnection, response, err := dialer.DialContext(
		t.Context(),
		"ws"+strings.TrimPrefix(endpoint, "http"),
		header,
	)
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("dialing websocket: %v: %s", err, body)
		}
		t.Fatalf("dialing websocket: %v", err)
	}
	defer websocketConnection.Close()
	if tcpConnection == nil {
		t.Fatal("websocket dial did not use a TCP connection")
	}

	if err := tcpConnection.CloseWrite(); err != nil {
		t.Fatalf("closing client write half: %v", err)
	}
	if err := tcpConnection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("setting client read deadline: %v", err)
	}

	var data [1]byte
	count, err := tcpConnection.Read(data[:])
	if count != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("reading server half after client CloseWrite = (%d, %v), want (0, EOF)", count, err)
	}
}
