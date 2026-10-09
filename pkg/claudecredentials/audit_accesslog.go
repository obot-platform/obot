package claudecredentials

import (
	"io"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"

	"google.golang.org/protobuf/proto"

	als "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
)

// AccessLogger supplements buffered capture with Envoy's final status, including local denials.
type AccessLogger struct {
	als.UnimplementedAccessLogServiceServer
	Provider *CredentialProvider
}

func (p *AccessLogger) StreamAccessLogs(stream als.AccessLogService_StreamAccessLogsServer) error {
	if err := verifyEgressPeer(stream.Context()); err != nil {
		return err
	}
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&als.StreamAccessLogsResponse{})
		}
		if err != nil {
			return err
		}
		for _, event := range batch.GetHttpLogs().GetLogEntry() {
			meta := event.GetCommonProperties().GetMetadata().GetFilterMetadata()["obot.audit"]
			id := meta.GetFields()["id"].GetStringValue()
			if id == "" {
				continue
			}
			code := int(event.GetResponse().GetResponseCode().GetValue())
			duration := event.GetCommonProperties().GetTimeToLastDownstreamTxByte().AsDuration().Milliseconds()
			outcome := egressOutcome(code)
			if code < 400 && proto.Size(event.GetCommonProperties().GetResponseFlags()) > 0 {
				outcome = "interrupted"
			}
			p.Provider.Gateway.LogEgressAuditEntry(types.EgressAuditLog{
				ID:             id,
				CreatedAt:      time.Now().UTC(),
				ResponseStatus: code,
				DurationMs:     duration,
				Outcome:        outcome,
				Finalized:      true,
			})
		}
	}
}
