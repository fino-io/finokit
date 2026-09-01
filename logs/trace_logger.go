package logs

import (
	"context"

	corelogs "github.com/fino-io/core/go/logs"
	"go.opentelemetry.io/otel/trace"
)

type traceLogger struct {
	corelogs.Logger
}

func withTrace(logger corelogs.Logger) corelogs.Logger {
	if logger == nil {
		return nil
	}
	if _, ok := logger.(*traceLogger); ok {
		return logger
	}
	return &traceLogger{Logger: logger}
}

func (l *traceLogger) With(fields ...Field) Logger {
	return withTrace(l.Logger.With(fields...))
}

func (l *traceLogger) Log(ctx context.Context, entry Entry) {
	entry.CallerSkip++
	if ctx != nil {
		span := trace.SpanContextFromContext(ctx)
		if span.IsValid() {
			entry.Fields = upsertFields(entry.Fields,
				Field{Key: "trace_id", Value: span.TraceID().String()},
				Field{Key: "span_id", Value: span.SpanID().String()},
			)
		}
	}
	l.Logger.Log(ctx, entry)
}

func upsertFields(fields []Field, authoritative ...Field) []Field {
	for _, candidate := range authoritative {
		updated := false
		for i := range fields {
			if fields[i].Key == candidate.Key {
				fields[i] = candidate
				updated = true
				break
			}
		}
		if !updated {
			fields = append(fields, candidate)
		}
	}
	return fields
}
