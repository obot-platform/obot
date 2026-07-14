package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"k8s.io/apiserver/pkg/authentication/user"
)

type fakeTunnelConnector struct {
	connections   []types.TunnelConnection
	err           error
	context       context.Context
	connectedName string
	peerServed    bool
}

func (f *fakeTunnelConnector) ConnectionsContext(ctx context.Context) ([]types.TunnelConnection, error) {
	f.context = ctx
	return f.connections, f.err
}

func (f *fakeTunnelConnector) ServeConnect(_ http.ResponseWriter, _ *http.Request, name string) {
	f.connectedName = name
}

func (f *fakeTunnelConnector) ServePeer(_ http.ResponseWriter, _ *http.Request) {
	f.peerServed = true
}

func TestTunnelHandlerList(t *testing.T) {
	connector := &fakeTunnelConnector{connections: []types.TunnelConnection{{
		Name: "office",
	}}}
	handler := NewTunnelHandler(connector)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tunnels", nil)
	if err := handler.List(api.Context{ResponseWriter: recorder, Request: request}); err != nil {
		t.Fatal(err)
	}

	if got := recorder.Body.String(); got != "{\"items\":[{\"name\":\"office\"}]}\n" {
		t.Fatalf("response = %q", got)
	}
	if connector.context != request.Context() {
		t.Fatal("Connections did not receive the request context")
	}
}

func TestTunnelHandlerListReturnsListerError(t *testing.T) {
	wantErr := errors.New("list failed")
	handler := NewTunnelHandler(&fakeTunnelConnector{err: wantErr})
	request := httptest.NewRequest(http.MethodGet, "/api/tunnels", nil)
	err := handler.List(api.Context{ResponseWriter: httptest.NewRecorder(), Request: request})
	if !errors.Is(err, wantErr) {
		t.Fatalf("List() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestTunnelHandlerConnectUsesAuthenticatedTunnelName(t *testing.T) {
	manager := &fakeTunnelConnector{}
	handler := NewTunnelHandler(manager)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tunnel/connect", nil)

	if err := handler.Connect(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		User: &user.DefaultInfo{
			Name: "mt1office",
			UID:  "mt1office",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if manager.connectedName != "mt1office" {
		t.Fatalf("connected name = %q, want %q", manager.connectedName, "mt1office")
	}
}

func TestTunnelHandlerPeerServesAuthenticatedReplica(t *testing.T) {
	manager := &fakeTunnelConnector{}
	handler := NewTunnelHandler(manager)
	request := httptest.NewRequest(http.MethodGet, "/tunnel/peer", nil)

	if err := handler.Peer(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
	}); err != nil {
		t.Fatal(err)
	}
	if !manager.peerServed {
		t.Fatal("ServePeer was not called")
	}
}
