package nats

import (
	"context"
	"testing"

	gonats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
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
