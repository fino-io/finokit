package nats

import (
	"context"
	"strings"

	gonats "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName identifies NATS messaging instrumentation in OpenTelemetry data.
const ScopeName = "github.com/fino-io/finokit/messaging/nats"

var natsPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

var tracer = otel.Tracer(ScopeName)

func startProducer(ctx context.Context, topic string) (context.Context, trace.Span) {
	return startSpan(ctx, "publish", topic, trace.SpanKindProducer)
}

func startConsumer(ctx context.Context, topic string) (context.Context, trace.Span) {
	return startSpan(ctx, "process", topic, trace.SpanKindConsumer)
}

func startSpan(ctx context.Context, operation, topic string, kind trace.SpanKind) (context.Context, trace.Span) {
	topic = telemetryString(topic)
	return tracer.Start(ctx, operation+" "+topic,
		trace.WithSpanKind(kind),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", topic),
		),
	)
}

func endSpan(span trace.Span, err error) {
	if err != nil {
		recordSpanError(span, err)
	}
	span.End()
}

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	// Keep the original error for business logic; only the telemetry copy is sanitized.
	rawMessage := err.Error()
	message := telemetryString(rawMessage)
	recordedErr := err
	if message != rawMessage {
		recordedErr = sanitizedError{cause: err, message: message}
	}

	span.RecordError(recordedErr)
	span.SetStatus(codes.Error, message)
}

func telemetryString(value string) string {
	return strings.ToValidUTF8(value, "\uFFFD")
}

type sanitizedError struct {
	cause   error
	message string
}

func (e sanitizedError) Error() string {
	return e.message
}

func (e sanitizedError) Unwrap() error {
	return e.cause
}

func injectContext(ctx context.Context, header gonats.Header) {
	natsPropagator.Inject(ctx, headerCarrier(header))
}

func extractContext(ctx context.Context, header gonats.Header) context.Context {
	return natsPropagator.Extract(ctx, headerCarrier(header))
}

type headerCarrier gonats.Header

func (h headerCarrier) Get(key string) string {
	return gonats.Header(h).Get(key)
}

func (h headerCarrier) Set(key, value string) {
	gonats.Header(h).Set(key, value)
}

func (h headerCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		keys = append(keys, key)
	}
	return keys
}
