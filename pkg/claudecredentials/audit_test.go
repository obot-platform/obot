package claudecredentials

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type auditStream struct {
	grpc.ServerStream
	ctx       context.Context
	messages  []*ext.ProcessingRequest
	sent      []*ext.ProcessingResponse
	onSend    func()
	recvError error
}

func (s *auditStream) Context() context.Context { return s.ctx }
func (s *auditStream) Recv() (*ext.ProcessingRequest, error) {
	if len(s.messages) == 0 {
		if s.recvError != nil {
			return nil, s.recvError
		}
		return nil, io.EOF
	}
	msg := s.messages[0]
	s.messages = s.messages[1:]
	return msg, nil
}
func (s *auditStream) Send(r *ext.ProcessingResponse) error {
	if s.onSend != nil {
		s.onSend()
	}
	s.sent = append(s.sent, r)
	return nil
}

func auditContext(t *testing.T) context.Context {
	t.Helper()
	uri, err := url.Parse(egressIdentity)
	require.NoError(t, err)
	cert := &x509.Certificate{URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	return peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}}})
}
func auditHeaders(values map[string]string) *ext.HttpHeaders {
	result := &ext.HttpHeaders{Headers: &core.HeaderMap{}}
	for key, val := range values {
		result.Headers.Headers = append(result.Headers.Headers, &core.HeaderValue{Key: key, RawValue: []byte(val)})
	}
	return result
}

func TestEgressAuditStream(t *testing.T) {
	gateway, _ := newCredentialTestGateway(t)
	inst := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: "7", ClaudeAccount: "user_claude", ClaudeEnvironment: "env", ClaudeSession: "session"}}
	inst.Name, inst.Namespace = "acr-test", system.DefaultNamespace
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(inst).Build()
	processor := &AuditProcessor{Provider: &CredentialProvider{Gateway: gateway, Storage: storage}}
	actor := substrate.Ref(inst)
	attrs, err := structpb.NewStruct(map[string]any{"filter_state['dev.ate.actor.identity']": "spiffe://substrate-actor.local/ateom-for-actor/" + actor.Atespace + "/" + actor.Name})
	require.NoError(t, err)
	first := &ext.ProcessingRequest{Attributes: map[string]*structpb.Struct{"audit": attrs}, Request: &ext.ProcessingRequest_RequestHeaders{RequestHeaders: auditHeaders(map[string]string{":method": "POST", ":authority": "example.com", ":scheme": "https", ":path": "/api?key=secret", "authorization": "Bearer secret"})}}
	messages := []*ext.ProcessingRequest{
		first,
		{Request: &ext.ProcessingRequest_RequestBody{RequestBody: &ext.HttpBody{Body: []byte("request"), EndOfStream: true}}},
		{Request: &ext.ProcessingRequest_ResponseHeaders{ResponseHeaders: auditHeaders(map[string]string{":status": "200", "set-cookie": "private"})}},
		{Request: &ext.ProcessingRequest_ResponseBody{ResponseBody: &ext.HttpBody{Body: []byte(strings.Repeat("a", egressBodyLimit+1)), EndOfStream: true}}},
	}
	stream := &auditStream{ctx: auditContext(t), messages: messages}
	stream.onSend = func() {
		_, total, err := gateway.ListEgressAuditLogs(t.Context(), gclient.EgressAuditLogOptions{Limit: 1})
		require.NoError(t, err)
		require.Zero(t, total, "body chunks must not write to the database")
	}
	require.NoError(t, processor.Process(stream))
	require.NoError(t, gateway.FlushEgressAuditLogs())
	require.Len(t, stream.sent, 4)
	entries, _, err := gateway.ListEgressAuditLogs(t.Context(), gclient.EgressAuditLogOptions{Limit: 1})
	require.NoError(t, err)
	entry, err := gateway.GetEgressAuditLog(t.Context(), entries[0].ID, true)
	require.NoError(t, err)
	require.Equal(t, "complete", entry.Outcome)
	require.True(t, entry.ResponseTruncated)
	require.EqualValues(t, egressBodyLimit+1, entry.ResponseBytes)
	require.NotContains(t, string(entry.RequestHeaders), "secret")
	require.NotContains(t, string(entry.ResponseHeaders), "private")
	require.NotContains(t, entry.Path, "secret")
	var body []byte
	require.NoError(t, json.Unmarshal(entry.ResponseBody, &body))
	require.Len(t, body, egressBodyLimit)

	// Direct actor calls and forged identities cannot write audit records.
	require.Equal(t, codes.PermissionDenied, status.Code(processor.Process(&auditStream{ctx: t.Context(), messages: messages})))
	attrs.Fields["filter_state['dev.ate.actor.identity']"] = structpb.NewStringValue("spiffe://substrate-actor.local/ateom-for-actor/other/acr-test")
	require.Equal(t, codes.PermissionDenied, status.Code(processor.Process(&auditStream{ctx: auditContext(t), messages: []*ext.ProcessingRequest{first}})))
}

func TestEgressAuditInterruptedResponse(t *testing.T) {
	gateway, _ := newCredentialTestGateway(t)
	inst := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: "7", ClaudeSession: "session"}}
	inst.Name, inst.Namespace = "acr-test", system.DefaultNamespace
	processor := &AuditProcessor{Provider: &CredentialProvider{Gateway: gateway, Storage: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(inst).Build()}}
	ref := substrate.Ref(inst)
	attrs, _ := structpb.NewStruct(map[string]any{"filter_state['dev.ate.actor.identity']": "spiffe://substrate-actor.local/ateom-for-actor/" + ref.Atespace + "/" + ref.Name})
	stream := &auditStream{ctx: auditContext(t), recvError: context.Canceled, messages: []*ext.ProcessingRequest{
		{Attributes: map[string]*structpb.Struct{"audit": attrs}, Request: &ext.ProcessingRequest_RequestHeaders{RequestHeaders: auditHeaders(map[string]string{":method": "GET", ":path": "/"})}},
		{Request: &ext.ProcessingRequest_ResponseHeaders{ResponseHeaders: auditHeaders(map[string]string{":status": "200"})}},
		{Request: &ext.ProcessingRequest_ResponseBody{ResponseBody: &ext.HttpBody{Body: []byte("partial")}}},
	}}
	require.ErrorIs(t, processor.Process(stream), context.Canceled)
	require.NoError(t, gateway.FlushEgressAuditLogs())
	entries, _, err := gateway.ListEgressAuditLogs(t.Context(), gclient.EgressAuditLogOptions{Limit: 1})
	require.NoError(t, err)
	entry, err := gateway.GetEgressAuditLog(t.Context(), entries[0].ID, true)
	require.NoError(t, err)
	require.Equal(t, "interrupted", entry.Outcome)
	var body []byte
	require.NoError(t, json.Unmarshal(entry.ResponseBody, &body))
	require.Equal(t, "partial", string(body))
}
