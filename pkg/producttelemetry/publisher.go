package producttelemetry

import (
	"context"
	"errors"
	"log/slog"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/upgrade"
)

const (
	dailyReportOffset = 5 * time.Minute
)

type consentReader interface {
	Get(context.Context) (*bool, error)
}

type reportSender interface {
	Send(context.Context, clienttypes.ProductTelemetryRequest) error
}

// Publisher collects and sends product telemetry at startup and at a fixed daily UTC time.
type Publisher struct {
	consent       consentReader
	gatewayClient installationPropertyClient
	sender        reportSender
	done          chan struct{}
}

// NewPublisher creates and immediately starts a product telemetry publisher.
func NewPublisher(ctx context.Context, consent *Consent, gatewayClient *gatewayclient.Client) *Publisher {
	publisher := newPublisher(
		consent,
		gatewayClient,
		NewClient(upgrade.ServerBaseURL(), nil),
	)
	go publisher.run(ctx)
	return publisher
}

func newPublisher(consent consentReader, gatewayClient installationPropertyClient, sender reportSender) *Publisher {
	return &Publisher{
		consent:       consent,
		gatewayClient: gatewayClient,
		sender:        sender,
		done:          make(chan struct{}),
	}
}

func (p *Publisher) run(ctx context.Context) {
	defer close(p.done)
	p.runOnce(ctx)

	for {
		now := time.Now()
		delay := nextDailyReportTime(now).Sub(now)
		if err := wait(ctx, delay); err != nil {
			return
		}
		p.runOnce(ctx)
	}
}

func (p *Publisher) runOnce(ctx context.Context) {
	consent, err := p.consent.Get(ctx)
	if err != nil {
		logJobError(ctx, "failed to read product telemetry consent", err)
		return
	}
	if consent == nil || !*consent {
		return
	}

	report, err := buildRequest(ctx, p.gatewayClient)
	if err != nil {
		logJobError(ctx, "failed to build product telemetry report", err)
		return
	}
	if err := p.sender.Send(ctx, report); err != nil {
		logJobError(ctx, "failed to send product telemetry report", err)
	}
}

func nextDailyReportTime(now time.Time) time.Time {
	now = now.UTC()
	next := now.Truncate(24 * time.Hour).Add(dailyReportOffset)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func logJobError(ctx context.Context, message string, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}
	slog.Warn(message, "error", err)
}
