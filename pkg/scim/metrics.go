package scim

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterName = "github.com/obot-platform/obot/pkg/scim"
)

var (
	// knownResources are the resources whose requests are counted under their own name. Everything else is counted as
	// "other", so that request paths cannot grow the number of metric series.
	knownResources = []string{
		"Users",
		"Groups",
		"ServiceProviderConfig",
		"ResourceTypes",
		"Schemas",
	}

	requestCounter, requestDuration = newInstruments()
)

// newInstruments creates the SCIM request metrics. They are created from the global meter provider, which forwards
// them to the provider that the server installs at startup.
func newInstruments() (metric.Int64Counter, metric.Float64Histogram) {
	meter := otel.Meter(meterName)

	counter, err := meter.Int64Counter("obot.scim.requests",
		metric.WithDescription("SCIM requests handled, by resource, operation, status, and outcome"),
		metric.WithUnit("{request}"))
	if err != nil {
		slog.Warn("Failed to create the SCIM request counter", "error", err)
	}

	histogram, err := meter.Float64Histogram("obot.scim.request.duration",
		metric.WithDescription("Latency of SCIM requests, by resource and operation"),
		metric.WithUnit("s"))
	if err != nil {
		slog.Warn("Failed to create the SCIM request duration histogram", "error", err)
	}

	return counter, histogram
}

// describeRequest returns the resource and operation of a SCIM request, for metrics.
func describeRequest(method string, segments []string) (string, string) {
	resource := "other"
	if len(segments) > 0 {
		for _, known := range knownResources {
			if strings.EqualFold(segments[0], known) {
				resource = known
				break
			}
		}
	}

	switch resource {
	case "Users", "Groups":
	case "other":
		return resource, strings.ToLower(method)
	default:
		return resource, "discovery"
	}

	item := len(segments) > 1
	switch method {
	case http.MethodGet:
		if item {
			return resource, "get"
		}
		return resource, "list"
	case http.MethodPost:
		return resource, "create"
	case http.MethodPut:
		return resource, "replace"
	case http.MethodPatch:
		return resource, "patch"
	case http.MethodDelete:
		return resource, "delete"
	default:
		return resource, strings.ToLower(method)
	}
}

// recordMetrics records a handled request's outcome and latency.
func recordMetrics(ctx context.Context, resource, operation string, status int, duration time.Duration) {
	outcome := "success"
	switch {
	case status >= http.StatusInternalServerError:
		outcome = "error"
	case status >= http.StatusBadRequest:
		outcome = "failure"
	}

	// The request may have run out of time, but its metrics are still recorded.
	ctx = context.WithoutCancel(ctx)
	if requestCounter != nil {
		requestCounter.Add(ctx, 1, metric.WithAttributes(
			otelattribute.String("resource", resource),
			otelattribute.String("operation", operation),
			otelattribute.String("status", strconv.Itoa(status)),
			otelattribute.String("outcome", outcome),
		))
	}
	if requestDuration != nil {
		requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
			otelattribute.String("resource", resource),
			otelattribute.String("operation", operation),
		))
	}
}
