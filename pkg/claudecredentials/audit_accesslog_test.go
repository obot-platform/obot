package claudecredentials

import (
	"context"
	"io"
	"testing"
	"time"

	logpb "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	als "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type accessLogStream struct {
	grpc.ServerStream
	ctx   context.Context
	batch *als.StreamAccessLogsMessage
}

func (s *accessLogStream) Context() context.Context { return s.ctx }
func (s *accessLogStream) Recv() (*als.StreamAccessLogsMessage, error) {
	if s.batch == nil {
		return nil, io.EOF
	}
	batch := s.batch
	s.batch = nil
	return batch, nil
}
func (s *accessLogStream) SendAndClose(*als.StreamAccessLogsResponse) error { return nil }

func TestAuditTLSHostnameDenial(t *testing.T) {
	gateway, _ := newCredentialTestGateway(t)
	inst := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{
		UserID:            "7",
		ClaudeSession:     "session",
		ClaudeEnvironment: "environment",
	}}
	inst.Name, inst.Namespace = "acr-test", system.DefaultNamespace
	logger := &AccessLogger{Provider: &CredentialProvider{
		Gateway: gateway,
		Storage: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(inst).Build(),
	}}
	ref := substrate.Ref(inst)
	identity, err := anypb.New(wrapperspb.String("spiffe://substrate-actor.local/ateom-for-actor/" + ref.Atespace + "/" + ref.Name))
	require.NoError(t, err)
	deleted, err := anypb.New(wrapperspb.String("spiffe://substrate-actor.local/ateom-for-actor/" + ref.Atespace + "/deleted"))
	require.NoError(t, err)
	started := time.Now().UTC().Truncate(time.Microsecond)
	batch := &als.StreamAccessLogsMessage{LogEntries: &als.StreamAccessLogsMessage_TcpLogs{
		TcpLogs: &als.StreamAccessLogsMessage_TCPAccessLogEntries{LogEntry: []*logpb.TCPAccessLogEntry{
			// A denial that cannot be attributed is skipped without dropping later entries.
			{CommonProperties: &logpb.AccessLogCommon{
				TlsProperties:      &logpb.TLSProperties{TlsSniHostname: "deleted.example.com"},
				ResponseFlags:      &logpb.ResponseFlags{NoRouteFound: true},
				FilterStateObjects: map[string]*anypb.Any{"dev.ate.actor.identity": deleted},
			}},
			{CommonProperties: &logpb.AccessLogCommon{
				StartTime:          timestamppb.New(started),
				Duration:           durationpb.New(25 * time.Millisecond),
				TlsProperties:      &logpb.TLSProperties{TlsSniHostname: "blocked.example.com"},
				ResponseFlags:      &logpb.ResponseFlags{NoRouteFound: true},
				FilterStateObjects: map[string]*anypb.Any{"dev.ate.actor.identity": identity},
			}},
			// Missing identity, successful connections, and inspector timeouts are not hostname denials.
			{CommonProperties: &logpb.AccessLogCommon{
				TlsProperties: &logpb.TLSProperties{TlsSniHostname: "unattributed.example.com"},
				ResponseFlags: &logpb.ResponseFlags{NoRouteFound: true},
			}},
			{CommonProperties: &logpb.AccessLogCommon{
				TlsProperties:      &logpb.TLSProperties{TlsSniHostname: "allowed.example.com"},
				FilterStateObjects: map[string]*anypb.Any{"dev.ate.actor.identity": identity},
			}},
			{CommonProperties: &logpb.AccessLogCommon{
				ResponseFlags:      &logpb.ResponseFlags{NoRouteFound: true},
				FilterStateObjects: map[string]*anypb.Any{"dev.ate.actor.identity": identity},
			}},
		}},
	}}
	require.Equal(t, codes.PermissionDenied, status.Code(logger.StreamAccessLogs(&accessLogStream{ctx: t.Context(), batch: batch})))
	require.NoError(t, logger.StreamAccessLogs(&accessLogStream{ctx: auditContext(t), batch: batch}))
	require.NoError(t, gateway.FlushEgressAuditLogs())
	entries, total, err := gateway.ListEgressAuditLogs(t.Context(), gclient.EgressAuditLogOptions{Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "blocked.example.com", entry.Host)
	require.Equal(t, "denied", entry.Outcome)
	require.Equal(t, "tls", entry.Scheme)
	require.Equal(t, "7", entry.UserID)
	require.Equal(t, "session", entry.ClaudeSession)
	require.Equal(t, "environment", entry.EnvironmentID)
	require.EqualValues(t, 25, entry.DurationMs)
	require.True(t, entry.CreatedAt.Equal(started))
	require.Zero(t, entry.ResponseStatus)
	require.Empty(t, entry.Method)
	require.Empty(t, entry.Path)
}
