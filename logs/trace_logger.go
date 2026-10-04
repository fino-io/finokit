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
			fields := make([]Field, 0, len(entry.Fields)+2)
			for _, field := range entry.Fields {
				if field.Key != "trace_id" && field.Key != "span_id" {
					fields = append(fields, field)
				}
			}
			entry.Fields = append(fields,
				Field{Key: "trace_id", Value: span.TraceID().String()},
				Field{Key: "span_id", Value: span.SpanID().String()},
			)
		}
	}
	l.Logger.Log(ctx, entry)
}
