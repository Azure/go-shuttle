package shuttle_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"
	"github.com/Azure/go-shuttle/v2/otel"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestHandlers_SetMessageTrace(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))

	// fake a remote span
	remoteCtx, remoteSpan := tp.Tracer("test-tracer").Start(context.Background(), "remote-span")
	remoteSpan.End()

	// Inject the trace context into the message
	handler := shuttle.WithTracePropagation(remoteCtx)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set trace carrier test: %s", err)
	}

	if blankMsg.ApplicationProperties == nil {
		t.Errorf("for trace carrier expected application properties to be set")
	}

	propogator := propagation.TraceContext{}
	ctx := propogator.Extract(context.TODO(), otel.MessageCarrierAdapter(blankMsg))
	extractedSpan := trace.SpanFromContext(ctx)

	if !extractedSpan.SpanContext().IsValid() || !extractedSpan.SpanContext().IsRemote() {
		t.Errorf("expected extracted span to be valid and remote")
	}

	if reflect.DeepEqual(extractedSpan, remoteSpan) {
		t.Errorf("expected extracted span to be the same with remote span")
	}
}

func Test_NewTracingHandler(t *testing.T) {
	testCases := []struct {
		name                     string
		hasTraceContextOnContext bool
		hasTraceContextOnMessage bool
	}{
		{
			name:                     "no traceCtx - local traceContext added",
			hasTraceContextOnContext: false,
			hasTraceContextOnMessage: false,
		},
		{
			name:                     "has traceCtx on context - use the one on context",
			hasTraceContextOnContext: true,
			hasTraceContextOnMessage: false,
		},
		{
			name:                     "has traceCtx on message - extract from message ",
			hasTraceContextOnContext: true,
			hasTraceContextOnMessage: true,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(tt *testing.T) {
			g := NewWithT(tt)
			settler := &fakeSettler{}
			contextTraceID := trace.TraceID{1, 2, 3}
			messageTraceID := trace.TraceID{2, 3, 4}
			var spanCtx trace.SpanContext
			h := shuttle.NewTracingHandler(
				shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
					spanCtx = trace.SpanContextFromContext(ctx)
				}))

			ctx := context.TODO()
			if tc.hasTraceContextOnContext {
				ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
					TraceID: contextTraceID,
					SpanID:  trace.SpanID{1, 2, 3},
				}))
			}

			msg := &azservicebus.ReceivedMessage{}
			if tc.hasTraceContextOnMessage {
				msgContext := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
					TraceID: messageTraceID,
					SpanID:  trace.SpanID{1, 2, 3},
				}))
				propogator := propagation.TraceContext{}
				propogator.Inject(msgContext, otel.ReceivedMessageCarrierAdapter(msg))
			}

			h.Handle(ctx, settler, msg)
			if tc.hasTraceContextOnMessage {
				g.Expect(spanCtx.TraceID()).To(Equal(messageTraceID))
			} else if tc.hasTraceContextOnContext {
				g.Expect(spanCtx.TraceID()).To(Equal(contextTraceID))
			}
			g.Expect(spanCtx.IsRemote()).To(Equal(tc.hasTraceContextOnMessage))
		})
	}
}
