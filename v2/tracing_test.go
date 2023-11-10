package shuttle_test

import (
	"context"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Azure/go-shuttle/v2"
	shuttleotel "github.com/Azure/go-shuttle/v2/otel"
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
	ctx := propogator.Extract(context.TODO(), shuttleotel.MessageCarrierAdapter(blankMsg))
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
		spanStartOptions         []trace.SpanStartOption
		customSpanName           string
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
		{
			name:                     "has traceCtx on message - extract from message- custom attribute",
			hasTraceContextOnContext: true,
			hasTraceContextOnMessage: true,
			spanStartOptions: []trace.SpanStartOption{
				trace.WithAttributes(attribute.String("custom-attribute", "value")),
			},
		},
		{
			name:                     "has traceCtx on message - extract from message- custom spanName",
			hasTraceContextOnContext: true,
			hasTraceContextOnMessage: true,
			customSpanName:           "customSpanName",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(tt *testing.T) {
			g := NewWithT(tt)
			settler := &fakeSettler{}
			contextTraceID := trace.TraceID{1, 2, 3}
			messageTraceID := trace.TraceID{2, 3, 4}
			ctx := context.TODO()
			var spanCtx trace.SpanContext
			recorder := tracetest.NewSpanRecorder()
			tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()), tracesdk.WithSpanProcessor(recorder))
			defer recorder.Shutdown(ctx)
			options := []func(*shuttle.TracingHandlerOpts){shuttle.WithTraceProvider(tp)}
			if tc.customSpanName != "" {
				options = append(options,
					shuttle.WithReceiverSpanNameFormatter(func(_ string, _ *azservicebus.ReceivedMessage) string {
						return tc.customSpanName
					}),
				)
			}
			if tc.spanStartOptions != nil {
				options = append(options, shuttle.WithSpanStartOptions(tc.spanStartOptions))
			}
			h := shuttle.NewTracingHandler(
				shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
					spanCtx = trace.SpanContextFromContext(ctx)
				}), options...)

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
				propogator.Inject(msgContext, shuttleotel.ReceivedMessageCarrierAdapter(msg))
			}

			h.Handle(ctx, settler, msg)

			if tc.hasTraceContextOnMessage {
				g.Expect(spanCtx.TraceID()).To(Equal(messageTraceID))
			} else if tc.hasTraceContextOnContext {
				g.Expect(spanCtx.TraceID()).To(Equal(contextTraceID))
			}

			if len(tc.spanStartOptions) > 0 {
				spans := recorder.Ended()
				g.Expect(spans[0].Attributes()).To(ContainElement(attribute.String("custom-attribute", "value")))
			}

			if tc.customSpanName != "" {
				spans := recorder.Ended()
				g.Expect(spans[0].Name()).To(Equal(tc.customSpanName))
			}
		})
	}
}
