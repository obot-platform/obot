package services

import (
	"context"
	"errors"

	"github.com/obot-platform/obot/pkg/version"
	"go.opentelemetry.io/contrib/samplers/probability/consistent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

type OtelOptions struct {
	SampleProb         float64 `usage:"The probability of sampling a trace" default:"0.1" name:"otel-sample-prob"`
	BaseExportEndpoint string  `usage:"The base endpoint to export to, if not set, no metrics, tracing, or logging will be exported" name:"otel-base-export-endpoint"`
	BearerToken        string  `usage:"Bearer token for authentication" name:"otel-bearer-token"`
}

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

// newOtel bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func newOtel(ctx context.Context, opts OtelOptions) (o *Otel, err error) {
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

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, resource, opts)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, resource, opts)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx, resource, opts)
	if err != nil {
		return
	}
	o.shutdown = append(o.shutdown, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, resource *resource.Resource, cfg OtelOptions) (*trace.TracerProvider, error) {
	var (
		traceExporter trace.SpanExporter = (*dummyTraceExporter)(nil)
		err           error
	)
	if cfg.BaseExportEndpoint != "" {
		traceExporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.BaseExportEndpoint+"/v1/traces"), otlptracegrpc.WithHeaders(map[string]string{"Authorization": "Bearer " + cfg.BearerToken}))
		if err != nil {
			return nil, err
		}
	}

	return trace.NewTracerProvider(trace.WithSampler(trace.ParentBased(consistent.ProbabilityBased(cfg.SampleProb))), trace.WithBatcher(traceExporter), trace.WithResource(resource)), nil
}

func newMeterProvider(ctx context.Context, resource *resource.Resource, cfg OtelOptions) (*metric.MeterProvider, error) {
	var (
		metricExporter metric.Exporter = (*dummyMetricsExporter)(nil)
		err            error
	)
	if cfg.BaseExportEndpoint != "" {
		metricExporter, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.BaseExportEndpoint+"/v1/metrics"), otlpmetricgrpc.WithHeaders(map[string]string{"Authorization": "Bearer " + cfg.BearerToken}))
		if err != nil {
			return nil, err
		}
	}

	return metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(metricExporter)), metric.WithResource(resource)), nil
}

func newLoggerProvider(ctx context.Context, resource *resource.Resource, cfg OtelOptions) (*log.LoggerProvider, error) {
	var (
		logExporter log.Exporter = (*dummyLogExporter)(nil)
		err         error
	)
	if cfg.BaseExportEndpoint != "" {
		logExporter, err = otlploggrpc.New(ctx, otlploggrpc.WithEndpointURL(cfg.BaseExportEndpoint+"/v1/logs"), otlploggrpc.WithHeaders(map[string]string{"Authorization": "Bearer " + cfg.BearerToken}))
		if err != nil {
			return nil, err
		}
	}

	return log.NewLoggerProvider(log.WithProcessor(log.NewBatchProcessor(logExporter)), log.WithResource(resource)), nil
}

// What follows are dummy exporters for when OTel is not enabled.
// This allows the code to rename the same without worrying about whether metrics, tracing, or logging is enabled.

type dummyTraceExporter struct{}

// ExportSpans implements trace.SpanExporter.
func (d *dummyTraceExporter) ExportSpans(context.Context, []trace.ReadOnlySpan) error {
	return nil
}

// Shutdown implements trace.SpanExporter.
func (d *dummyTraceExporter) Shutdown(context.Context) error {
	return nil
}

type dummyMetricsExporter struct{}

// Aggregation implements metric.Exporter.
func (d *dummyMetricsExporter) Aggregation(metric.InstrumentKind) metric.Aggregation {
	return nil
}

// Export implements metric.Exporter.
func (d *dummyMetricsExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	return nil
}

// ForceFlush implements metric.Exporter.
func (d *dummyMetricsExporter) ForceFlush(context.Context) error {
	return nil
}

// Temporality implements metric.Exporter.
func (d *dummyMetricsExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return 0
}

// Shutdown implements metric.Exporter.
func (d *dummyMetricsExporter) Shutdown(context.Context) error {
	return nil
}

type dummyLogExporter struct{}

// Export implements log.Exporter.
func (d *dummyLogExporter) Export(context.Context, []log.Record) error {
	return nil
}

// ForceFlush implements log.Exporter.
func (d *dummyLogExporter) ForceFlush(context.Context) error {
	return nil
}

// Shutdown implements log.Exporter.
func (d *dummyLogExporter) Shutdown(context.Context) error {
	return nil
}
