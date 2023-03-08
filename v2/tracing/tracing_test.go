package tracing

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/propagation"
)

func Test_TracingMiddleware(t *testing.T) {
	correlationId := "correlation-id"
	timeToLive := time.Duration(10)
	scheduledEnqueueTime := time.Now().Add(time.Duration(10))

	// fake a remote span
	remoteCtx, remoteSpan := tracer.Start(context.Background(), "remote-span")
	remoteSpan.End()

	// inject the remote span into a trace carrier
	carrier := make(map[string]string)
	propogator := propagation.TraceContext{}
	propogator.Inject(remoteCtx, propagation.MapCarrier(carrier))

	testCases := []struct {
		description string
		message     *azservicebus.ReceivedMessage
	}{
		{
			description: "nil message, should start a new span without parent",
			message:     nil,
		},
		{
			description: "should set span attributes from message",
			message: &azservicebus.ReceivedMessage{
				MessageID:            "message-id",
				CorrelationID:        &correlationId,
				ScheduledEnqueueTime: &scheduledEnqueueTime,
				TimeToLive:           &timeToLive,
			},
		},
		{
			description: "should create child span from remote parent",
			message: &azservicebus.ReceivedMessage{
				MessageID: "message-id",
				ApplicationProperties: map[string]interface{}{
					traceCarrierField: carrier,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g := NewGomegaWithT(t)
			_, span := applyTracingMiddleWare(context.TODO(), tc.message)
			g.Expect(span.SpanContext().IsValid()).To(BeTrue())
		})
	}
}

func Test_getRemoteParentSpan(t *testing.T) {
	ctx, remoteSpan := tracer.Start(context.Background(), "remote-span")
	remoteSpan.End()

	carrier := make(map[string]string)
	propogator := propagation.TraceContext{}
	propogator.Inject(ctx, propagation.MapCarrier(carrier))

	testCases := []struct {
		description     string
		traceCarrier    map[string]string
		expectValidSpan bool // if the span is valid, it must be a remote span
	}{
		{
			description:     "nil trace carrier",
			traceCarrier:    nil,
			expectValidSpan: false,
		},
		{
			description:     "empty trace carrier",
			traceCarrier:    make(map[string]string),
			expectValidSpan: false,
		},
		{
			description:     "has valid trace carrier",
			traceCarrier:    carrier,
			expectValidSpan: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g := NewGomegaWithT(t)
			_, span := getRemoteParentSpan(context.TODO(), tc.traceCarrier)
			g.Expect(span.SpanContext().IsValid()).To(Equal(tc.expectValidSpan))
			g.Expect(span.SpanContext().IsRemote()).To(Equal(tc.expectValidSpan))
		})
	}
}
