package tracing

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func restoreGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})
}

func TestDisabledTracingKeepsNoopProvider(t *testing.T) {
	restoreGlobals(t)
	otel.SetTracerProvider(noop.NewTracerProvider())
	before := otel.GetTracerProvider()
	tp, err := InitTracer(context.Background(), DefaultConfig())
	if err != nil || tp != nil {
		t.Fatalf("disabled tracing must be a no-op, got provider=%v err=%v", tp, err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("disabled tracing replaced the global provider")
	}
	if err := Shutdown(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		config  Config
		wantErr bool
	}{
		"disabled ignores bad values": {Config{SampleRate: 7, Exporter: "bogus"}, false},
		"valid":                       {Config{Enabled: true, SampleRate: 0.5}, false},
		"stdout":                      {Config{Enabled: true, SampleRate: 1, Exporter: "STDOUT"}, false},
		"rate above one":              {Config{Enabled: true, SampleRate: 1.5}, true},
		"negative rate":               {Config{Enabled: true, SampleRate: -0.1}, true},
		"unknown exporter":            {Config{Enabled: true, SampleRate: 1, Exporter: "jaeger"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.config.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// Regression: enabling tracing used to fail at startup with "conflicting Schema URL".
func TestStdoutExporterEmitsSpansWithResource(t *testing.T) {
	restoreGlobals(t)
	var out bytes.Buffer
	stdoutWriter = &out
	t.Cleanup(func() { stdoutWriter = os.Stderr })
	config := DefaultConfig()
	config.Enabled, config.Exporter = true, ExporterStdout
	config.ServiceVersion, config.NodeID = "v1.2.3", "node-7"
	tp, err := InitTracer(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := StartSpan(context.Background(), "unit.test", trace.WithAttributes(attribute.Int("n", 1)))
	RecordError(ctx, context.DeadlineExceeded, "boom")
	span.End()
	if err := Shutdown(context.Background(), tp); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{`"Name":"unit.test"`, "mu-service", "v1.2.3", "node-7", "boom"} {
		if !strings.Contains(text, want) {
			t.Errorf("exported span is missing %q:\n%s", want, text)
		}
	}
}

func TestOTLPInitDoesNotBlockWithoutCollector(t *testing.T) {
	restoreGlobals(t)
	config := DefaultConfig()
	config.Enabled = true
	tp, err := InitTracer(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if err := Shutdown(context.Background(), tp); err != nil {
		t.Fatal(err)
	}
}

func TestPropagatorIsConfigured(t *testing.T) {
	restoreGlobals(t)
	config := DefaultConfig()
	config.Enabled, config.Exporter = true, ExporterStdout
	stdoutWriter = &bytes.Buffer{}
	t.Cleanup(func() { stdoutWriter = os.Stderr })
	tp, err := InitTracer(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Shutdown(context.Background(), tp) }()
	ctx, span := StartSpan(context.Background(), "parent")
	defer span.End()
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" {
		t.Fatal("traceparent header was not injected")
	}
}
