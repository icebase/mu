// Package tracing configures OpenTelemetry tracing for mu.
//
// Tracing is disabled by default. When disabled the global OpenTelemetry tracer provider stays the
// no-op default, so instrumented code costs (almost) nothing.
package tracing

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// InstrumentationName is the tracer name used for mu spans.
	InstrumentationName = "github.com/icebase/mu"

	// ExporterOTLP exports spans to an OTLP/gRPC collector (default).
	ExporterOTLP = "otlp"
	// ExporterStdout prints finished spans as JSON to stderr; useful for debugging without a collector.
	ExporterStdout = "stdout"

	defaultEndpoint = "localhost:4317"
)

// stdoutWriter is replaced in tests.
var stdoutWriter io.Writer = os.Stderr

// Config defines the tracing configuration.
type Config struct {
	Enabled     bool    `json:"enabled" yaml:"enabled"`
	ServiceName string  `json:"service_name" yaml:"service_name"`
	Exporter    string  `json:"exporter" yaml:"exporter"`       // "otlp" (default) or "stdout"
	Endpoint    string  `json:"endpoint" yaml:"endpoint"`       // host:port or URL; empty defers to OTEL_EXPORTER_OTLP_* variables
	Insecure    bool    `json:"insecure" yaml:"insecure"`       // plaintext gRPC for host:port endpoints
	SampleRate  float64 `json:"sample_rate" yaml:"sample_rate"` // 0.0-1.0, parent-based

	// ServiceVersion and NodeID are exported as resource attributes (service.version, mu.node_id).
	ServiceVersion string `json:"service_version" yaml:"service_version"`
	NodeID         string `json:"node_id" yaml:"node_id"`
}

// Validate reports configuration errors that would otherwise surface as confusing runtime behavior.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if math.IsNaN(c.SampleRate) || c.SampleRate < 0 || c.SampleRate > 1 {
		return fmt.Errorf("tracing sample rate %v must be between 0 and 1", c.SampleRate)
	}
	switch c.exporter() {
	case ExporterOTLP, ExporterStdout:
	default:
		return fmt.Errorf("unsupported tracing exporter %q (use %q or %q)", c.Exporter, ExporterOTLP, ExporterStdout)
	}
	return nil
}

func (c Config) exporter() string {
	if exporter := strings.ToLower(strings.TrimSpace(c.Exporter)); exporter != "" {
		return exporter
	}
	return ExporterOTLP
}

// InitTracer configures the global tracer provider and propagators.
// It returns a nil provider when tracing is disabled.
func InitTracer(ctx context.Context, config Config) (*sdktrace.TracerProvider, error) {
	if !config.Enabled {
		return nil, nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	res, err := newResource(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.SampleRate))),
	}
	switch config.exporter() {
	case ExporterStdout:
		exporter, err := stdouttrace.New(stdouttrace.WithWriter(stdoutWriter))
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
		// Synchronous export so spans appear immediately while debugging.
		options = append(options, sdktrace.WithSyncer(exporter))
	default:
		exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlpOptions(config)...))
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	tp := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("OpenTelemetry error", "error", err)
	}))
	return tp, nil
}

// newResource intentionally avoids resource.Default(): it carries a different semconv schema URL
// than any pinned semconv package and Merge would fail. OTEL_RESOURCE_ATTRIBUTES and
// OTEL_SERVICE_NAME are still honored through WithFromEnv; explicit config wins for service.name.
func newResource(ctx context.Context, config Config) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if config.ServiceName != "" {
		attrs = append(attrs, attribute.String("service.name", config.ServiceName))
	}
	if config.ServiceVersion != "" {
		attrs = append(attrs, attribute.String("service.version", config.ServiceVersion))
	}
	if config.NodeID != "" {
		attrs = append(attrs, attribute.String("mu.node_id", config.NodeID))
	}
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessPID(),
		resource.WithAttributes(attrs...),
	)
}

func otlpOptions(config Config) []otlptracegrpc.Option {
	endpoint := strings.TrimSpace(config.Endpoint)
	switch {
	case strings.Contains(endpoint, "://"):
		// The URL scheme (http/https) selects plaintext or TLS.
		return []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	case endpoint == "" && (os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""):
		// Let the SDK read the standard OTEL_EXPORTER_OTLP_* variables (endpoint, headers, TLS, ...).
		return nil
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return opts
}

// Shutdown flushes and stops the tracer provider. A nil provider is a no-op.
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	// Bound the final export so a dead collector cannot block process exit.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracer provider: %w", err)
	}
	return nil
}

// Tracer returns the mu tracer from the global provider.
func Tracer() trace.Tracer { return otel.Tracer(InstrumentationName) }

// StartSpan starts a span with the mu tracer.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, opts...)
}

// RecordError records err on the span in ctx and marks the span as failed.
func RecordError(ctx context.Context, err error, description string) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, trace.WithAttributes(attribute.String("error.description", description)))
	span.SetStatus(codes.Error, description)
}

// SetAttributes sets attributes on the span in ctx.
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// DefaultConfig returns the default (disabled) configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		ServiceName: "mu-service",
		Exporter:    ExporterOTLP,
		Insecure:    true,
		SampleRate:  1.0,
	}
}
