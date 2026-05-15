// Package tracing wires the OpenTelemetry SDK into Gun. By default
// (no environment configuration), every span call resolves to a no-op
// — the cost is a single method dispatch through the global tracer
// provider, ~10 ns per call — so leaving the tracing wired in the hot
// path is fine for runtime cost even at 2M ops/sec/symbol.
//
// To enable real export, set:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
//	GUN_TRACE_SAMPLE_RATIO=0.001            # 0..1, default 0.001
//
// Spans flow as gRPC OTLP. With the default 0.1% sample ratio, a fully
// saturated 16-core engine produces ~2000 sampled spans/sec, well
// within what a single collector instance can absorb.
package tracing

import (
	"context"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName       = "github.com/aliraad79/Gun"
	defaultSampleRatio = 0.001 // 0.1% — bounded cost at 2M ops/sec/core
)

// ShutdownFunc flushes any pending spans and releases exporter
// resources. Always non-nil, even when tracing is disabled (in which
// case it is a no-op).
type ShutdownFunc func(ctx context.Context) error

// Init configures the global TracerProvider. When OTEL_EXPORTER_OTLP_ENDPOINT
// is set, traces are exported over gRPC. Otherwise the global provider
// stays at the SDK default (no-op).
//
// Returns a ShutdownFunc to call at process exit. Errors here are
// non-fatal in production — tracing should never block startup.
func Init(ctx context.Context) (ShutdownFunc, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return noopShutdown, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return noopShutdown, err
	}

	ratio := defaultSampleRatio
	if v := os.Getenv("GUN_TRACE_SAMPLE_RATIO"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 && parsed <= 1 {
			ratio = parsed
		}
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName("gun"),
			semconv.ServiceVersion("phase-4"),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

func noopShutdown(context.Context) error { return nil }

// tracer is resolved lazily so that Init (called from main) wins the
// race against any import-time span creation.
func tracer() trace.Tracer { return otel.Tracer(tracerName) }

// Start begins a span. The returned context carries the span so child
// spans can attach. When tracing is disabled the span is a no-op and
// End() is essentially free.
func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// StringAttr is a thin alias so callers don't have to import the otel
// attribute package directly.
func StringAttr(k, v string) attribute.KeyValue { return attribute.String(k, v) }

// Int64Attr is the int64 counterpart of StringAttr.
func Int64Attr(k string, v int64) attribute.KeyValue { return attribute.Int64(k, v) }
