package otel

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func Test_TracingMiddleware(t *testing.T) {
	testCases := []struct {
		description     string
		message         *azservicebus.ReceivedMessage
		carryRemoteSpan bool
	}{
		{
			description:     "nil message, context should not contain remote tracecontext",
			message:         nil,
			carryRemoteSpan: false,
		},
		{
			description: "context should contain remote tracecontext",
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
				// fake a remote span
				initUTTracerProvider()
				testRemoteCtx, testRemoteSpan := otel.Tracer("test-tracer").Start(context.Background(), "remote-span")
				testRemoteSpan.End()

				p := propagation.TraceContext{}
				p.Inject(testRemoteCtx, ReceivedMessageCarrierAdapter(tc.message))
			}

			ctx := Extract(context.TODO(), tc.message)

			// this works because TraceContext.Extract() sets the extracted tracecontext as the remote SpanContext of returned Context
			span := trace.SpanFromContext(ctx)

			g.Expect(span.SpanContext().IsValid()).To(Equal(tc.carryRemoteSpan))
		})
	}
}

func Test_MessageAttributes(t *testing.T) {
	testCases := []struct {
		name    string
		message *azservicebus.ReceivedMessage
		attr    []attribute.KeyValue
	}{
		{
			name: "nil message, context should not contain remote tracecontext",
			message: &azservicebus.ReceivedMessage{
				MessageID:     "message-id",
				CorrelationID: to.Ptr("correlation-id"),
				TimeToLive:    to.Ptr(time.Duration(10)),
			},
			attr: []attribute.KeyValue{
				attribute.String("message.id", "message-id"),
				attribute.String("message.correlationId", "correlation-id"),
				attribute.String("message.ttl", "10ns"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			g.Expect(MessageAttributes(tc.message)).To(Equal(tc.attr))
		})
	}
}

func initUTTracerProvider() {
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))
	otel.SetTracerProvider(tp)
}
