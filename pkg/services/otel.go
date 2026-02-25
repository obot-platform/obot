package services

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/obot-platform/obot/pkg/version"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Otel struct {
	shutdown []func(context.Context) error
}

func (s *Otel) Shutdown(ctx context.Context) error {
	var err error
	for _, fn := range s.shutdown {
		err = errors.Join(err, fn(ctx))
	}
	return err
}

// newOtel bootstraps the OpenTelemetry pipeline using the autoexport package.
// All configuration is driven by standard OTEL environment variables
// (e.g. OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_PROTOCOL,
// OTEL_TRACES_SAMPLER, OTEL_TRACES_SAMPLER_ARG, etc.).
// If it does not return an error, make sure to call Shutdown for proper cleanup.
func newOtel(ctx context.Context) (o *Otel, err error) {
	resource, err := resource.New(ctx, resource.WithAttributes(
		attribute.Key("service.name").String("obot"),
		attribute.Key("service.version").String(version.Get().String()),
	))
	if err != nil {
		return nil, err
	}

	o = new(Otel)
	defer func() {
		if err != nil {
			err = errors.Join(err, o.Shutdown(context.Background()))
		}
	}()

	exportEnabled := otelExportEnabled()

	// Set up propagator.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, resource, exportEnabled)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, resource, exportEnabled)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx, resource, exportEnabled)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

// otelExportEnabled returns true if any standard OTEL environment variable
// that implies exporting is set.
func otelExportEnabled() bool {
	for _, k := range []string{
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_LOGS_EXPORTER",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		if strings.TrimSpace(os.Getenv(k)) != "" {
			return true
		}
	}
	return false
}

func newTracerProvider(ctx context.Context, resource *resource.Resource, exportEnabled bool) (*trace.TracerProvider, error) {
	providerOpts := []trace.TracerProviderOption{trace.WithResource(resource)}

	if !exportEnabled {
		return trace.NewTracerProvider(providerOpts...), nil
	}

	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}
	if autoexport.IsNoneSpanExporter(exporter) {
		return trace.NewTracerProvider(providerOpts...), nil
	}

	providerOpts = append(providerOpts, trace.WithBatcher(exporter))
	return trace.NewTracerProvider(providerOpts...), nil
}

func newMeterProvider(ctx context.Context, resource *resource.Resource, exportEnabled bool) (*metric.MeterProvider, error) {
	providerOpts := []metric.Option{metric.WithResource(resource)}

	if !exportEnabled {
		return metric.NewMeterProvider(providerOpts...), nil
	}

	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, err
	}
	if autoexport.IsNoneMetricReader(reader) {
		return metric.NewMeterProvider(providerOpts...), nil
	}

	providerOpts = append(providerOpts, metric.WithReader(reader))
	return metric.NewMeterProvider(providerOpts...), nil
}

func newLoggerProvider(ctx context.Context, resource *resource.Resource, exportEnabled bool) (*log.LoggerProvider, error) {
	providerOpts := []log.LoggerProviderOption{log.WithResource(resource)}

	if !exportEnabled {
		return log.NewLoggerProvider(providerOpts...), nil
	}

	exporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, err
	}
	if autoexport.IsNoneLogExporter(exporter) {
		return log.NewLoggerProvider(providerOpts...), nil
	}

	providerOpts = append(providerOpts, log.WithProcessor(log.NewBatchProcessor(exporter)))
	return log.NewLoggerProvider(providerOpts...), nil
}
