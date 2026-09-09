package producttelemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

type consentReaderFunc func(context.Context) (*bool, error)
type reportSenderFunc func(context.Context, clienttypes.ProductTelemetryRequest) error

func (f consentReaderFunc) Get(ctx context.Context) (*bool, error) {
	return f(ctx)
}

func (f reportSenderFunc) Send(ctx context.Context, report clienttypes.ProductTelemetryRequest) error {
	return f(ctx, report)
}

func TestNextDailyReportTime(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
	}{
		{
			name: "local time uses next UTC day",
			now:  time.Date(2026, time.September, 2, 17, 4, 0, 0, time.FixedZone("local", -7*60*60)),
		},
		{
			name: "before midnight uses next UTC day",
			now:  time.Date(2026, time.September, 2, 23, 59, 0, 0, time.UTC),
		},
		{
			name: "after midnight uses following UTC day",
			now:  time.Date(2026, time.September, 3, 18, 0, 0, 0, time.UTC),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			start := testCase.now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
			end := start.Add(dailyReportWindow)
			if got := nextDailyReportTime(testCase.now); got.Before(start) || got.After(end) {
				t.Fatalf("nextDailyReportTime(%v) = %v, want between %v and %v", testCase.now, got, start, end)
			}
		})
	}
}

func TestNewPublisherNormalizesEngine(t *testing.T) {
	publisher := newPublisher(nil, nil, nil, nil, "k8s", nil)
	if publisher.engine != mcp.RuntimeBackendKubernetes {
		t.Fatalf("engine = %q, want %q", publisher.engine, mcp.RuntimeBackendKubernetes)
	}
}

func TestPublisherReadsConsentBeforeEveryRun(t *testing.T) {
	values := []*bool{nil, new(false), new(true)}
	var consentCalls, sendCalls int
	gatewayClient := newRequestGateway()
	publisher := newPublisher(
		consentReaderFunc(func(context.Context) (*bool, error) {
			value := values[consentCalls]
			consentCalls++
			return value, nil
		}),
		gatewayClient,
		testStorageClient(),
		testEntitlements(),
		"docker",
		reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
			sendCalls++
			return nil
		}),
	)

	for range values {
		publisher.runOnce(t.Context())
	}
	if consentCalls != 3 || gatewayClient.propertyCalls != 2 || sendCalls != 1 {
		t.Fatalf("calls = consent:%d properties:%d send:%d, want 3,2,1", consentCalls, gatewayClient.propertyCalls, sendCalls)
	}
}

func TestPublisherForceEnabledSendsReport(t *testing.T) {
	var got clienttypes.ProductTelemetryRequest
	publisher := newPublisher(
		NewConsent(nil, true),
		newRequestGateway(),
		testStorageClient(),
		testEntitlements(),
		"docker",
		reportSenderFunc(func(_ context.Context, report clienttypes.ProductTelemetryRequest) error {
			got = report
			return nil
		}),
	)

	publisher.runOnce(t.Context())
	if got.InstallationID != "installation-id" {
		t.Fatalf("installation ID = %q, want installation-id", got.InstallationID)
	}
}

func TestPublisherFailuresAreNonFatal(t *testing.T) {
	tests := []struct {
		name          string
		consent       consentReader
		gatewayClient requestGatewayClient
		sender        reportSender
		wantSends     int
	}{
		{
			name: "consent failure",
			consent: consentReaderFunc(func(context.Context) (*bool, error) {
				return nil, errors.New("consent unavailable")
			}),
			gatewayClient: newRequestGateway(),
			sender: reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
				t.Fatal("sender called after consent failure")
				return nil
			}),
		},
		{
			name:    "builder failure",
			consent: consentReaderFunc(func(context.Context) (*bool, error) { return new(true), nil }),
			gatewayClient: func() requestGatewayClient {
				gateway := newRequestGateway()
				gateway.propertyErrors = map[string]error{"installation_id": errors.New("database unavailable")}
				return gateway
			}(),
			sender: reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
				t.Fatal("sender called after builder failure")
				return nil
			}),
		},
		{
			name:          "sender failure",
			consent:       consentReaderFunc(func(context.Context) (*bool, error) { return new(true), nil }),
			gatewayClient: newRequestGateway(),
			sender: reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
				return errors.New("delivery unavailable")
			}),
			wantSends: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var sends int
			sender := reportSenderFunc(func(ctx context.Context, report clienttypes.ProductTelemetryRequest) error {
				sends++
				return testCase.sender.Send(ctx, report)
			})
			publisher := newPublisher(testCase.consent, testCase.gatewayClient, testStorageClient(), testEntitlements(), "docker", sender)
			publisher.runOnce(t.Context())
			if sends != testCase.wantSends {
				t.Fatalf("sender calls = %d, want %d", sends, testCase.wantSends)
			}
		})
	}
}

func TestPublisherDoesNotStartForDevelopmentVersion(t *testing.T) {
	publisher := newPublisher(
		consentReaderFunc(func(context.Context) (*bool, error) {
			t.Fatal("consent read for development version")
			return nil, nil
		}),
		nil,
		nil,
		nil,
		"docker",
		reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
			t.Fatal("telemetry sent for development version")
			return nil
		}),
	)

	publisher.start(t.Context(), "v0.0.0-dev", false)

	select {
	case <-publisher.done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not stop")
	}
}

func TestPublisherDevelopmentVersionCanBeForcedWithoutBypassingConsent(t *testing.T) {
	consentRead := make(chan struct{})
	publisher := newPublisher(
		consentReaderFunc(func(context.Context) (*bool, error) {
			close(consentRead)
			return new(false), nil
		}),
		nil,
		nil,
		nil,
		"docker",
		reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
			t.Fatal("telemetry sent without consent")
			return nil
		}),
	)
	ctx, cancel := context.WithCancel(t.Context())
	publisher.start(ctx, "v0.0.0-dev", true)

	select {
	case <-consentRead:
	case <-time.After(time.Second):
		t.Fatal("forced publisher did not read consent")
	}
	cancel()
	select {
	case <-publisher.done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not stop after context cancellation")
	}
}

func TestPublisherStartsForReleaseVersion(t *testing.T) {
	started := make(chan struct{})
	publisher := newPublisher(
		consentReaderFunc(func(context.Context) (*bool, error) { return new(true), nil }),
		newRequestGateway(),
		testStorageClient(),
		testEntitlements(),
		"docker",
		reportSenderFunc(func(context.Context, clienttypes.ProductTelemetryRequest) error {
			close(started)
			return nil
		}),
	)
	ctx, cancel := context.WithCancel(t.Context())
	publisher.start(ctx, "v1.2.3", false)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("release publisher did not start")
	}
	cancel()
	select {
	case <-publisher.done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not stop after context cancellation")
	}
}

func TestNewPublisherStartsImmediatelyAndWaitHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(started)
		response.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OBOT_UPGRADE_SERVER_URL", server.URL)
	t.Setenv("OBOT_FORCE_PRODUCT_TELEMETRY", "true")

	gatewayClient := newConsentTestGatewayClient(t)
	consent := NewConsent(gatewayClient, false)
	if err := consent.Set(t.Context(), true); err != nil {
		t.Fatalf("enable consent: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	publisher := NewPublisher(ctx, consent, gatewayClient, testStorageClient(), testEntitlements(), "docker")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background job did not start")
	}
	cancel()
	select {
	case <-publisher.done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not stop after context cancellation")
	}
}
