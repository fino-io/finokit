package nats

import (
	"context"
	"errors"
	"testing"
	"unicode/utf8"

	gonats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceContextRoundTripThroughNATSHeader(t *testing.T) {
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
	})
	header := gonats.Header{}
	injectContext(trace.ContextWithSpanContext(context.Background(), parent), header)

	received := trace.SpanContextFromContext(extractContext(context.Background(), header))
	require.Equal(t, parent.TraceID(), received.TraceID())
	require.Equal(t, parent.SpanID(), received.SpanID())
	require.True(t, received.IsRemote())
}

func TestRecordSpanErrorSanitizesInvalidUTF8(t *testing.T) {
	wantErr := errors.New(string([]byte{'b', 0xff, 'd'}))
	span := &recordingSpan{}

	recordSpanError(span, wantErr)

	require.ErrorIs(t, span.recordedErr, wantErr)
	require.Equal(t, "b\uFFFDd", span.recordedErr.Error())
	require.True(t, utf8.ValidString(span.recordedErr.Error()))
	require.Equal(t, "b\uFFFDd", span.statusDescription)
}

type recordingSpan struct {
	trace.Span
	recordedErr       error
	statusDescription string
}

func (s *recordingSpan) RecordError(err error, _ ...trace.EventOption) {
	s.recordedErr = err
}

func (s *recordingSpan) SetStatus(_ codes.Code, description string) {
	s.statusDescription = description
}
