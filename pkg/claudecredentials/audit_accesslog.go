package claudecredentials

import (
	"io"
	"log/slog"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

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
		for _, event := range batch.GetTcpLogs().GetLogEntry() {
			common := event.GetCommonProperties()
			host := common.GetTlsProperties().GetTlsSniHostname()
			if !common.GetResponseFlags().GetNoRouteFound() || host == "" {
				continue
			}

			identity := &wrapperspb.StringValue{}
			value := common.GetFilterStateObjects()["dev.ate.actor.identity"]
			if value == nil || value.UnmarshalTo(identity) != nil {
				continue
			}

			// TLS was rejected before HTTP existed: do not invent a method, path, or status.
			entry := types.EgressAuditLog{
				ID:         uuid.New().String(),
				CreatedAt:  common.GetStartTime().AsTime(),
				Scheme:     "tls",
				Host:       host,
				DurationMs: common.GetDuration().AsDuration().Milliseconds(),
				Outcome:    "denied",
				Finalized:  true,
			}
			processor := AuditProcessor{Provider: p.Provider}
			if err := processor.attribute(stream.Context(), identity.Value, &entry); err != nil {
				// One unattributable denial, such as a deleted session, must not drop the rest of the batch.
				slog.WarnContext(stream.Context(), "Cannot attribute Claude egress TLS denial", "host", host, "error", err)
				continue
			}
			p.Provider.Gateway.LogEgressAuditEntry(entry)
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
