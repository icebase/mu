package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config 定义 tracing 配置
type Config struct {
	Enabled     bool    `json:"enabled" yaml:"enabled"`
	ServiceName string  `json:"service_name" yaml:"service_name"`
	Endpoint    string  `json:"endpoint" yaml:"endpoint"`       // OTLP endpoint, e.g., "localhost:4317"
	Insecure    bool    `json:"insecure" yaml:"insecure"`       // 是否使用不安全连接
	SampleRate  float64 `json:"sample_rate" yaml:"sample_rate"` // 采样率 0.0-1.0
}

// InitTracer 初始化 OpenTelemetry tracer
func InitTracer(ctx context.Context, config Config) (*sdktrace.TracerProvider, error) {
	if !config.Enabled {
		// 如果未启用，返回一个 noop tracer provider
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, nil
	}

	// 创建 resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion("v0.0.1"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 创建 OTLP exporter
	var opts []otlptracegrpc.Option

	if config.Endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(config.Endpoint))
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		))
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	client := otlptracegrpc.NewClient(opts...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// 创建 sampler
	sampler := sdktrace.TraceIDRatioBased(config.SampleRate)

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局 Propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// Shutdown 关闭 tracer provider
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	// 给予 10 秒的时间来完成导出
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := tp.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown tracer provider: %w", err)
	}

	return nil
}

// StartSpan 创建一个新的 span，这是一个便利函数
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("mu-app")
	return tracer.Start(ctx, name, opts...)
}

// RecordError 记录错误到当前 span
func RecordError(ctx context.Context, err error, description string) {
	span := trace.SpanFromContext(ctx)
	if span != nil && err != nil {
		span.RecordError(err, trace.WithAttributes(
			attribute.String("error.description", description),
		))
		span.SetStatus(codes.Error, description)
	}
}

// SetAttributes 设置 span 属性
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetAttributes(attrs...)
	}
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		ServiceName: "mu-service",
		Endpoint:    "localhost:4317",
		Insecure:    true,
		SampleRate:  1.0,
	}
}
