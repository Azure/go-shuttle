package tracing

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func Test_TracingMiddleware(t *testing.T) {
	correlationId := "correlation-id"
	timeToLive := time.Duration(10)
	scheduledEnqueueTime := time.Now().Add(time.Duration(10))

	// fake a remote span
	initUTTracerProvider()
	remoteCtx, remoteSpan := otel.Tracer("test-tracer").Start(context.Background(), "remote-span")
	remoteSpan.End()

	testCases := []struct {
		description     string
		message         *azservicebus.ReceivedMessage
		carryRemoteSpan bool
	}{
		{
			description:     "nil message, should start a new span without parent",
			message:         nil,
			carryRemoteSpan: false,
		},
		{
			description: "should set span attributes from message",
			message: &azservicebus.ReceivedMessage{
				MessageID:            "message-id",
				CorrelationID:        &correlationId,
				ScheduledEnqueueTime: &scheduledEnqueueTime,
				TimeToLive:           &timeToLive,
			},
			carryRemoteSpan: false,
		},
		{
			description: "should create child span from remote parent",
			message: &azservicebus.ReceivedMessage{
				MessageID: "message-id",
			},
			carryRemoteSpan: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g := NewGomegaWithT(t)
			if tc.carryRemoteSpan {
				propogator := propagation.TraceContext{}
				propogator.Inject(remoteCtx, carrierAdapterForReceivedMsg(tc.message))
				_, remoteSpan := getRemoteParentSpan(context.TODO(), tc.message)
				g.Expect(remoteSpan.SpanContext().IsValid()).To(BeTrue())
				g.Expect(remoteSpan.SpanContext().IsRemote()).To(BeTrue())
			}

			_, span := applyTracingMiddleWare(context.TODO(), tc.message)
			g.Expect(span.SpanContext().IsValid()).To(BeTrue())
		})
	}
}

func TestHandlers_SetMessageTrace(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))

	// fake a remote span
	remoteCtx, remoteSpan := tp.Tracer("test-tracer").Start(context.Background(), "remote-span")
	remoteSpan.End()

	// Inject the trace context into the message
	handler := PropagateFromContext(remoteCtx)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set trace carrier test: %s", err)
	}

	if blankMsg.ApplicationProperties == nil {
		t.Errorf("for trace carrier expected application properties to be set")
	}

	propogator := propagation.TraceContext{}
	ctx := propogator.Extract(context.TODO(), carrierAdapterForMsg(blankMsg))
	extractedSpan := trace.SpanFromContext(ctx)

	if !extractedSpan.SpanContext().IsValid() || !extractedSpan.SpanContext().IsRemote() {
		t.Errorf("expected extracted span to be valid and remote")
	}

	if reflect.DeepEqual(extractedSpan, remoteSpan) {
		t.Errorf("expected extracted span to be the same with remote span")
	}
}

func initUTTracerProvider() {
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))
	otel.SetTracerProvider(tp)
}
