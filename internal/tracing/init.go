package tracing

import (
	"context"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	AppName = "WeKnoraApp"
)

type Tracer struct {
	Cleanup func(context.Context) error
}

var tracer trace.Tracer

// InitTracer initializes OpenTelemetry tracer
func InitTracer() (*Tracer, error) {
	// Create resource description
	labels := []attribute.KeyValue{
		semconv.TelemetrySDKLanguageGo,
		semconv.ServiceNameKey.String(AppName),
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, labels...)
	var err error

	// First try to create OTLP exporter (can connect to Jaeger, Zipkin, Langfuse, etc.)
	var traceExporter sdktrace.SpanExporter
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		// Build gRPC client options
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		}
		// Support auth headers for services like Langfuse.
		// Format: "key1=val1,key2=val2" (standard OTEL_EXPORTER_OTLP_HEADERS)
		if headers := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); headers != "" {
			headerMap := parseOTLPHeaders(headers)
			if len(headerMap) > 0 {
				opts = append(opts, otlptracegrpc.WithHeaders(headerMap))
			}
		}
		client := otlptracegrpc.NewClient(opts...)
		traceExporter, err = otlptrace.New(context.Background(), client)
		if err != nil {
			return nil, err
		}
	} else {
		// If no OTLP endpoint is set, default to standard output
		traceExporter, err = stdouttrace.New()
		if err != nil {
			return nil, err
		}
	}

	// Create batch SpanProcessor
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

	sampler := sdktrace.AlwaysSample()

	// Create and register TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create Tracer for project use
	tracer = tp.Tracer(AppName)

	// Return cleanup function
	return &Tracer{
		Cleanup: func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Printf("Error shutting down tracer provider: %v", err)
				return err
			}
			return nil
		},
	}, nil
}

// GetTracer gets global Tracer
func GetTracer() trace.Tracer {
	return tracer
}

// noopTracer is used when the global tracer has not been initialised.
var noopTracer = trace.NewNoopTracerProvider().Tracer("")

// Create context with span. Returns a no-op span when the tracer has not been
// initialised (e.g. in unit tests).
func ContextWithSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	t := GetTracer()
	if t == nil {
		t = noopTracer
	}
	return t.Start(ctx, name, opts...)
}

// parseOTLPHeaders parses the OTEL_EXPORTER_OTLP_HEADERS format "k1=v1,k2=v2".
func parseOTLPHeaders(raw string) map[string]string {
	m := make(map[string]string)
	for _, pair := range splitUnescaped(raw, ',') {
		k, v, ok := cutString(pair, "=")
		if ok && k != "" {
			m[k] = v
		}
	}
	return m
}

func splitUnescaped(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func cutString(s, sep string) (string, string, bool) {
	i := 0
	for i < len(s) {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
		i++
	}
	return s, "", false
}
